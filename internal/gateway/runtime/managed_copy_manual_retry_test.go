package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/pkg/filestorage"
)

func TestManualImportRetryUsesDurableActionAfterAutomaticExhaustion(t *testing.T) {
	restore := stubManagedStorage(t)
	defer restore()
	job := runtimeImportJob(runtimeImportSource())
	job.CreatedAt = time.Now().Add(-25 * time.Hour)
	intent := managedImportOperatorRetry{ID: 51, RequestedAt: time.Now()}
	ctx := context.WithValue(context.Background(), managedImportOperatorRetryKey{}, intent)
	task := filestorage.URLImportTask{TaskKey: job.TaskKey, State: "failed", Attempts: 5, CanRetry: true, ErrorCode: "import_cloud_source_slow"}
	readManagedImport = func(context.Context, repository.TokenFileStorage, string) (filestorage.URLImportTask, error) {
		return task, nil
	}
	requests, dispatched := 0, false
	retryManagedImportOnce = func(_ context.Context, storage repository.TokenFileStorage, key string, requestID uint64) (filestorage.URLImportTask, error) {
		if storage.APIKey != testStorageAPIKey || key != job.TaskKey || requestID != intent.ID {
			t.Fatal("manual action changed its storage identity")
		}
		requests++
		if !dispatched {
			dispatched = true
			task.State, task.Attempts, task.CanRetry = "running", 6, false
			return filestorage.URLImportTask{}, &filestorage.StorageError{Code: "timeout", Retryable: true}
		}
		return task, nil
	}
	if _, err := resolveManagedImportTask(ctx, runtimeImportStorage(), job, runtimeImportSource().URL); filestorage.ClassifyError(err).Code != "timeout" {
		t.Fatalf("lost acknowledgement was hidden: %v", err)
	}
	if _, err := resolveManagedImportTask(ctx, runtimeImportStorage(), job, runtimeImportSource().URL); filestorage.ClassifyError(err).Code != "pending" || !filestorage.ClassifyError(err).Retryable {
		t.Fatalf("new manual task inherited expired original wait: %v", err)
	}
	task.State, task.CanRetry = "failed", true
	if _, err := resolveManagedImportTask(ctx, runtimeImportStorage(), job, runtimeImportSource().URL); filestorage.ClassifyError(err).Code != "source_slow" || filestorage.ClassifyError(err).Retryable {
		t.Fatalf("confirmed manual failure did not stop: %v", err)
	}
	if requests != 2 || task.Attempts != 6 {
		t.Fatalf("duplicate manual request started another execution: requests=%d attempts=%d", requests, task.Attempts)
	}
}

func TestManualImportRetryNeverRestartsUncertainOrExpiredAction(t *testing.T) {
	for _, state := range []string{"queued", "running", "uncertain", "failed"} {
		t.Run(state, func(t *testing.T) {
			restore := stubManagedStorage(t)
			defer restore()
			job := runtimeImportJob(runtimeImportSource())
			ctx := context.WithValue(context.Background(), managedImportOperatorRetryKey{}, managedImportOperatorRetry{ID: 51, RequestedAt: time.Now().Add(-25 * time.Hour)})
			readManagedImport = func(context.Context, repository.TokenFileStorage, string) (filestorage.URLImportTask, error) {
				return filestorage.URLImportTask{TaskKey: job.TaskKey, State: state, Attempts: 5, CanRetry: state == "failed"}, nil
			}
			if _, err := resolveManagedImportTask(ctx, runtimeImportStorage(), job, runtimeImportSource().URL); err == nil || filestorage.ClassifyError(err).Retryable {
				t.Fatalf("expired manual action remained active: %v", err)
			}
		})
	}
}
