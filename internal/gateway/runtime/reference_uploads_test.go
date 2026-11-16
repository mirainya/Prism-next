package runtime

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/video/generic"
)

func TestReferenceUploadOptIn(t *testing.T) {
	d := &AsyncDispatcher{}
	for _, config := range []string{`{}`, `{"adapter":{"request":{}}}`} {
		prepared := AsyncRequest{Body: []byte(`{"content":[{"url":"https://media.example/a.png"}]}`)}
		got, err := d.prepareReferenceUploads(context.Background(), repository.OutboxItem{}, repository.AsyncDispatch{AdapterCode: "generic", AdapterConfig: []byte(config)}, prepared, nil)
		if err != nil || !bytes.Equal(got.Body, prepared.Body) {
			t.Fatalf("unconfigured channel changed: %v", err)
		}
	}
}

func TestMappedReferenceUploadReplacesURLsAndReusesDuplicates(t *testing.T) {
	input := []byte(`{"model":"video-model","content":[{"type":"audio_url","role":"reference_audio","url":"https://media.example/a.mp3","duration_seconds":2.5},{"type":"audio_url","role":"reference_audio","url":"https://media.example/a.mp3","duration_seconds":3}]}`)
	loads, calls := 0, 0
	load := func(context.Context, string) (AsyncAsset, error) {
		loads++
		return AsyncAsset{Data: []byte("audio"), ContentType: "audio/mpeg"}, nil
	}
	exchange := func(_ context.Context, action string, r *http.Request, body []byte) ([]byte, http.Header, error) {
		calls++
		switch action {
		case "upload_prepare":
			if r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Idempotency-Key") == "" {
				t.Fatal("missing upload authorization/idempotency")
			}
			return []byte(`{"code":0,"data":{"disposition":"owner","upload":{"session_token":"session","generation":1,"mode":"single","upload_url":"https://storage.example/signed"}}}`), nil, nil
		case "upload_part":
			if r.Header.Get("Authorization") != "" || r.Header.Get("If-None-Match") != "*" || r.Header.Get("Content-Type") != "audio/mpeg" {
				t.Fatal("incorrect signed upload headers")
			}
			read, _ := io.ReadAll(r.Body)
			if string(read) != "audio" || r.ContentLength != 5 {
				t.Fatal("incorrect upload bytes")
			}
			return nil, http.Header{}, nil
		case "upload_complete":
			return []byte(`{"code":0,"data":{"storage_object_id":"object-1"}}`), nil, nil
		default:
			t.Fatalf("unexpected action %s", action)
			return nil, nil, nil
		}
	}
	output, err := uploadMappedReferences(context.Background(), "https://provider.example", []byte("secret"), input, uploadTestConfig(), load, exchange)
	if err != nil {
		t.Fatal(err)
	}
	if loads != 1 || calls != 3 || !strings.Contains(string(input), "https://media.example") {
		t.Fatal("duplicate upload or mutated input")
	}
	var parsed struct {
		Content []map[string]any `json:"content"`
	}
	if err = json.Unmarshal(output, &parsed); err != nil {
		t.Fatal(err)
	}
	for i, item := range parsed.Content {
		if item["url"] != nil || item["storage_object_id"] != "object-1" || item["role"] != "reference_audio" || item["duration_seconds"] != []float64{2.5, 3}[i] {
			t.Fatalf("incorrect reference: %#v", item)
		}
	}
}

func TestMappedUploadMultipartAndCompletion(t *testing.T) {
	var uploaded []string
	var completed bool
	exchange := func(_ context.Context, action string, r *http.Request, body []byte) ([]byte, http.Header, error) {
		switch action {
		case "upload_prepare":
			return []byte(`{"code":0,"data":{"disposition":"owner","upload":{"session_token":"s","generation":2,"mode":"multipart","parts":[{"part_number":2,"size_bytes":2,"upload_url":"https://storage.example/2"},{"part_number":1,"size_bytes":3,"upload_url":"https://storage.example/1"}]}}}`), nil, nil
		case "upload_part":
			if r.Header.Get("Authorization") != "" || r.Header.Get("Content-Type") != "" || r.Header.Get("Content-Range") != "" {
				t.Fatal("multipart header leak")
			}
			uploaded = append(uploaded, string(body))
			return nil, http.Header{"Etag": []string{`"` + r.URL.Path + `"`}}, nil
		case "upload_complete":
			var value struct {
				Parts []struct {
					Number int    `json:"part_number"`
					ETag   string `json:"etag"`
				} `json:"parts"`
			}
			if err := json.Unmarshal(body, &value); err != nil {
				t.Fatal(err)
			}
			if len(value.Parts) != 2 || value.Parts[0].Number != 1 || value.Parts[0].ETag != `"/1"` || value.Parts[1].ETag != `"/2"` {
				t.Fatal("incorrect completed parts")
			}
			completed = true
			return []byte(`{"code":0,"data":{"storage_object_id":"video-object"}}`), nil, nil
		default:
			t.Fatal(action)
			return nil, nil, nil
		}
	}
	ref, err := uploadMappedAsset(context.Background(), "https://provider.example", []byte("secret"), "video", AsyncAsset{Data: []byte("abcde"), ContentType: "video/mp4"}, uploadTestConfig(), exchange)
	if err != nil || ref.StorageObjectID != "video-object" || !completed || strings.Join(uploaded, ",") != "abc,de" {
		t.Fatalf("incorrect upload: %v, %v", uploaded, err)
	}
}

func TestMappedUploadFailureAbortsAndDoesNotComplete(t *testing.T) {
	for _, mode := range []string{"single", "multipart"} {
		t.Run(mode, func(t *testing.T) {
			aborted := false
			exchange := func(_ context.Context, action string, r *http.Request, body []byte) ([]byte, http.Header, error) {
				switch action {
				case "upload_prepare":
					return []byte(`{"code":0,"data":{"disposition":"owner","upload":{"session_token":"s","generation":1,"mode":"` + mode + `","upload_url":"https://storage.example/file","parts":[{"part_number":1,"size_bytes":99,"upload_url":"https://storage.example/file"}]}}}`), nil, nil
				case "upload_part":
					return nil, nil, errors.New("disconnected")
				case "upload_abort":
					aborted = true
					return []byte(`{"code":0,"data":{}}`), nil, nil
				default:
					t.Fatal("failed file was completed")
					return nil, nil, nil
				}
			}
			_, err := uploadMappedAsset(context.Background(), "https://provider.example", nil, "image", AsyncAsset{Data: []byte("png"), ContentType: "image/png"}, uploadTestConfig(), exchange)
			if err == nil || !aborted {
				t.Fatalf("failure not aborted: %v", err)
			}
		})
	}
}

func TestMappedUploadReuseAndRetry(t *testing.T) {
	keys := map[string]bool{}
	for attempt := 0; attempt < 2; attempt++ {
		exchange := func(_ context.Context, action string, r *http.Request, body []byte) ([]byte, http.Header, error) {
			if action != "upload_prepare" {
				t.Fatal("reused file uploaded again")
			}
			key := r.Header.Get("Idempotency-Key")
			if key == "" || keys[key] {
				t.Fatal("new upload attempt reused stale idempotency key")
			}
			keys[key] = true
			return []byte(`{"code":0,"data":{"disposition":"reused","storage_object_id":"existing"}}`), nil, nil
		}
		ref, err := uploadMappedAsset(context.Background(), "https://provider.example", nil, "image", AsyncAsset{Data: []byte("png"), ContentType: "image/png"}, uploadTestConfig(), exchange)
		if err != nil || ref.StorageObjectID != "existing" {
			t.Fatalf("reuse failed: %v", err)
		}
	}
}

func TestMappedUploadWaitingHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	exchange := func(_ context.Context, action string, _ *http.Request, _ []byte) ([]byte, http.Header, error) {
		if action != "upload_prepare" {
			t.Fatal("cancelled wait made another request")
		}
		cancel()
		return []byte(`{"code":0,"data":{"disposition":"waiting","ticket":"wait-ticket","retry_after_seconds":30}}`), nil, nil
	}
	_, err := uploadMappedAsset(ctx, "https://provider.example", nil, "image", AsyncAsset{Data: []byte("png"), ContentType: "image/png"}, uploadTestConfig(), exchange)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("wait ignored cancellation: %v", err)
	}
}

func TestMappedUploadRejectsUnsafeSignedTarget(t *testing.T) {
	exchange := func(_ context.Context, action string, _ *http.Request, _ []byte) ([]byte, http.Header, error) {
		switch action {
		case "upload_prepare":
			return []byte(`{"code":0,"data":{"disposition":"owner","upload":{"session_token":"s","generation":1,"mode":"single","upload_url":"http://storage.example/file"}}}`), nil, nil
		case "upload_abort":
			return []byte(`{"code":0,"data":{}}`), nil, nil
		default:
			t.Fatal("unsafe signed target was contacted")
			return nil, nil, nil
		}
	}
	_, err := uploadMappedAsset(context.Background(), "https://provider.example", nil, "image", AsyncAsset{Data: []byte("png"), ContentType: "image/png"}, uploadTestConfig(), exchange)
	if err == nil {
		t.Fatal("insecure upload target accepted")
	}
}

func TestReferenceUploadRejectsExpiredLeaseBeforeSending(t *testing.T) {
	dispatcher, mock := testDispatcher(t, nil)
	item := repository.OutboxItem{ID: 31, AsyncExecutionID: 9, ActionSeq: 4, StateVersion: 3, Action: "submit", Attempts: 1, LeaseOwner: "worker"}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT attempt_id FROM gw_async_executions`).WithArgs(uint64(9)).WillReturnRows(sqlmock.NewRows([]string{"attempt"}).AddRow(12))
	mock.ExpectQuery(`SELECT id FROM gw_api_call_attempts`).WithArgs(uint64(12)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(12))
	mock.ExpectQuery(`SELECT state,state_version,action_seq FROM gw_async_executions`).WithArgs(uint64(9)).WillReturnRows(sqlmock.NewRows([]string{"state", "version", "seq"}).AddRow("submitting", 3, 4))
	mock.ExpectQuery(`SELECT 1 FROM gw_async_outbox`).WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	request, _ := http.NewRequest(http.MethodPut, "https://storage.example/upload", nil)
	_, _, err := dispatcher.referenceUploadExchange(context.Background(), item, "upload_part", request, nil)
	if !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("stale upload dispatched: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func uploadTestConfig() *generic.ReferenceUploadConfig {
	return &generic.ReferenceUploadConfig{
		Operations:  map[string]string{"prepare": "/uploads/new", "wait": "/uploads/pending", "complete": "/uploads/finish", "abort": "/uploads/cancel"},
		ContentPath: "content", ContentFields: map[string]string{"url": "url", "type": "type", "reference": "storage_object_id"},
		RequestFields:  map[string]string{"sha256": "sha256", "size_bytes": "size_bytes", "kind": "kind", "content_type": "content_type", "ticket": "ticket", "session_token": "session_token", "generation": "generation", "parts": "parts"},
		ResponseFields: map[string]string{"disposition": "data.disposition", "ticket": "data.ticket", "retry_after_seconds": "data.retry_after_seconds", "storage_object_id": "data.storage_object_id", "upload.session_token": "data.upload.session_token", "upload.generation": "data.upload.generation", "upload.mode": "data.upload.mode", "upload.upload_url": "data.upload.upload_url", "upload.parts": "data.upload.parts"},
		PartFields:     map[string]string{"part_number": "part_number", "size_bytes": "size_bytes", "upload_url": "upload_url"},
		CompletedParts: map[string]string{"part_number": "part_number", "etag": "etag"},
		SuccessPath:    "code", SuccessValues: []string{"0"},
		StateValues: map[string]string{"owner": "owner", "waiting": "waiting", "reused": "reused"},
		ModeValues:  map[string]string{"single": "single", "multipart": "multipart"},
	}
}

func TestReferenceUploadContractIsSuppliedByConfiguration(t *testing.T) {
	config := uploadTestConfig()
	config.Operations["prepare"] = "/assets/reserve"
	config.Operations["complete"] = "/assets/commit"
	config.RequestFields["sha256"] = "input.digest"
	config.FixedBody = map[string]map[string]any{"prepare": {"purpose": "example"}}
	config.ResponseFields["disposition"] = "payload.status"
	config.ResponseFields["storage_object_id"] = "payload.object"
	config.ResponseFields["upload.mode"] = "payload.transfer"
	config.ResponseFields["upload.session_token"] = "payload.session"
	config.ResponseFields["upload.generation"] = "payload.version"
	config.ResponseFields["upload.upload_url"] = "payload.destination"
	config.StateValues["owner"] = "reserved"
	config.ModeValues["single"] = "put"
	config.ContentPath = "attachments"
	config.ContentFields = map[string]string{"url": "source", "type": "media", "reference": "object"}
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	load := func(context.Context, string) (AsyncAsset, error) {
		return AsyncAsset{Data: []byte("image"), ContentType: "image/png"}, nil
	}
	exchange := func(_ context.Context, action string, request *http.Request, body []byte) ([]byte, http.Header, error) {
		switch action {
		case "upload_prepare":
			if request.URL.Path != "/assets/reserve" || !bytes.Contains(body, []byte(`"digest"`)) || !bytes.Contains(body, []byte(`"purpose":"example"`)) {
				t.Fatalf("configured upload request was not applied: %s", body)
			}
			return []byte(`{"code":0,"payload":{"status":"reserved","transfer":"put","session":"s","version":1,"destination":"https://storage.example/object"}}`), nil, nil
		case "upload_part":
			if request.Header.Get("Authorization") != "" {
				t.Fatal("provider credential leaked to signed upload")
			}
			return nil, http.Header{}, nil
		case "upload_complete":
			if request.URL.Path != "/assets/commit" {
				t.Fatal("configured completion path was not applied")
			}
			return []byte(`{"code":0,"payload":{"object":"example-object"}}`), nil, nil
		default:
			t.Fatalf("unexpected upload action %s", action)
			return nil, nil, nil
		}
	}
	out, err := uploadMappedReferences(context.Background(), "https://provider.example", []byte("secret"), []byte(`{"attachments":[{"media":"image","source":"https://media.example/input.png"}]}`), config, load, exchange)
	if err != nil || !bytes.Contains(out, []byte(`"object":"example-object"`)) || bytes.Contains(out, []byte(`"source"`)) {
		t.Fatalf("configured reference mapping failed: %s %v", out, err)
	}
}

func TestReferenceUploadRejectsInvalidConfiguration(t *testing.T) {
	for _, change := range []func(*generic.ReferenceUploadConfig){
		func(c *generic.ReferenceUploadConfig) { c.Operations["prepare"] = "https://other.example/upload" },
		func(c *generic.ReferenceUploadConfig) { c.Operations["prepare"] = "//other.example/upload" },
		func(c *generic.ReferenceUploadConfig) { c.Operations["prepare"] = "/%2e%2e/upload" },
		func(c *generic.ReferenceUploadConfig) { c.RequestFields["sha256"] = c.RequestFields["kind"] },
		func(c *generic.ReferenceUploadConfig) { c.ContentFields["reference"] = "nested.object" },
		func(c *generic.ReferenceUploadConfig) { c.ResponseFields["other"] = "data.other" },
		func(c *generic.ReferenceUploadConfig) { c.SuccessValues = nil },
		func(c *generic.ReferenceUploadConfig) { c.StateValues["owner"] = c.StateValues["reused"] },
	} {
		config := uploadTestConfig()
		change(config)
		if err := config.Validate(); err == nil {
			t.Fatal("unsafe or ambiguous upload configuration was accepted")
		}
	}
	config := uploadTestConfig()
	for _, body := range []string{`{}`, `{"code":1}`, `{"code":0,"data":{"disposition":"unknown"}}`, `{"code":0,"data":{"upload":{"parts":{}}}}`} {
		if _, err := config.DecodeResponse([]byte(body)); err == nil {
			t.Fatalf("invalid upload response was accepted: %s", body)
		}
	}
}
