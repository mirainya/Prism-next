package filestorage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mirainya/Prism/pkg/config"
)

const (
	managedImportTestKey  = "prism-78034fab-1ed8-4630-96d9-b10cd6650394"
	managedImportTestPath = "prism/gateway-results/17/"
	managedImportTestName = "generated-video.mp4"
)

func managedImportTestResult() UploadResult {
	return UploadResult{
		URL: "http://storage.example/view?signature=test-view-signature", RawURL: managedImportTestPath + "stored.mp4",
		Size: 42, Filename: "stored.mp4", OriginalName: managedImportTestName, Path: managedImportTestPath,
		ContentType: "video/mp4", Platform: "test-platform", HashInfo: UploadHashInfo{"SHA-256": strings.Repeat("a", 64)},
	}
}

func writeManagedImportResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": data})
}

func requireManagedImportError(t *testing.T, err error, code string) {
	t.Helper()
	var storage *StorageError
	if !errors.As(err, &storage) || storage.Code != code {
		t.Fatalf("error=%v, want storage code %s", err, code)
	}
	for _, sensitive := range []string{"test-origin-signature", "bound-storage-key", "test-view-signature", "provider.example"} {
		if strings.Contains(err.Error(), sensitive) {
			t.Fatal("error contains sensitive data")
		}
	}
}

func TestManagedURLImportBindingFingerprint(t *testing.T) {
	client := WithBaseURLAndAPIKey("https://storage.example/", "bound-storage-key")
	digest := client.URLImportBindingSHA256()
	if len(digest) != 64 || digest != WithBaseURLAndAPIKey("https://storage.example", "bound-storage-key").URLImportBindingSHA256() {
		t.Fatal("binding identity is not stable")
	}
	for _, changed := range []Client{
		WithBaseURLAndAPIKey("https://storage.example", "different-test-key"),
		WithBaseURLAndAPIKey("https://another-storage.example", "bound-storage-key"),
	} {
		if changed.URLImportBindingSHA256() == digest {
			t.Fatal("changed endpoint or credential retained the binding identity")
		}
	}
	for _, unconfigured := range []Client{WithBaseURLAndAPIKey("", "key"), WithBaseURLAndAPIKey("https://storage.example", "")} {
		if unconfigured.URLImportBindingSHA256() != "" {
			t.Fatal("unconfigured client has an import binding")
		}
	}
}

func TestManagedURLImportRequestsKeepBoundClientAndIdentity(t *testing.T) {
	var globalCalls, boundCalls atomic.Int32
	global := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		globalCalls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer global.Close()
	bound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		boundCalls.Add(1)
		if r.Header.Get("X-Api-Key") != "bound-storage-key" {
			t.Error("request did not use the bound storage credential")
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/url-imports" {
			writeManagedImportResponse(w, map[string]int{"schemaVersion": 1})
			return
		}
		task := URLImportTask{TaskKey: managedImportTestKey, State: "queued"}
		switch r.Method + " " + r.URL.Path {
		case "POST /api/v1/url-imports":
			var body map[string]string
			if json.NewDecoder(r.Body).Decode(&body) != nil || !reflect.DeepEqual(body, map[string]string{
				"taskKey": managedImportTestKey, "url": "https://provider.example/video.mp4?signature=test-origin-signature",
				"path": managedImportTestPath, "originalFilename": managedImportTestName,
			}) || r.Header.Get("Content-Type") != "application/json" {
				t.Error("submission changed the import identity or target")
			}
		case "GET /api/v1/url-imports/" + managedImportTestKey:
			task.State = "running"
		case "POST /api/v1/url-imports/" + managedImportTestKey + "/retry":
		default:
			t.Error("unexpected endpoint: " + r.Method + " " + r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		writeManagedImportResponse(w, task)
	}))
	defer bound.Close()
	previous := config.C
	config.C = &config.Config{FileStorage: config.FileStorageConfig{BaseURL: global.URL, APIKey: "global-test-key"}}
	t.Cleanup(func() { config.C = previous })
	client := WithBaseURLAndAPIKey(bound.URL, "bound-storage-key")
	if supported, err := client.SupportsURLImportTasks(context.Background()); err != nil || !supported {
		t.Fatalf("capability supported=%v error=%v", supported, err)
	}
	for range 2 {
		task, err := client.CreateURLImportTask(context.Background(), managedImportTestKey,
			" https://provider.example/video.mp4?signature=test-origin-signature ", "/"+managedImportTestPath, " "+managedImportTestName+" ")
		if err != nil || task.State != "queued" || task.TaskKey != managedImportTestKey {
			t.Fatalf("create state=%s error=%v", task.State, err)
		}
	}
	if task, err := client.ReadURLImportTask(context.Background(), managedImportTestKey); err != nil || task.State != "running" {
		t.Fatalf("read state=%s error=%v", task.State, err)
	}
	if task, err := client.RetryURLImportTask(context.Background(), managedImportTestKey); err != nil || task.State != "queued" {
		t.Fatalf("retry state=%s error=%v", task.State, err)
	}
	if globalCalls.Load() != 0 || boundCalls.Load() != 5 {
		t.Fatalf("global requests=%d bound requests=%d", globalCalls.Load(), boundCalls.Load())
	}
}

func TestManagedURLImportCapabilityProbe(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		supported  bool
		wantError  bool
	}{
		{"v1", `{"code":200,"data":{"schemaVersion":1}}`, 200, true, false},
		{"disabled", "not found", 404, false, false},
		{"old_server", "not allowed", 405, false, false},
		{"unimplemented", "not implemented", 501, false, false},
		{"application_not_found", `{"code":404}`, 200, false, false},
		{"unauthorized", "bound-storage-key", 401, false, true},
		{"unavailable", "test-origin-signature", 503, false, true},
		{"future_version", `{"code":200,"data":{"schemaVersion":2}}`, 200, false, true},
		{"missing_version", `{"code":200,"data":{}}`, 200, false, true},
		{"malformed", `{"code":200,"data":`, 200, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/url-imports" {
					t.Error("probe attempted a write")
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			supported, err := WithBaseURLAndAPIKey(server.URL, "bound-storage-key").SupportsURLImportTasks(context.Background())
			if supported != tc.supported || (err != nil) != tc.wantError {
				t.Fatalf("supported=%v error=%v", supported, err)
			}
		})
	}
}

func TestManagedURLImportDecodeStates(t *testing.T) {
	for _, state := range []string{"queued", "running", "uncertain", "failed", "succeeded"} {
		t.Run(state, func(t *testing.T) {
			task := URLImportTask{TaskKey: managedImportTestKey, State: state, Attempts: 2}
			if state == "failed" {
				task.CanRetry, task.ErrorCode = true, "import_source_unavailable"
			}
			if state == "succeeded" {
				result := managedImportTestResult()
				task.Result = &result
			}
			encoded, _ := json.Marshal(task)
			got, err := decodeURLImportTask(encoded, managedImportTestKey)
			if err != nil || got.State != state || got.CanRetry != task.CanRetry {
				t.Fatalf("state=%s error=%v", got.State, err)
			}
			if got.Result != nil && (got.Result.URL != "https://storage.example/view?signature=test-view-signature" || got.Result.RawURL != task.Result.RawURL) {
				t.Fatal("success lost the browser URL or durable locator")
			}
			if strings.Contains(fmt.Sprint(got), "signature") || strings.Contains(fmt.Sprintf("%+v", got), "signature") {
				t.Fatal("task diagnostics expose a signed URL")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		mutate func(*URLImportTask)
	}{
		{"wrong_identity", func(v *URLImportTask) { v.TaskKey = "another-task" }},
		{"unknown_state", func(v *URLImportTask) { v.State = "completed" }},
		{"negative_attempts", func(v *URLImportTask) { v.Attempts = -1 }},
		{"pending_retry", func(v *URLImportTask) { v.State, v.CanRetry, v.Result = "queued", true, nil }},
		{"running_result", func(v *URLImportTask) { v.State = "running" }},
		{"uncertain_retry", func(v *URLImportTask) { v.State, v.CanRetry, v.Result = "uncertain", true, nil }},
		{"failed_result", func(v *URLImportTask) { v.State = "failed" }},
		{"success_retry", func(v *URLImportTask) { v.CanRetry = true }},
		{"success_no_result", func(v *URLImportTask) { v.Result = nil }},
		{"zero_size", func(v *URLImportTask) { v.Result.Size = 0 }},
		{"invalid_digest", func(v *URLImportTask) { v.Result.HashInfo = UploadHashInfo{"sha256": "bad"} }},
		{"no_platform", func(v *URLImportTask) { v.Result.Platform = "" }},
		{"no_mime", func(v *URLImportTask) { v.Result.ContentType = "" }},
		{"bad_filename", func(v *URLImportTask) { v.Result.Filename = "../object.mp4" }},
		{"relative_view", func(v *URLImportTask) { v.Result.URL = "objects/object.mp4" }},
		{"unsafe_view", func(v *URLImportTask) { v.Result.URL = "file:///tmp/object.mp4" }},
		{"view_userinfo", func(v *URLImportTask) { v.Result.URL = "https://user:password@storage.example/object" }},
		{"view_fragment", func(v *URLImportTask) { v.Result.URL += "#fragment" }},
		{"locator_injection", func(v *URLImportTask) { v.Result.RawURL += "\nignored" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := managedImportTestResult()
			task := URLImportTask{TaskKey: managedImportTestKey, State: "succeeded", Result: &result}
			tc.mutate(&task)
			encoded, _ := json.Marshal(task)
			_, err := decodeURLImportTask(encoded, managedImportTestKey)
			requireManagedImportError(t, err, "response")
		})
	}
	for _, data := range []string{`null`, `{`, `{"state":"queued"}`, `[]`} {
		_, err := decodeURLImportTask([]byte(data), managedImportTestKey)
		requireManagedImportError(t, err, "response")
	}
	got, err := decodeURLImportTask([]byte(`{"taskKey":"`+managedImportTestKey+`","state":"uncertain","errorCode":"test-origin-signature"}`), managedImportTestKey)
	if err != nil || got.ErrorCode != "import_outcome_unknown" {
		t.Fatalf("unknown error code was not sanitized: %v", err)
	}
}

func TestManagedURLImportPrivateObjectLocator(t *testing.T) {
	for _, rawURL := range []string{"", managedImportTestPath + "stored.mp4"} {
		t.Run(fmt.Sprintf("explicit_locator_%t", rawURL != ""), func(t *testing.T) {
			result := managedImportTestResult()
			result.URL, result.RawURL = managedImportTestPath+"stored.mp4", rawURL
			encoded, _ := json.Marshal(URLImportTask{
				TaskKey: managedImportTestKey, State: "succeeded", Attempts: 1, Result: &result,
			})
			task, err := decodeURLImportTask(encoded, managedImportTestKey)
			if err != nil {
				t.Fatalf("private storage success rejected: %v", err)
			}
			if task.Result.URL != "" || task.Result.StorageLocator() != result.URL ||
				task.Result.Path != result.Path || task.Result.OriginalName != result.OriginalName ||
				task.Result.SHA256() != result.SHA256() {
				t.Fatal("private object identity or metadata changed")
			}
		})
	}
	for _, locator := range []string{
		"//other.example/object.mp4", "objects/../object.mp4", "objects/%2e%2e/object.mp4",
		"objects/%5cobject.mp4", "objects/%0aobject.mp4", "objects/object.mp4?signature=value",
		"objects/object.mp4#fragment", "file:///tmp/object.mp4", "javascript:alert(1)",
	} {
		t.Run(locator, func(t *testing.T) {
			result := managedImportTestResult()
			result.URL, result.RawURL = locator, locator
			encoded, _ := json.Marshal(URLImportTask{TaskKey: managedImportTestKey, State: "succeeded", Result: &result})
			_, err := decodeURLImportTask(encoded, managedImportTestKey)
			requireManagedImportError(t, err, "response")
		})
	}
}

func TestManagedURLImportCreateChecksReturnedTarget(t *testing.T) {
	for _, field := range []string{"valid", "private_locator", "path", "originalFilename"} {
		t.Run(field, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				result := managedImportTestResult()
				if field == "private_locator" {
					result.URL = result.RawURL
				} else if field == "path" {
					result.Path = "another-attempt/"
				} else if field == "originalFilename" {
					result.OriginalName = "another-file.mp4"
				}
				writeManagedImportResponse(w, URLImportTask{TaskKey: managedImportTestKey, State: "succeeded", Result: &result})
			}))
			defer server.Close()
			got, err := WithBaseURLAndAPIKey(server.URL, "bound-storage-key").CreateURLImportTask(context.Background(), managedImportTestKey,
				"https://provider.example/video.mp4", "/"+managedImportTestPath, " "+managedImportTestName+" ")
			if field == "valid" || field == "private_locator" {
				if err != nil || got.State != "succeeded" || got.Result == nil || got.Result.Path != managedImportTestPath {
					t.Fatalf("valid success was not accepted: %v", err)
				}
				return
			}
			requireManagedImportError(t, err, "response")
		})
	}
}

func TestManagedURLImportRejectsInvalidInputBeforeRequest(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	client := WithBaseURLAndAPIKey(server.URL, "bound-storage-key")
	for _, tc := range []struct{ key, source, path, name string }{
		{"", "https://provider.example/video", "files/", "video.mp4"},
		{"../task", "https://provider.example/video", "files/", "video.mp4"},
		{"task", "file:///tmp/video", "files/", "video.mp4"},
		{"task", "https://user:password@provider.example/video", "files/", "video.mp4"},
		{"task", "https://provider.example/video#fragment", "files/", "video.mp4"},
		{"task", "https://provider.example/video\n", "files/", "video.mp4"},
		{"task", "https://provider.example/" + strings.Repeat("a", maxImportURLBytes), "files/", "video.mp4"},
		{"task", "https://provider.example/video", "files/../private", "video.mp4"},
		{"task", "https://provider.example/video", "files\\private", "video.mp4"},
		{"task", "https://provider.example/video", "/", "video.mp4"},
		{"task", "https://provider.example/video", strings.Repeat("a", maxStoragePathBytes), "video.mp4"},
		{"task", "https://provider.example/video", "files/", "../video.mp4"},
		{"task", "https://provider.example/video", "files/", "video.mp4\x00"},
		{"task", "https://provider.example/video", "files/", " "},
	} {
		_, err := client.CreateURLImportTask(context.Background(), tc.key, tc.source, tc.path, tc.name)
		requireManagedImportError(t, err, "rejected")
	}
	for _, method := range []func(context.Context, string) (URLImportTask, error){client.ReadURLImportTask, client.RetryURLImportTask} {
		_, err := method(context.Background(), "../task")
		requireManagedImportError(t, err, "rejected")
	}
	if calls.Load() != 0 {
		t.Fatal("invalid input reached storage")
	}
}

func TestManagedURLImportLostResponseDoesNotFallBack(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/url-imports" {
			t.Error("response loss triggered another upload endpoint")
			w.WriteHeader(400)
			return
		}
		if calls.Add(1) == 1 {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close() // Submission accepted, acknowledgement lost.
			return
		}
		writeManagedImportResponse(w, URLImportTask{TaskKey: managedImportTestKey, State: "queued"})
	}))
	defer server.Close()
	client := WithBaseURLAndAPIKey(server.URL, "bound-storage-key")
	for attempt := range 2 {
		task, err := client.CreateURLImportTask(context.Background(), managedImportTestKey,
			"https://provider.example/video", managedImportTestPath, managedImportTestName)
		if attempt == 0 {
			requireManagedImportError(t, err, "network")
			if calls.Load() != 1 {
				t.Fatal("client automatically repeated a submission")
			}
		} else if err != nil || task.State != "queued" {
			t.Fatalf("same-identity resubmission failed: %v", err)
		}
	}
	if calls.Load() != 2 {
		t.Fatal("unexpected automatic request")
	}
}

type managedImportRoundTripper func(*http.Request) (*http.Response, error)

func (f managedImportRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestManagedURLImportTransportErrorsAndCancellation(t *testing.T) {
	previous := xfsHTTPClient
	t.Cleanup(func() { xfsHTTPClient = previous })
	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{"timeout", fmt.Errorf("test-origin-signature: %w", context.DeadlineExceeded), "timeout"},
		{"connection_lost", fmt.Errorf("bound-storage-key: %w", io.ErrUnexpectedEOF), "network"},
		{"wrapped_transport_error", errors.New("https://provider.example/?signature=test-origin-signature"), "network"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			xfsHTTPClient = &http.Client{Transport: managedImportRoundTripper(func(r *http.Request) (*http.Response, error) {
				calls++
				deadline, ok := r.Context().Deadline()
				if !ok || time.Until(deadline) > 15*time.Second || r.URL.Path != "/api/v1/url-imports" {
					t.Error("request has no bounded deadline or changed endpoint")
				}
				return nil, tc.err
			})}
			_, err := WithBaseURLAndAPIKey("https://storage.example", "bound-storage-key").CreateURLImportTask(context.Background(),
				managedImportTestKey, "https://provider.example/video", managedImportTestPath, managedImportTestName)
			requireManagedImportError(t, err, tc.code)
			if calls != 1 {
				t.Fatal("transport error triggered another request")
			}
		})
	}
	xfsHTTPClient = &http.Client{Transport: managedImportRoundTripper(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err := WithBaseURLAndAPIKey("https://storage.example", "bound-storage-key").ReadURLImportTask(ctx, managedImportTestKey)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("caller cancellation was lost: %v", err)
	}
}

func TestManagedURLImportRejectsRedirectAndUnboundedResponse(t *testing.T) {
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Store(true) }))
	defer target.Close()
	for _, mode := range []string{"redirect", "large", "invalid_json", "application_error", "not_found", "app_not_found"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch mode {
				case "redirect":
					http.Redirect(w, r, target.URL+"/capture", http.StatusTemporaryRedirect)
				case "large":
					_, _ = io.WriteString(w, strings.Repeat("a", int(maxXFSResponseBytes)+1))
				case "invalid_json":
					_, _ = io.WriteString(w, `{"code":200,"data":test-origin-signature`)
				case "application_error":
					_, _ = io.WriteString(w, `{"code":500,"message":"bound-storage-key https://provider.example/test-origin-signature"}`)
				case "not_found":
					w.WriteHeader(404)
				case "app_not_found":
					_, _ = io.WriteString(w, `{"code":404}`)
				}
			}))
			defer server.Close()
			_, err := WithBaseURLAndAPIKey(server.URL, "bound-storage-key").ReadURLImportTask(context.Background(), managedImportTestKey)
			if mode == "not_found" || mode == "app_not_found" {
				if !errors.Is(err, ErrImportTaskNotFound) {
					t.Fatalf("not found error=%v", err)
				}
				return
			}
			code := "response"
			if mode == "redirect" {
				code = "http"
			} else if mode == "application_error" {
				code = "service"
			}
			requireManagedImportError(t, err, code)
		})
	}
	if redirected.Load() {
		t.Fatal("storage credential followed a redirect")
	}
}
