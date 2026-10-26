package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/mirainya/Prism/internal/gateway/delivery"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

func (d *AsyncDispatcher) recoverManagedCopy(ctx context.Context, item repository.OutboxItem, target repository.DeliveryRecoveryTarget) error {
	if d == nil || d.service == nil || target.Mode != "managed_copy" {
		return &PermanentDispatchError{Code: "invalid_managed_copy_recovery"}
	}
	role, err := managedCopyRecoveryRole(target.ResourceKind, target.Ordinal)
	if err != nil {
		return err
	}
	var source delivery.RemoteResult
	if target.SourceKind == "remote_url" {
		sourceURL, err := d.openBlob(ctx, target.SourceBlobID, fmt.Sprintf("delivery:%d:source:%d", target.DeliveryID, target.SourceSequence), "gateway-result-source", false)
		if err != nil {
			return &PermanentDispatchError{Code: "managed_copy_source_unavailable"}
		}
		defer clear(sourceURL)
		source, err = decodeManagedSource(role, sourceURL)
		if err != nil {
			return err
		}
		if source.UpstreamAuth != nil {
			fixed, err := d.service.Store.ReadAsyncDispatch(ctx, target.AsyncExecutionID)
			if err != nil {
				return err
			}
			defer clear(fixed.CredentialSecret)
			if fixed.AttemptID != target.AttemptID || fixed.DeliveryMode != "managed_copy" {
				return &PermanentDispatchError{Code: "delivery_dispatch_mismatch"}
			}
			if err := d.bindAuthenticatedResult(ctx, fixed, &source); err != nil {
				return err
			}
		}
	} else if target.SourceKind == "inline_response" {
		source, err = d.savedManagedCopySource(ctx, target)
		if err != nil {
			return err
		}
		defer clear(source.InlineData)
	} else {
		return &PermanentDispatchError{Code: "invalid_managed_copy_recovery"}
	}
	copy, err := d.service.prepareManagedResultCopyAt(ctx, target.AttemptID, target.Ordinal, source)
	if err != nil {
		return err
	}
	commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 4*time.Second)
	defer cancel()
	return d.service.ApplyManagedCopyRecovery(commitCtx, item, copy)
}

func managedCopyRecoveryRole(resourceKind string, ordinal uint32) (string, error) {
	switch resourceKind {
	case delivery.ResourceCapabilityTask:
		return "image", nil
	case delivery.ResourceVideoTask:
		if ordinal == 0 {
			return "video", nil
		}
		if ordinal == 1 {
			return "thumbnail", nil
		}
	}
	return "", &PermanentDispatchError{Code: "invalid_managed_copy_ordinal"}
}

// prepareManagedResultCopyAt retries exactly one result ordinal. The object
// key remains the one fixed by the original Attempt and ordinal.
func (s *Service) prepareManagedResultCopyAt(ctx context.Context, attemptID uint64, ordinal uint32, source delivery.RemoteResult) (ManagedCopy, error) {
	if s == nil || s.Store == nil || attemptID == 0 || !delivery.ValidSource(source) {
		return ManagedCopy{}, &PermanentDispatchError{Code: "invalid_managed_copy_source"}
	}
	storage, err := s.Store.ReadAttemptFileStorage(ctx, attemptID)
	if err != nil {
		return ManagedCopy{}, err
	}
	if storage.APIKey == "" {
		return ManagedCopy{}, &PermanentDispatchError{Code: "managed_copy_storage_unconfigured"}
	}
	// prepareManagedResultCopiesAt uses the original ordinal for the immutable
	// object key, including when only one inline result is retried.
	preparedCopies, err := s.prepareManagedResultCopiesAtWithStorage(ctx, storage, attemptID, []delivery.RemoteResult{source}, ordinal)
	if err != nil {
		return ManagedCopy{}, err
	}
	prepared := preparedCopies[0]
	if prepared.FailureReason == "" {
		return prepared.ManagedCopy, nil
	}
	return ManagedCopy{}, &managedCopyError{Reason: prepared.FailureReason}
}
