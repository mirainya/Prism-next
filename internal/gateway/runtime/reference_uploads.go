package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/video/generic"
	"github.com/mirainya/Prism/pkg/safeurl"
)

type referenceUploadExchange func(context.Context, string, *http.Request, []byte) ([]byte, http.Header, error)

// prepareReferenceUploads runs before the generation dispatch fence. Uploads can
// be retried safely; they never create a generation or alter the canonical input.
func (d *AsyncDispatcher) prepareReferenceUploads(ctx context.Context, item repository.OutboxItem, fixed repository.AsyncDispatch, prepared AsyncRequest, secret []byte) (AsyncRequest, error) {
	if fixed.AdapterCode != "generic" {
		return prepared, nil
	}
	var config struct {
		Adapter struct {
			Request struct {
				ReferenceUpload *generic.ReferenceUploadConfig `json:"reference_upload"`
			} `json:"request"`
		} `json:"adapter"`
	}
	if err := json.Unmarshal(fixed.AdapterConfig, &config); err != nil {
		return prepared, err
	}
	if config.Adapter.Request.ReferenceUpload == nil {
		return prepared, nil
	}
	if err := config.Adapter.Request.ReferenceUpload.Validate(); err != nil {
		return prepared, &PermanentDispatchError{Code: "unsupported_reference_upload"}
	}
	// Validate the configured provider origin against the transport allowlist
	// before any credential-bearing upload request.
	if _, err := d.prepareHTTP(ctx, fixed, prepared, secret); err != nil {
		return prepared, err
	}
	timeout := time.Until(item.LeaseExpiresAt) - 5*time.Second
	if timeout <= 0 {
		return prepared, context.DeadlineExceeded
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	loader := storedAsyncAssetLoader{store: d.service.Store, callID: fixed.CallID, maxBytes: 200 << 20}
	load := func(ctx context.Context, location string) (AsyncAsset, error) {
		return loader.loadAsyncAsset(ctx, location, func(ctx context.Context, location string) (AsyncAsset, error) {
			r, err := safeurl.Download(ctx, location, 200<<20)
			if err != nil {
				return AsyncAsset{}, errors.Join(ErrAsyncAssetUnavailable, err)
			}
			return AsyncAsset{Data: r.Data, ContentType: r.ContentType}, nil
		})
	}
	exchange := func(ctx context.Context, action string, request *http.Request, body []byte) ([]byte, http.Header, error) {
		return d.referenceUploadExchange(ctx, item, action, request, body)
	}
	body, err := uploadMappedReferences(ctx, fixed.BaseURL, secret, prepared.Body, config.Adapter.Request.ReferenceUpload, load, exchange)
	if err != nil {
		return prepared, err
	}
	clear(prepared.Body)
	prepared.Body = body
	return prepared, nil
}

// Ancillary requests get their own capacity slot and evidence, without the
// submit fence/outbox uniqueness key: only the final generation is a submit.
func (d *AsyncDispatcher) referenceUploadExchange(ctx context.Context, item repository.OutboxItem, action string, request *http.Request, body []byte) ([]byte, http.Header, error) {
	if !strings.HasPrefix(action, "upload_") {
		return nil, nil, repository.ErrInvalidInput
	}
	var payload *repository.BlobInput
	if len(body) > 0 && len(body) <= maxCapturedExchangeBody {
		blob, err := d.payloadBlob(ctx, body)
		if err != nil {
			return nil, nil, err
		}
		payload = &blob
	}
	var requestID uint64
	err := d.service.Store.WithTx(ctx, func(tx *sql.Tx) error {
		state, err := d.service.lockAsyncAction(ctx, tx, item, false)
		if err != nil {
			return err
		}
		if item.Action != "submit" || state.version != item.StateVersion || state.sequence != item.ActionSeq || !asyncRequestAllowed(state.state, item.Action) {
			return repository.ErrConflict
		}
		// A worker may have stopped during an upload. Release its request slot;
		// hash-based upstream reuse will safely reconcile the file on this run.
		rows, err := tx.QueryContext(ctx, `SELECT id FROM gw_channel_request_logs WHERE attempt_id=? AND action IN ('upload_prepare','upload_part','upload_complete','upload_wait','upload_abort') AND status IN ('dispatching','sent') FOR UPDATE`, state.attemptID)
		if err != nil {
			return err
		}
		var stale []uint64
		for rows.Next() {
			var id uint64
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			stale = append(stale, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range stale {
			if err = d.service.finishRequest(ctx, tx, id, "unknown", repository.RequestLogResult{ErrorCode: "worker_upload_exchange_lost"}); err != nil {
				return err
			}
		}
		var seq uint64
		err = tx.QueryRowContext(ctx, `SELECT request_seq FROM gw_channel_request_logs WHERE attempt_id=? ORDER BY request_seq DESC LIMIT 1 FOR UPDATE`, state.attemptID).Scan(&seq)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		requestID, err = d.service.beginRequest(ctx, tx, repository.RequestLogInput{AttemptID: &state.attemptID, RequestSeq: seq + 1, Action: action, MappingHMAC: d.digest([]byte(request.Method + " " + request.URL.String())), RequestBytesHMAC: d.digest(body)})
		if err != nil {
			return err
		}
		if payload != nil {
			if _, err = d.service.Store.PutRequestLogPayload(ctx, tx, requestID, "request", *payload); err != nil {
				return err
			}
		}
		// Upload evidence belongs to the attempt, not the unique generation outbox exchange.
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	start := time.Now()
	response, sendErr := d.client.Do(request)
	elapsed := uint64(time.Since(start).Milliseconds())
	result := repository.RequestLogResult{RequestComplete: true, DurationMS: &elapsed}
	var data []byte
	var headers http.Header
	status := "unknown"
	if sendErr == nil {
		defer response.Body.Close()
		code := uint16(response.StatusCode)
		result.HTTPStatus = &code
		headers = response.Header.Clone()
		data, sendErr = io.ReadAll(io.LimitReader(response.Body, maxCapturedExchangeBody+1))
		if sendErr == nil && len(data) <= maxCapturedExchangeBody {
			result.ResponseComplete = true
			status = "response_recorded"
		} else if sendErr == nil {
			sendErr = errors.New("upload response too large")
		}
	}
	if sendErr != nil {
		result.ErrorCode = "reference_upload_exchange_failed"
	} else if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.ErrorCode = "reference_upload_http_error"
	}
	result.ResponseBytesHMAC = d.digest(data)
	if len(data) > 0 && len(data) <= maxCapturedExchangeBody {
		blob, e := d.payloadBlob(context.WithoutCancel(ctx), data)
		if e != nil {
			return nil, nil, e
		}
		result.ResponsePayload = &blob
	}
	markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err = d.service.FinishRequest(markCtx, requestID, status, result); err != nil {
		return nil, nil, err
	}
	if sendErr != nil {
		return nil, nil, errors.New("reference upload exchange failed")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("reference upload HTTP %d", response.StatusCode)
	}
	return data, headers, nil
}

type referenceUploadState struct {
	Disposition       string `json:"disposition"`
	Ticket            string `json:"ticket"`
	RetryAfterSeconds int    `json:"retry_after_seconds"`

	StorageObjectID string `json:"storage_object_id"`
	Upload          struct {
		SessionToken string `json:"session_token"`
		Generation   int    `json:"generation"`
		Mode         string `json:"mode"`
		URL          string `json:"upload_url"`
		Parts        []struct {
			Number int    `json:"part_number"`
			Size   int64  `json:"size_bytes"`
			URL    string `json:"upload_url"`
		} `json:"parts"`
	} `json:"upload"`
}

func uploadMappedReferences(ctx context.Context, base string, secret, body []byte, config *generic.ReferenceUploadConfig, load func(context.Context, string) (AsyncAsset, error), exchange referenceUploadExchange) ([]byte, error) {
	var input map[string]json.RawMessage
	if err := json.Unmarshal(body, &input); err != nil {
		return nil, err
	}
	var content []map[string]json.RawMessage
	if err := json.Unmarshal(input[config.ContentPath], &content); err != nil && len(input[config.ContentPath]) > 0 {
		return nil, err
	}
	cache := map[string]referenceUploadState{}
	for index, entry := range content {
		var location, kind, existing string
		_ = json.Unmarshal(entry[config.ContentFields["url"]], &location)
		_ = json.Unmarshal(entry[config.ContentFields["type"]], &kind)
		_ = json.Unmarshal(entry[config.ContentFields["reference"]], &existing)
		if existing != "" || location == "" {
			continue
		}
		kind = strings.TrimSuffix(kind, "_url")
		if kind != "image" && kind != "video" && kind != "audio" {
			return nil, errors.New("unsupported reference upload kind")
		}
		key := kind + ":" + location
		ref, ok := cache[key]
		if !ok {
			asset, err := load(ctx, location)
			if err != nil {
				return nil, fmt.Errorf("reference %d could not be read: %w", index, err)
			}
			ref, err = uploadMappedAsset(ctx, base, secret, kind, asset, config, exchange)
			clear(asset.Data)
			if err != nil {
				return nil, fmt.Errorf("reference %d upload: %w", index, err)
			}
			cache[key] = ref
		}
		entry[config.ContentFields["reference"]], _ = json.Marshal(ref.StorageObjectID)
		delete(entry, config.ContentFields["url"])
	}
	if len(content) > 0 {
		input[config.ContentPath], _ = json.Marshal(content)
	}
	return json.Marshal(input)
}

func uploadMappedAsset(ctx context.Context, base string, secret []byte, kind string, asset AsyncAsset, config *generic.ReferenceUploadConfig, exchange referenceUploadExchange) (out referenceUploadState, err error) {
	limit := 20 << 20
	if kind == "video" {
		limit = 200 << 20
	}
	ct, _, parseErr := mime.ParseMediaType(asset.ContentType)
	if ct == "audio/wave" || ct == "audio/vnd.wave" {
		ct = "audio/wav"
	}
	if parseErr != nil || len(asset.Data) == 0 || len(asset.Data) > limit {
		return out, errors.New("invalid reference file size or content type")
	}
	allowed := map[string]string{"image/png": "image", "image/jpeg": "image", "image/gif": "image", "image/webp": "image", "video/mp4": "video", "video/quicktime": "video", "audio/mpeg": "audio", "audio/wav": "audio", "audio/x-wav": "audio"}
	if allowed[ct] != kind {
		return out, errors.New("unsupported reference file type")
	}
	origin, e := url.Parse(base)
	if e != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") {
		return out, errors.New("invalid upload base URL")
	}
	call := func(ctx context.Context, action string, value map[string]any) (referenceUploadState, error) {
		var state referenceUploadState
		data, e := config.RequestBody(action, value)
		if e != nil {
			return state, e
		}
		location := origin.ResolveReference(&url.URL{Path: config.Operations[action]}).String()
		request, e := http.NewRequestWithContext(ctx, http.MethodPost, location, bytes.NewReader(data))
		if e != nil {
			return state, e
		}
		request.Header.Set("Authorization", "Bearer "+string(secret))
		request.Header.Set("Content-Type", "application/json")
		if action == "prepare" {
			request.Header.Set("Idempotency-Key", uuid.NewString())
		}
		response, _, e := exchange(ctx, "upload_"+action, request, data)
		if e != nil {
			return state, e
		}
		normalized, e := config.DecodeResponse(response)
		if e != nil {
			return state, e
		}
		if e = json.Unmarshal(normalized, &state); e != nil {
			return state, errors.New("invalid reference upload response")
		}
		return state, nil
	}
	hash := sha256.Sum256(asset.Data)
	state, err := call(ctx, "prepare", map[string]any{"sha256": hex.EncodeToString(hash[:]), "size_bytes": len(asset.Data), "kind": kind, "content_type": ct})
	if err != nil {
		return out, err
	}
	for state.Disposition == "waiting" {
		if state.Ticket == "" {
			return out, errors.New("missing upload wait ticket")
		}
		delay := state.RetryAfterSeconds
		if delay < 1 {
			delay = 1
		}
		if delay > 30 {
			delay = 30
		}
		timer := time.NewTimer(time.Duration(delay) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return out, ctx.Err()
		case <-timer.C:
		}
		state, err = call(ctx, "wait", map[string]any{"ticket": state.Ticket})
		if err != nil {
			return out, err
		}
	}
	if state.Disposition == "reused" {
		out = state
		if out.StorageObjectID == "" {
			return out, errors.New("missing reused reference ID")
		}
		return out, nil
	}
	if state.Disposition != "owner" || state.Upload.SessionToken == "" || state.Upload.Generation <= 0 {
		return out, errors.New("invalid upload ownership response")
	}
	complete := map[string]any{"session_token": state.Upload.SessionToken, "generation": state.Upload.Generation}
	done := false
	defer func() {
		if !done {
			abortCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_, _ = call(abortCtx, "abort", complete)
		}
	}()
	put := func(location string, data []byte, single bool) (string, error) {
		u, e := url.Parse(location)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
			return "", errors.New("invalid signed upload URL")
		}
		request, e := http.NewRequestWithContext(ctx, http.MethodPut, location, bytes.NewReader(data))
		if e != nil {
			return "", e
		}
		if single {
			request.Header.Set("Content-Type", ct)
			request.Header.Set("If-None-Match", "*")
		}
		_, headers, e := exchange(ctx, "upload_part", request, data)
		if e != nil {
			return "", e
		}
		return headers.Get("ETag"), nil
	}
	switch state.Upload.Mode {
	case "single":
		_, err = put(state.Upload.URL, asset.Data, true)
	case "multipart":
		parts := state.Upload.Parts
		sort.Slice(parts, func(i, j int) bool { return parts[i].Number < parts[j].Number })
		var total int64
		for i, p := range parts {
			if p.Number != i+1 || p.Size <= 0 || p.Size > int64(len(asset.Data))-total {
				return out, errors.New("invalid upload parts")
			}
			total += p.Size
		}
		if total != int64(len(asset.Data)) {
			return out, errors.New("upload parts size mismatch")
		}
		completed := make([]map[string]any, 0, len(parts))
		var offset int64
		for _, part := range parts {
			etag, e := put(part.URL, asset.Data[offset:offset+part.Size], false)
			if e != nil {
				return out, e
			}
			if etag == "" {
				return out, errors.New("missing upload ETag")
			}
			partBody := map[string]any{}
			partBody[config.CompletedParts["part_number"]] = part.Number
			partBody[config.CompletedParts["etag"]] = etag
			completed = append(completed, partBody)
			offset += part.Size
		}
		complete["parts"] = completed
	default:
		return out, errors.New("unsupported upload mode")
	}
	if err != nil {
		return out, err
	}
	out, err = call(ctx, "complete", complete)
	if err != nil {
		return out, err
	}
	if out.StorageObjectID == "" {
		return out, errors.New("missing uploaded reference ID")
	}
	done = true
	return out, nil
}
