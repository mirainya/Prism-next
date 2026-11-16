package runtime

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/delivery"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/pkg/filestorage"
)

const runtimeImportColumns = "attempt_id,result_ordinal,import_mode,task_key,storage_path,original_filename,source_sha256,storage_binding_sha256,legacy_started,created_at"

func runtimeImportStorage() repository.TokenFileStorage {
	return repository.TokenFileStorage{UserID: 10, TokenID: 20, XFSBaseURL: testStorageBaseURL, APIKey: testStorageAPIKey}
}

func runtimeImportSource() delivery.RemoteResult {
	return delivery.RemoteResult{Role: "video", URL: "https://provider.example/video.mp4?signature=test-signed-source"}
}

func runtimeImportJob(source delivery.RemoteResult) repository.ManagedURLImport {
	return repository.ManagedURLImport{
		AttemptID: 2, Ordinal: 3, Mode: repository.URLImportAsyncV1, TaskKey: "prism-78034fab-1ed8-4630-96d9-b10cd6650394",
		StoragePath: managedResultImportStoragePath(2, 3), OriginalFilename: managedResultImportFilename(source),
		SourceSHA256: managedImportSourceDigest(source), StorageBindingSHA256: managedImportClient(runtimeImportStorage()).URLImportBindingSHA256(),
		CreatedAt: time.Now().UTC(),
	}
}

func runtimeImportRows(job repository.ManagedURLImport) *sqlmock.Rows {
	return sqlmock.NewRows(strings.Split(runtimeImportColumns, ",")).AddRow(
		job.AttemptID, job.Ordinal, job.Mode, job.TaskKey, job.StoragePath, job.OriginalFilename,
		job.SourceSHA256, job.StorageBindingSHA256, job.LegacyStarted, job.CreatedAt,
	)
}

func runtimeImportService(t *testing.T) (*Service, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store, _ := repository.New(db)
	service, _ := New(store)
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		_ = db.Close()
	})
	return service, mock
}

func expectRuntimeImportRead(mock sqlmock.Sqlmock, job repository.ManagedURLImport) {
	mock.ExpectQuery("SELECT "+runtimeImportColumns+" FROM gw_result_import_jobs").WithArgs(job.AttemptID, job.Ordinal).
		WillReturnRows(runtimeImportRows(job))
}

func expectRuntimeImportReservation(mock sqlmock.Sqlmock, attemptID uint64, ordinal uint32, source delivery.RemoteResult, mode string, historical bool) {
	mock.ExpectQuery("SELECT "+runtimeImportColumns+" FROM gw_result_import_jobs").WithArgs(attemptID, ordinal).WillReturnError(sql.ErrNoRows)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM gw_api_call_attempts WHERE id=\\? FOR UPDATE").WithArgs(attemptID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(attemptID))
	mock.ExpectQuery("SELECT "+runtimeImportColumns+" FROM gw_result_import_jobs").WithArgs(attemptID, ordinal).WillReturnError(sql.ErrNoRows)
	initialMode := repository.URLImportDetect
	if historical {
		initialMode = repository.URLImportLegacy
	}
	mock.ExpectExec("INSERT INTO gw_result_import_jobs").WithArgs(attemptID, ordinal, initialMode, sqlmock.AnyArg(),
		managedResultImportStoragePath(attemptID, ordinal), managedResultImportFilename(source), managedImportSourceDigest(source),
		managedImportClient(runtimeImportStorage()).URLImportBindingSHA256(), historical, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if !historical {
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE gw_result_import_jobs SET import_mode=").WithArgs(mode, attemptID, ordinal, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery("SELECT import_mode,legacy_started FROM gw_result_import_jobs").WithArgs(attemptID, ordinal, sqlmock.AnyArg()).
			WillReturnRows(sqlmock.NewRows([]string{"mode", "started"}).AddRow(mode, false))
		mock.ExpectCommit()
	}
}

func expectRuntimeLegacyClaim(mock sqlmock.Sqlmock, attemptID uint64, ordinal uint32, changed int64) {
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE gw_result_import_jobs SET legacy_started=1").WithArgs(attemptID, ordinal, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, changed))
	mock.ExpectCommit()
}

func TestManagedImportPersistsBeforeSubmissionAndReusesTaskAfterResponseLoss(t *testing.T) {
	service, mock := runtimeImportService(t)
	restore := stubManagedStorage(t)
	defer restore()
	source := runtimeImportSource()
	expectRuntimeImportReservation(mock, 2, 3, source, repository.URLImportAsyncV1, false)
	probeManagedImports = func(context.Context, repository.TokenFileStorage) (bool, error) { return true, nil }
	var savedKey string
	readManagedImport = func(_ context.Context, storage repository.TokenFileStorage, key string) (filestorage.URLImportTask, error) {
		if storage.APIKey != testStorageAPIKey || storage.XFSBaseURL != testStorageBaseURL {
			t.Fatal("wrong storage binding")
		}
		if savedKey != "" && key != savedKey {
			t.Fatal("response loss changed the task identity")
		}
		savedKey = key
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("external request preceded committed identity: %v", err)
		}
		return filestorage.URLImportTask{}, filestorage.ErrImportTaskNotFound
	}
	createManagedImport = func(_ context.Context, _ repository.TokenFileStorage, job repository.ManagedURLImport, url string) (filestorage.URLImportTask, error) {
		if job.TaskKey != savedKey || url != source.URL || job.StoragePath != managedResultImportStoragePath(2, 3) {
			t.Fatal("submission changed the identity")
		}
		return filestorage.URLImportTask{}, &filestorage.StorageError{Code: "timeout", Retryable: true}
	}
	_, err := service.importManagedPublicResult(context.Background(), runtimeImportStorage(), 2, 3, source, false)
	if filestorage.ClassifyError(err).Code != "timeout" || savedKey == "" {
		t.Fatalf("lost response error=%v", err)
	}
	job := runtimeImportJob(source)
	job.TaskKey = savedKey
	expectRuntimeImportRead(mock, job)
	probeManagedImports = func(context.Context, repository.TokenFileStorage) (bool, error) {
		t.Fatal("existing task probed capabilities again")
		return false, nil
	}
	readManagedImport = func(_ context.Context, _ repository.TokenFileStorage, key string) (filestorage.URLImportTask, error) {
		if key != savedKey {
			t.Fatal("recovery queried another task")
		}
		return filestorage.URLImportTask{TaskKey: key, State: "running"}, nil
	}
	_, err = service.importManagedPublicResult(context.Background(), runtimeImportStorage(), 2, 3, source, true)
	if code := filestorage.ClassifyError(err).Code; code != "pending" {
		t.Fatalf("recovery code=%s", code)
	}
}

func TestManagedImportHistoricalRecoveryOnlyFindsOriginalFile(t *testing.T) {
	for _, found := range []bool{true, false} {
		t.Run(map[bool]string{true: "found", false: "missing"}[found], func(t *testing.T) {
			service, mock := runtimeImportService(t)
			restore := stubManagedStorage(t)
			defer restore()
			source := runtimeImportSource()
			expectRuntimeImportReservation(mock, 2, 3, source, repository.URLImportLegacy, true)
			findManagedImport = func(_ context.Context, storage repository.TokenFileStorage, path, name string) (filestorage.UploadResult, error) {
				if path != managedResultImportStoragePath(2, 3) || name != "video.mp4" || storage.APIKey != testStorageAPIKey {
					t.Fatal("historical lookup lost the original ordinal or binding")
				}
				if !found {
					return filestorage.UploadResult{}, filestorage.ErrImportedFileNotFound
				}
				return filestorage.UploadResult{RawURL: "objects/original.mp4"}, nil
			}
			got, err := service.importManagedPublicResult(context.Background(), runtimeImportStorage(), 2, 3, source, true)
			if found && (err != nil || got.RawURL != "objects/original.mp4") || !found && filestorage.ClassifyError(err).Code != "uncertain" {
				t.Fatalf("historical recovery error=%v", err)
			}
		})
	}
}

func TestManagedImportLegacyTimeoutOrConcurrentClaimNeverRepeatsUpload(t *testing.T) {
	for _, won := range []bool{true, false} {
		t.Run(map[bool]string{true: "lost_acknowledgement", false: "concurrent_claim"}[won], func(t *testing.T) {
			service, mock := runtimeImportService(t)
			restore := stubManagedStorage(t)
			defer restore()
			source := runtimeImportSource()
			job := runtimeImportJob(source)
			job.Mode = repository.URLImportLegacy
			expectRuntimeImportRead(mock, job)
			changed := int64(0)
			if won {
				changed = 1
			}
			expectRuntimeLegacyClaim(mock, 2, 3, changed)
			calls := 0
			importManagedResult = func(context.Context, repository.TokenFileStorage, string, string, string) (filestorage.UploadResult, error) {
				calls++
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatalf("upload preceded committed claim: %v", err)
				}
				return filestorage.UploadResult{}, &filestorage.StorageError{Code: "timeout", Retryable: true}
			}
			findManagedImport = func(context.Context, repository.TokenFileStorage, string, string) (filestorage.UploadResult, error) {
				return filestorage.UploadResult{}, filestorage.ErrImportedFileNotFound
			}
			_, err := service.importManagedPublicResult(context.Background(), runtimeImportStorage(), 2, 3, source, false)
			if code := filestorage.ClassifyError(err).Code; code != "uncertain" || calls != int(changed) {
				t.Fatalf("first calls=%d code=%s", calls, code)
			}
			job.LegacyStarted = true
			expectRuntimeImportRead(mock, job)
			_, err = service.importManagedPublicResult(context.Background(), runtimeImportStorage(), 2, 3, source, true)
			if code := filestorage.ClassifyError(err).Code; code != "uncertain" || calls != int(changed) {
				t.Fatalf("recovery repeated the upload: calls=%d code=%s", calls, code)
			}
		})
	}
}

func TestManagedImportRejectsChangedSourceOrBindingWithoutNetwork(t *testing.T) {
	for _, field := range []string{"source", "binding"} {
		t.Run(field, func(t *testing.T) {
			service, mock := runtimeImportService(t)
			restore := stubManagedStorage(t)
			defer restore()
			source := runtimeImportSource()
			job := runtimeImportJob(source)
			if field == "source" {
				job.SourceSHA256 = strings.Repeat("b", 64)
			} else {
				job.StorageBindingSHA256 = strings.Repeat("b", 64)
			}
			expectRuntimeImportRead(mock, job)
			_, err := service.importManagedPublicResult(context.Background(), runtimeImportStorage(), 2, 3, source, true)
			if code := filestorage.ClassifyError(err).Code; code != "binding" {
				t.Fatalf("changed identity code=%s", code)
			}
		})
	}
}

func TestManagedImportTaskOnlyRetriesConfirmedFailure(t *testing.T) {
	for _, state := range []string{"queued", "running", "uncertain", "failed", "succeeded", "unretryable", "exhausted", "old_running", "old_uncertain", "wrong_target", "disabled_endpoint"} {
		t.Run(state, func(t *testing.T) {
			restore := stubManagedStorage(t)
			defer restore()
			job := runtimeImportJob(runtimeImportSource())
			if strings.HasPrefix(state, "old_") {
				job.CreatedAt = time.Now().Add(-25 * time.Hour)
			}
			retries := 0
			readManagedImport = func(context.Context, repository.TokenFileStorage, string) (filestorage.URLImportTask, error) {
				task := filestorage.URLImportTask{TaskKey: job.TaskKey, State: state}
				switch state {
				case "failed", "unretryable", "exhausted":
					task.State, task.CanRetry, task.Attempts = "failed", state != "unretryable", 1
					if state == "exhausted" {
						task.Attempts = 5
					}
				case "succeeded", "wrong_target":
					task.State = "succeeded"
					task.Result = &filestorage.UploadResult{Path: job.StoragePath, OriginalName: job.OriginalFilename, RawURL: "objects/result.mp4"}
					if state == "wrong_target" {
						task.Result.Path = "another-attempt/"
					}
				case "disabled_endpoint":
					return filestorage.URLImportTask{}, &filestorage.StorageError{Code: "http", HTTPStatus: 503, Retryable: true}
				case "old_running", "old_uncertain":
					task.State = strings.TrimPrefix(state, "old_")
				}
				return task, nil
			}
			retryManagedImport = func(context.Context, repository.TokenFileStorage, string) (filestorage.URLImportTask, error) {
				retries++
				return filestorage.URLImportTask{TaskKey: job.TaskKey, State: "queued"}, nil
			}
			result, err := resolveManagedImportTask(context.Background(), runtimeImportStorage(), job, runtimeImportSource().URL)
			if state == "succeeded" {
				if err != nil || result.RawURL != "objects/result.mp4" {
					t.Fatalf("success error=%v", err)
				}
			} else {
				failure := filestorage.ClassifyError(err)
				wantRetry := state == "queued" || state == "running" || state == "uncertain" || state == "failed" || state == "disabled_endpoint"
				if err == nil || failure.Retryable != wantRetry {
					t.Fatalf("state=%s error=%v retry=%v", state, err, failure.Retryable)
				}
			}
			if (retries == 1) != (state == "failed") {
				t.Fatalf("state=%s retries=%d", state, retries)
			}
		})
	}
}

func TestManagedImportReservationFailureNeverCallsStorage(t *testing.T) {
	service, mock := runtimeImportService(t)
	restore := stubManagedStorage(t)
	defer restore()
	source := runtimeImportSource()
	mock.ExpectQuery("SELECT "+runtimeImportColumns+" FROM gw_result_import_jobs").WithArgs(uint64(2), uint32(3)).WillReturnError(sql.ErrNoRows)
	mock.ExpectBegin()
	dbFailure := errors.New("database temporarily unavailable")
	mock.ExpectQuery("SELECT id FROM gw_api_call_attempts").WithArgs(uint64(2)).WillReturnError(dbFailure)
	mock.ExpectRollback()
	_, err := service.importManagedPublicResult(context.Background(), runtimeImportStorage(), 2, 3, source, true)
	var persistence *managedImportPersistenceError
	if !errors.As(err, &persistence) || !errors.Is(persistence.error, dbFailure) {
		t.Fatalf("reservation failure error=%v", err)
	}
}
