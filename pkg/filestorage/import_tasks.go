package filestorage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	ErrImportTaskNotFound   = errors.New("file import task not found")
	ErrImportedFileNotFound = errors.New("imported file not found")
	importTaskKeyPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

// URLImportTask contains only the public task view, never the source URL or key.
type URLImportTask struct {
	TaskKey   string        `json:"taskKey"`
	State     string        `json:"state"`
	ErrorCode string        `json:"errorCode"`
	Attempts  int           `json:"attempts"`
	CanRetry  bool          `json:"canRetry"`
	Result    *UploadResult `json:"result"`
}

func (t URLImportTask) String() string {
	return fmt.Sprintf("file import task (state=%s, attempts=%d)", t.State, t.Attempts)
}

// URLImportBindingSHA256 identifies the bound endpoint and credential without
// persisting them. A changed binding must not create the task in another scope.
func (c Client) URLImportBindingSHA256() string {
	if !c.Configured() {
		return ""
	}
	encoded, _ := json.Marshal([]string{strings.TrimRight(c.config.BaseURL, "/"), c.config.APIKey})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// SupportsURLImportTasks is a read-only version probe. Callers must persist
// the chosen mode before submitting anything; a later failed probe must never
// switch an existing task to the synchronous upload endpoint.
func (c Client) SupportsURLImportTasks(ctx context.Context) (bool, error) {
	response, status, err := c.importAPI(ctx, http.MethodGet, "/api/v1/url-imports", nil, nil)
	if status == 404 || status == 405 || status == 501 || response.Code == 404 {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var capability struct {
		Version int `json:"schemaVersion"`
	}
	if json.Unmarshal(response.Data, &capability) != nil || capability.Version != 1 {
		return false, &StorageError{Code: "response"}
	}
	return true, nil
}

// CreateURLImportTask is safe to repeat only with the same durable task key
// and immutable request. It never falls back to /upload-url.
func (c Client) CreateURLImportTask(ctx context.Context, taskKey, originURL, storagePath, originalFilename string) (URLImportTask, error) {
	request, err := validatedTaskImport(taskKey, originURL, storagePath, originalFilename)
	if err != nil {
		return URLImportTask{}, err
	}
	response, _, err := c.importAPI(ctx, http.MethodPost, "/api/v1/url-imports", nil, request)
	if err != nil {
		return URLImportTask{}, err
	}
	task, err := decodeURLImportTask(response.Data, taskKey)
	if err != nil {
		return URLImportTask{}, err
	}
	if task.Result != nil {
		expectedPath, _ := validatedImportLookupPath(storagePath, originalFilename)
		if task.Result.Path != expectedPath || task.Result.OriginalName != strings.TrimSpace(originalFilename) {
			return URLImportTask{}, &StorageError{Code: "response"}
		}
	}
	return task, nil
}

func (c Client) ReadURLImportTask(ctx context.Context, taskKey string) (URLImportTask, error) {
	return c.requestURLImportTask(ctx, http.MethodGet, taskKey, "")
}

// RetryURLImportTask is explicit: pending or uncertain tasks are never retried
// by this client. The storage server atomically enforces confirmed failure.
func (c Client) RetryURLImportTask(ctx context.Context, taskKey string) (URLImportTask, error) {
	return c.requestURLImportTask(ctx, http.MethodPost, taskKey, "/retry")
}

// RetryURLImportTaskOnce gives one authorized manual action one durable retry.
// A dedicated endpoint prevents an older server from silently ignoring the ID.
func (c Client) RetryURLImportTaskOnce(ctx context.Context, taskKey string, requestID uint64) (URLImportTask, error) {
	if !importTaskKeyPattern.MatchString(taskKey) || requestID == 0 || requestID > 1<<63-1 {
		return URLImportTask{}, &StorageError{Code: "rejected"}
	}
	response, status, err := c.importAPI(ctx, http.MethodPost, "/api/v1/url-imports/"+url.PathEscape(taskKey)+"/manual-retry", nil,
		struct {
			RequestID uint64 `json:"retryRequestId"`
		}{requestID})
	if status == http.StatusNotFound {
		return URLImportTask{}, &StorageError{Code: "configuration"}
	}
	if err != nil {
		return URLImportTask{}, err
	}
	return decodeURLImportTask(response.Data, taskKey)
}

func (c Client) requestURLImportTask(ctx context.Context, method, taskKey, suffix string) (URLImportTask, error) {
	if !importTaskKeyPattern.MatchString(taskKey) {
		return URLImportTask{}, &StorageError{Code: "rejected"}
	}
	response, status, err := c.importAPI(ctx, method, "/api/v1/url-imports/"+url.PathEscape(taskKey)+suffix, nil, nil)
	if status == 404 || response.Code == 404 {
		return URLImportTask{}, ErrImportTaskNotFound
	}
	if err != nil {
		return URLImportTask{}, err
	}
	return decodeURLImportTask(response.Data, taskKey)
}

func validatedTaskImport(taskKey, originURL, storagePath, originalFilename string) (any, error) {
	if !importTaskKeyPattern.MatchString(taskKey) || strings.ContainsAny(originURL, "\r\n\x00") {
		return nil, &StorageError{Code: "rejected"}
	}
	originURL = strings.TrimSpace(originURL)
	parsed, err := url.Parse(originURL)
	if err != nil || parsed == nil || parsed.Opaque != "" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" ||
		!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") || len(originURL) > maxImportURLBytes {
		return nil, &StorageError{Code: "rejected"}
	}
	storagePath, err = validatedImportLookupPath(storagePath, originalFilename)
	if err != nil {
		return nil, err
	}
	return struct {
		TaskKey          string `json:"taskKey"`
		URL              string `json:"url"`
		Path             string `json:"path"`
		OriginalFilename string `json:"originalFilename"`
	}{taskKey, originURL, storagePath, strings.TrimSpace(originalFilename)}, nil
}

func validatedImportLookupPath(storagePath, originalFilename string) (string, error) {
	if strings.ContainsAny(storagePath, "\\\r\n\x00") || strings.ContainsAny(originalFilename, "/\\\r\n\x00") {
		return "", &StorageError{Code: "rejected"}
	}
	storagePath = strings.TrimLeft(strings.TrimSpace(storagePath), "/")
	originalFilename = strings.TrimSpace(originalFilename)
	if storagePath == "" || len(storagePath) > maxStoragePathBytes || strings.TrimSpace(originalFilename) == "" ||
		len(originalFilename) > maxStorageFilenameBytes || originalFilename == "." || originalFilename == ".." {
		return "", &StorageError{Code: "rejected"}
	}
	for _, segment := range strings.Split(storagePath, "/") {
		if strings.TrimSpace(segment) == "." || strings.TrimSpace(segment) == ".." {
			return "", &StorageError{Code: "rejected"}
		}
	}
	if !strings.HasSuffix(storagePath, "/") {
		storagePath += "/"
	}
	if len(storagePath) > maxStoragePathBytes {
		return "", &StorageError{Code: "rejected"}
	}
	return storagePath, nil
}

func decodeURLImportTask(data json.RawMessage, expectedKey string) (URLImportTask, error) {
	var task URLImportTask
	invalid := func() (URLImportTask, error) { return URLImportTask{}, &StorageError{Code: "response"} }
	if json.Unmarshal(data, &task) != nil || task.TaskKey != expectedKey || task.Attempts < 0 {
		return invalid()
	}
	switch task.State {
	case "queued", "running", "uncertain":
		if task.CanRetry || task.Result != nil {
			return invalid()
		}
	case "failed":
		if task.Result != nil {
			return invalid()
		}
	case "succeeded":
		if task.CanRetry || task.Result == nil {
			return invalid()
		}
		result := task.Result
		if !validImportedMetadata(*result) {
			return invalid()
		}
		parsed, err := url.Parse(result.URL)
		if err != nil || parsed.User != nil || parsed.Fragment != "" {
			return invalid()
		}
		if parsed.Scheme == "" {
			// Private storage can return its durable object path in both URL fields.
			// Retain it for authenticated download/signing, never as a browser URL.
			locator := strings.TrimSpace(result.URL)
			if parsed.Host != "" || parsed.RawQuery != "" || parsed.Path == "" ||
				strings.ContainsAny(parsed.Path, "\\\r\n\x00") ||
				(result.RawURL != "" && strings.TrimSpace(result.RawURL) != locator) {
				return invalid()
			}
			for _, segment := range strings.Split(parsed.Path, "/") {
				if strings.TrimSpace(segment) == "." || strings.TrimSpace(segment) == ".." {
					return invalid()
				}
			}
			result.RawURL = locator
			result.URL = ""
		} else {
			if parsed.Hostname() == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
				return invalid()
			}
			result.URL = normalizePublicURL(result.URL)
			if result.RawURL == "" {
				result.RawURL = result.URL
			}
		}
	default:
		return invalid()
	}
	switch task.ErrorCode {
	case "", "import_source_too_large", "import_source_unavailable", "import_source_rejected", "import_configuration_invalid", "import_outcome_unknown", "import_worker_lost",
		"import_cloud_disabled", "import_cloud_unavailable", "import_cloud_source_rejected", "import_cloud_transfer_aborted",
		"import_cloud_source_too_large", "import_cloud_source_dns_unavailable", "import_cloud_source_resume_unavailable",
		"import_cloud_source_unavailable", "import_cloud_source_incomplete", "import_cloud_source_slow",
		"import_cloud_source_changed", "import_cloud_source_range_invalid", "import_cloud_source_retries_exhausted",
		"import_cloud_transfer_deadline", "import_cloud_destination_rejected", "import_cloud_storage_unavailable",
		"import_cloud_part_checksum_mismatch", "import_cloud_configuration_invalid":
	default:
		task.ErrorCode = "import_outcome_unknown"
	}
	return task, nil
}

func validImportedMetadata(result UploadResult) bool {
	locator := result.StorageLocator()
	return locator != "" && len(locator) <= 2048 && !strings.ContainsAny(result.RawURL+result.URL, "\r\n\x00") && result.Size > 0 &&
		result.SHA256() != "" && strings.TrimSpace(result.ContentType) != "" && len(result.ContentType) <= 128 &&
		!strings.ContainsAny(result.ContentType, "\r\n\x00") &&
		strings.TrimSpace(result.Platform) != "" && len(result.Platform) <= 255 &&
		result.Filename != "" && result.Filename != "." && result.Filename != ".." && len(result.Filename) <= maxStorageFilenameBytes && !strings.ContainsAny(result.Filename, "/\\\r\n\x00")
}

// importAPI returns bounded JSON and safe error codes only. It never includes
// a URL, key, response body or unstructured transport error in an error value.
func (c Client) importAPI(ctx context.Context, method, route string, query url.Values, body any) (xfsResponse, int, error) {
	var empty xfsResponse
	if !c.Configured() {
		return empty, 0, &StorageError{Code: "permission"}
	}
	endpoint := strings.TrimRight(c.config.BaseURL, "/") + route
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return empty, 0, &StorageError{Code: "response"}
		}
	}
	requestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, method, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return empty, 0, &StorageError{Code: "locator"}
	}
	req.Header.Set("X-Api-Key", c.config.APIKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := xfsHTTPClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return empty, 0, ctx.Err()
		}
		return empty, 0, ClassifyError(err)
	}
	defer response.Body.Close()
	status := response.StatusCode
	if status < 200 || status >= 300 {
		return empty, status, HTTPError(status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxXFSResponseBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return empty, status, ctx.Err()
		}
		return empty, status, ClassifyError(err)
	}
	if int64(len(data)) > maxXFSResponseBytes || json.Unmarshal(data, &empty) != nil || empty.Code == 0 {
		return xfsResponse{}, status, &StorageError{Code: "response"}
	}
	if empty.Code != 200 {
		return empty, status, storageAPIError(empty.Code, empty.Message)
	}
	return empty, status, nil
}
