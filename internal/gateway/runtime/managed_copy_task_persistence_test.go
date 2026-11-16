package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/pkg/filestorage"
)

func TestManagedImportKeepsConfirmedObjectWhenLocalRecordFails(t *testing.T) {
	for _, phase := range []string{"reserve", "record"} {
		t.Run(phase, func(t *testing.T) {
			service, mock := runtimeImportService(t)
			restore := stubManagedStorage(t)
			defer restore()
			source := runtimeImportSource()
			job := runtimeImportJob(source)
			expectManagedMissing(mock, 2, "gateway-result:2:3")
			expectRuntimeImportRead(mock, job)
			dbFailure := errors.New("simulated local write failure")
			if phase == "reserve" {
				mock.ExpectBegin()
				mock.ExpectQuery("SELECT c.id,c.user_id,c.token_id FROM gw_api_call_attempts").WillReturnError(dbFailure)
				mock.ExpectRollback()
			} else {
				expectManagedAllocation(mock, 2, 1, 10, 20, "gateway-result:2:3", 31)
				mock.ExpectBegin()
				mock.ExpectQuery("SELECT state,storage_locator FROM gw_media_assets").WithArgs(uint64(31)).
					WillReturnRows(sqlmock.NewRows([]string{"state", "storage_locator"}).AddRow("staging", nil))
				mock.ExpectExec("UPDATE gw_media_assets SET storage_locator").WillReturnError(dbFailure)
				mock.ExpectRollback()
			}
			readManagedImport = func(context.Context, repository.TokenFileStorage, string) (filestorage.URLImportTask, error) {
				return filestorage.URLImportTask{TaskKey: job.TaskKey, State: "succeeded", Result: &filestorage.UploadResult{
					Path: job.StoragePath, OriginalName: job.OriginalFilename, Filename: "stored.mp4",
					RawURL: "objects/original.mp4", Size: 42, ContentType: "video/mp4", HashInfo: filestorage.UploadHashInfo{"sha256": job.SourceSHA256},
				}}, nil
			}
			deleteManagedResult = func(context.Context, repository.TokenFileStorage, string) error {
				t.Fatal("local database failure deleted a confirmed asynchronous import")
				return nil
			}
			_, err := service.prepareManagedURLCopyMode(context.Background(), runtimeImportStorage(), 2, 3, source, 100, true)
			if !errors.Is(err, dbFailure) {
				t.Fatalf("local failure error=%v", err)
			}
		})
	}
}

func TestManagedImportProbeFailureRetainsUnsentIdentityForRecovery(t *testing.T) {
	service, mock := runtimeImportService(t)
	restore := stubManagedStorage(t)
	defer restore()
	source := runtimeImportSource()
	job := runtimeImportJob(source)
	job.Mode = repository.URLImportDetect
	expectRuntimeImportRead(mock, job)
	probeManagedImports = func(context.Context, repository.TokenFileStorage) (bool, error) {
		return false, &filestorage.StorageError{Code: "service", Retryable: true}
	}
	_, err := service.importManagedPublicResult(context.Background(), runtimeImportStorage(), 2, 3, source, true)
	if code := filestorage.ClassifyError(err).Code; code != "service" {
		t.Fatalf("probe failure code=%s", code)
	}
	expectRuntimeImportRead(mock, job)
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE gw_result_import_jobs SET import_mode=").WithArgs(repository.URLImportAsyncV1, uint64(2), uint32(3), job.TaskKey).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT import_mode,legacy_started FROM gw_result_import_jobs").WithArgs(uint64(2), uint32(3), job.TaskKey).
		WillReturnRows(sqlmock.NewRows([]string{"mode", "started"}).AddRow(repository.URLImportAsyncV1, false))
	mock.ExpectCommit()
	probeManagedImports = func(context.Context, repository.TokenFileStorage) (bool, error) { return true, nil }
	readManagedImport = func(_ context.Context, _ repository.TokenFileStorage, key string) (filestorage.URLImportTask, error) {
		if key != job.TaskKey {
			t.Fatal("probe recovery replaced the unsent identity")
		}
		return filestorage.URLImportTask{TaskKey: key, State: "running"}, nil
	}
	_, err = service.importManagedPublicResult(context.Background(), runtimeImportStorage(), 2, 3, source, true)
	if code := filestorage.ClassifyError(err).Code; code != "pending" {
		t.Fatalf("recovered probe code=%s", code)
	}
}

func TestManagedImportInitialCallWaitsForSameTaskCompletion(t *testing.T) {
	restore := stubManagedStorage(t)
	defer restore()
	job := runtimeImportJob(runtimeImportSource())
	queries := 0
	readManagedImport = func(_ context.Context, _ repository.TokenFileStorage, key string) (filestorage.URLImportTask, error) {
		queries++
		if key != job.TaskKey {
			t.Fatal("wait queried another task")
		}
		if queries == 1 {
			return filestorage.URLImportTask{TaskKey: key, State: "running"}, nil
		}
		return filestorage.URLImportTask{TaskKey: key, State: "succeeded", Result: &filestorage.UploadResult{
			Path: job.StoragePath, OriginalName: job.OriginalFilename, RawURL: "objects/final.mp4",
		}}, nil
	}
	result, err := awaitManagedImportTask(context.Background(), runtimeImportStorage(), job, runtimeImportSource().URL)
	if err != nil || result.RawURL != "objects/final.mp4" || queries != 2 {
		t.Fatalf("synchronous result was lost: queries=%d error=%v", queries, err)
	}
	// Preserve the time reserved for committing the provider result.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(10*time.Second))
	defer cancel()
	_, err = awaitManagedImportTask(ctx, runtimeImportStorage(), job, runtimeImportSource().URL)
	if code := filestorage.ClassifyError(err).Code; code != "pending" || queries != 2 {
		t.Fatalf("expired wait caused another request: queries=%d error=%v", queries, err)
	}
}

func TestManagedImportRecoversHTTPAcknowledgementLossWithoutAnotherUpload(t *testing.T) {
	service, mock := runtimeImportService(t)
	source := runtimeImportSource()
	job := runtimeImportJob(source)
	var submissions atomic.Int32
	var reads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != testStorageAPIKey {
			t.Error("task used another storage credential")
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/url-imports/" + job.TaskKey:
			if reads.Add(1) == 1 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": filestorage.URLImportTask{
				TaskKey: job.TaskKey, State: "succeeded", Attempts: 1, Result: &filestorage.UploadResult{
					Path: job.StoragePath, OriginalName: job.OriginalFilename, Filename: "saved.mp4", Platform: "test-platform",
					URL: "https://storage.example/saved.mp4", RawURL: "objects/saved.mp4", Size: 42,
					ContentType: "video/mp4", HashInfo: filestorage.UploadHashInfo{"sha256": job.SourceSHA256},
				},
			}})
		case "POST /api/v1/url-imports":
			submissions.Add(1)
			var body map[string]string
			if json.NewDecoder(r.Body).Decode(&body) != nil || body["taskKey"] != job.TaskKey || body["url"] != source.URL {
				t.Error("submission identity changed")
			}
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
		default:
			t.Error("recovery reached an unexpected endpoint")
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	storage := runtimeImportStorage()
	storage.XFSBaseURL = server.URL
	job.StorageBindingSHA256 = managedImportClient(storage).URLImportBindingSHA256()
	expectRuntimeImportRead(mock, job)
	_, err := service.importManagedPublicResult(context.Background(), storage, 2, 3, source, true)
	if code := filestorage.ClassifyError(err).Code; code != "network" {
		t.Fatalf("lost acknowledgement code=%s", code)
	}
	expectRuntimeImportRead(mock, job)
	result, err := service.importManagedPublicResult(context.Background(), storage, 2, 3, source, true)
	if err != nil || result.RawURL != "objects/saved.mp4" || reads.Load() != 2 || submissions.Load() != 1 {
		t.Fatalf("HTTP recovery duplicated an upload: reads=%d submissions=%d error=%v", reads.Load(), submissions.Load(), err)
	}
}
