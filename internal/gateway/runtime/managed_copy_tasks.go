package runtime

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/mirainya/Prism/internal/gateway/delivery"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/pkg/filestorage"
)

const managedImportMaxWait = 24 * time.Hour

type managedImportPersistenceError struct{ error }

// This metadata comes from the immutable, authorized delivery action, never
// from a downstream request parameter. Storage persists the ID before queueing.
type managedImportOperatorRetryKey struct{}
type managedImportOperatorRetry struct {
	ID          uint64
	RequestedAt time.Time
}

// These calls always use the storage snapshot of the original call.
var probeManagedImports = func(ctx context.Context, storage repository.TokenFileStorage) (bool, error) {
	return managedImportClient(storage).SupportsURLImportTasks(ctx)
}
var readManagedImport = func(ctx context.Context, storage repository.TokenFileStorage, key string) (filestorage.URLImportTask, error) {
	return managedImportClient(storage).ReadURLImportTask(ctx, key)
}
var createManagedImport = func(ctx context.Context, storage repository.TokenFileStorage, job repository.ManagedURLImport, source string) (filestorage.URLImportTask, error) {
	return managedImportClient(storage).CreateURLImportTask(ctx, job.TaskKey, source, job.StoragePath, job.OriginalFilename)
}
var retryManagedImport = func(ctx context.Context, storage repository.TokenFileStorage, key string) (filestorage.URLImportTask, error) {
	return managedImportClient(storage).RetryURLImportTask(ctx, key)
}
var retryManagedImportOnce = func(ctx context.Context, storage repository.TokenFileStorage, key string, requestID uint64) (filestorage.URLImportTask, error) {
	return managedImportClient(storage).RetryURLImportTaskOnce(ctx, key, requestID)
}
var findManagedImport = func(ctx context.Context, storage repository.TokenFileStorage, path, name string) (filestorage.UploadResult, error) {
	return managedImportClient(storage).FindImportedURLAtPath(ctx, path, name)
}

func managedImportClient(storage repository.TokenFileStorage) filestorage.Client {
	return filestorage.WithBaseURLAndAPIKey(storage.XFSBaseURL, storage.APIKey)
}

func managedImportSourceDigest(source delivery.RemoteResult) string {
	encoded, _ := json.Marshal([]string{source.Role, source.URL, source.AudioFormat})
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func (s *Service) reserveManagedImport(ctx context.Context, storage repository.TokenFileStorage, attemptID uint64, ordinal uint32, source delivery.RemoteResult, recovery bool) (repository.ManagedURLImport, error) {
	job, err := s.Store.ReadManagedURLImport(ctx, attemptID, ordinal)
	if err == nil {
		if job.SourceSHA256 != managedImportSourceDigest(source) || job.StorageBindingSHA256 != managedImportClient(storage).URLImportBindingSHA256() {
			return repository.ManagedURLImport{}, &filestorage.StorageError{Code: "binding"}
		}
		return job, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return repository.ManagedURLImport{}, &managedImportPersistenceError{err}
	}
	mode := repository.URLImportDetect
	if recovery {
		mode = repository.URLImportLegacy
	}
	in := repository.ManagedURLImport{
		AttemptID: attemptID, Ordinal: ordinal, Mode: mode,
		StoragePath: managedResultImportStoragePath(attemptID, ordinal), OriginalFilename: managedResultImportFilename(source),
		SourceSHA256: managedImportSourceDigest(source), StorageBindingSHA256: managedImportClient(storage).URLImportBindingSHA256(),
		// Without a durable intent a historical upload may already exist or
		// still be running. It must never be transmitted again.
		LegacyStarted: recovery,
	}
	err = s.Store.WithTx(ctx, func(tx *sql.Tx) error {
		var reserveErr error
		job, reserveErr = s.Store.ReserveManagedURLImport(ctx, tx, in)
		return reserveErr
	})
	if err != nil {
		return repository.ManagedURLImport{}, &managedImportPersistenceError{err}
	}
	return job, err
}

func (s *Service) importManagedPublicResult(ctx context.Context, storage repository.TokenFileStorage, attemptID uint64, ordinal uint32, source delivery.RemoteResult, recovery bool) (filestorage.UploadResult, error) {
	job, err := s.reserveManagedImport(ctx, storage, attemptID, ordinal, source, recovery)
	if err != nil {
		return filestorage.UploadResult{}, err
	}
	if job.Mode == repository.URLImportDetect {
		supported, probeErr := probeManagedImports(ctx, storage)
		if probeErr != nil {
			return filestorage.UploadResult{}, probeErr
		}
		mode := repository.URLImportLegacy
		if supported {
			mode = repository.URLImportAsyncV1
		}
		err := s.Store.WithTx(ctx, func(tx *sql.Tx) error {
			var selectErr error
			job, selectErr = s.Store.SelectManagedURLImportMode(ctx, tx, job, mode)
			return selectErr
		})
		if err != nil {
			return filestorage.UploadResult{}, &managedImportPersistenceError{err}
		}
	}
	if job.Mode == repository.URLImportAsyncV1 {
		if recovery {
			return resolveManagedImportTask(ctx, storage, job, source.URL)
		}
		return awaitManagedImportTask(ctx, storage, job, source.URL)
	}
	if !job.LegacyStarted {
		claimed := false
		err := s.Store.WithTx(ctx, func(tx *sql.Tx) error {
			var claimErr error
			claimed, claimErr = s.Store.ClaimLegacyURLImport(ctx, tx, job)
			return claimErr
		})
		if err != nil {
			return filestorage.UploadResult{}, &managedImportPersistenceError{err}
		}
		if claimed {
			result, err := importManagedResult(ctx, storage, source.URL, job.StoragePath, job.OriginalFilename)
			if err == nil {
				return result, nil
			}
			if ctx.Err() != nil {
				return filestorage.UploadResult{}, ctx.Err()
			}
			failure := filestorage.ClassifyError(err)
			if !failure.Retryable {
				return filestorage.UploadResult{}, failure
			}
			return filestorage.UploadResult{}, managedImportWaiting(job, "uncertain")
		}
	}
	result, err := findManagedImport(ctx, storage, job.StoragePath, job.OriginalFilename)
	if errors.Is(err, filestorage.ErrImportedFileNotFound) {
		return filestorage.UploadResult{}, managedImportWaiting(job, "uncertain")
	}
	return result, err
}

// Preserve synchronous callers' usual result when storage finishes promptly.
// Only the HTTP wait expires; the durable storage task continues independently.
func awaitManagedImportTask(ctx context.Context, storage repository.TokenFileStorage, job repository.ManagedURLImport, sourceURL string) (filestorage.UploadResult, error) {
	waitCtx, cancel := managedTransferContext(ctx)
	defer cancel()
	for waitCtx.Err() == nil {
		result, err := resolveManagedImportTask(waitCtx, storage, job, sourceURL)
		if waitCtx.Err() != nil {
			break
		}
		failure := filestorage.ClassifyError(err)
		if err == nil || failure.Code != "pending" || !failure.Retryable {
			return result, err
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-waitCtx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	if ctx.Err() != nil {
		return filestorage.UploadResult{}, ctx.Err()
	}
	return filestorage.UploadResult{}, managedImportWaiting(job, "pending")
}

func resolveManagedImportTask(ctx context.Context, storage repository.TokenFileStorage, job repository.ManagedURLImport, sourceURL string) (filestorage.UploadResult, error) {
	operator, manual := ctx.Value(managedImportOperatorRetryKey{}).(managedImportOperatorRetry)
	if manual && operator.ID != 0 && !operator.RequestedAt.IsZero() {
		// A new manual action has its own bounded wait window. The original
		// task identity and automatic lifetime attempt limit stay unchanged.
		job.CreatedAt = operator.RequestedAt
	} else {
		manual = false
	}
	task, err := readManagedImport(ctx, storage, job.TaskKey)
	if errors.Is(err, filestorage.ErrImportTaskNotFound) {
		// The persisted identity makes a lost acknowledgement safe to repeat.
		// An unavailable or disabled endpoint never selects synchronous import.
		task, err = createManagedImport(ctx, storage, job, sourceURL)
	}
	if err != nil {
		return filestorage.UploadResult{}, err
	}
	if task.TaskKey != job.TaskKey {
		return filestorage.UploadResult{}, &filestorage.StorageError{Code: "response"}
	}
	if task.State == "failed" && task.CanRetry && (manual || task.Attempts < 5) && time.Since(job.CreatedAt) < managedImportMaxWait {
		if manual {
			task, err = retryManagedImportOnce(ctx, storage, job.TaskKey, operator.ID)
		} else {
			task, err = retryManagedImport(ctx, storage, job.TaskKey)
		}
		if err != nil {
			return filestorage.UploadResult{}, err
		}
	}
	if task.TaskKey != job.TaskKey {
		return filestorage.UploadResult{}, &filestorage.StorageError{Code: "response"}
	}
	switch task.State {
	case "succeeded":
		if task.Result == nil || task.Result.Path != job.StoragePath || task.Result.OriginalName != job.OriginalFilename {
			return filestorage.UploadResult{}, &filestorage.StorageError{Code: "response"}
		}
		return *task.Result, nil
	case "queued", "running":
		return filestorage.UploadResult{}, managedImportWaiting(job, "pending")
	case "uncertain":
		return filestorage.UploadResult{}, managedImportWaiting(job, "uncertain")
	case "failed":
		code := "outcome_unknown"
		switch task.ErrorCode {
		case "import_configuration_invalid", "import_cloud_configuration_invalid", "import_cloud_disabled":
			code = "configuration"
		case "import_source_too_large", "import_cloud_source_too_large":
			return filestorage.UploadResult{}, &managedCopyError{Reason: delivery.ManagedCopySizeExceeded}
		case "import_source_unavailable", "import_cloud_source_unavailable":
			code = "source_unavailable"
		case "import_source_rejected", "import_cloud_source_rejected":
			code = "source_rejected"
		case "import_worker_lost":
			code = "worker_lost"
		case "import_cloud_unavailable", "import_cloud_storage_unavailable":
			code = "service"
		case "import_cloud_transfer_aborted":
			code = "transfer_aborted"
		case "import_cloud_source_dns_unavailable":
			code = "source_dns"
		case "import_cloud_source_resume_unavailable":
			code = "source_resume"
		case "import_cloud_source_incomplete":
			code = "source_incomplete"
		case "import_cloud_source_slow":
			code = "source_slow"
		case "import_cloud_source_changed", "import_cloud_source_range_invalid":
			code = "source_changed"
		case "import_cloud_source_retries_exhausted":
			code = "source_interrupted"
		case "import_cloud_transfer_deadline":
			code = "transfer_timeout"
		case "import_cloud_destination_rejected":
			code = "destination_rejected"
		case "import_cloud_part_checksum_mismatch":
			code = "digest"
		}
		return filestorage.UploadResult{}, &filestorage.StorageError{Code: code}
	default:
		return filestorage.UploadResult{}, &filestorage.StorageError{Code: "response"}
	}
}

func managedImportWaiting(job repository.ManagedURLImport, code string) error {
	return &filestorage.StorageError{Code: code, Retryable: time.Since(job.CreatedAt) < managedImportMaxWait}
}
