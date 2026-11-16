package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/mirainya/Prism/internal/gateway/delivery"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/pkg/filestorage"
)

func TestManagedImportRetainsFailureClassificationAfterRetriesExhausted(t *testing.T) {
	for sourceCode, want := range map[string]string{
		"import_configuration_invalid": "configuration", "import_cloud_disabled": "configuration",
		"import_source_unavailable": "source_unavailable", "import_source_rejected": "source_rejected",
		"import_worker_lost": "worker_lost", "import_cloud_unavailable": "service",
		"import_cloud_source_rejected": "source_rejected", "import_cloud_transfer_aborted": "transfer_aborted",
		"import_cloud_source_dns_unavailable": "source_dns", "import_cloud_source_resume_unavailable": "source_resume",
		"import_cloud_source_incomplete": "source_incomplete", "import_cloud_source_slow": "source_slow",
		"import_cloud_source_changed": "source_changed", "import_cloud_source_range_invalid": "source_changed",
		"import_cloud_source_retries_exhausted": "source_interrupted", "import_cloud_transfer_deadline": "transfer_timeout",
		"import_cloud_destination_rejected": "destination_rejected", "import_cloud_part_checksum_mismatch": "digest",
		"import_cloud_storage_unavailable": "service", "import_outcome_unknown": "outcome_unknown",
		"https://private.example/?key=test-secret": "outcome_unknown",
	} {
		t.Run(sourceCode, func(t *testing.T) {
			restore := stubManagedStorage(t)
			defer restore()
			job := runtimeImportJob(runtimeImportSource())
			readManagedImport = func(context.Context, repository.TokenFileStorage, string) (filestorage.URLImportTask, error) {
				return filestorage.URLImportTask{TaskKey: job.TaskKey, State: "failed", ErrorCode: sourceCode, CanRetry: true, Attempts: 5}, nil
			}
			retryManagedImport = func(context.Context, repository.TokenFileStorage, string) (filestorage.URLImportTask, error) {
				t.Fatal("diagnostics retried an exhausted task")
				return filestorage.URLImportTask{}, nil
			}
			_, err := resolveManagedImportTask(context.Background(), runtimeImportStorage(), job, runtimeImportSource().URL)
			got := filestorage.ClassifyError(err)
			if err == nil || got.Code != want || got.Retryable {
				t.Fatalf("code=%s err=%v", sourceCode, err)
			}
			failure := delivery.TransferFailure{Stage: "import", Code: got.Code}
			if parsed, ok := delivery.ParseTransferFailure(failure.Reason()); !ok || parsed != failure {
				t.Fatalf("lost persisted diagnosis: %s", failure.Reason())
			}
		})
	}
}

func TestManagedImportCloudSizeFailureKeepsSizeLimitReason(t *testing.T) {
	restore := stubManagedStorage(t)
	defer restore()
	job := runtimeImportJob(runtimeImportSource())
	readManagedImport = func(context.Context, repository.TokenFileStorage, string) (filestorage.URLImportTask, error) {
		return filestorage.URLImportTask{TaskKey: job.TaskKey, State: "failed", ErrorCode: "import_cloud_source_too_large"}, nil
	}
	_, err := resolveManagedImportTask(context.Background(), runtimeImportStorage(), job, runtimeImportSource().URL)
	var managed *managedCopyError
	if !errors.As(err, &managed) || managed.Reason != delivery.ManagedCopySizeExceeded {
		t.Fatalf("lost cloud size reason: %v", err)
	}
}
