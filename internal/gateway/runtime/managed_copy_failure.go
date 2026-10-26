package runtime

import (
	"context"
	"time"

	"github.com/mirainya/Prism/internal/gateway/delivery"
	"github.com/mirainya/Prism/pkg/filestorage"
)

func managedTransferContext(ctx context.Context) (context.Context, context.CancelFunc) {
	deadline := time.Now().Add(5 * time.Minute)
	// A successful provider response still has to be committed after a slow
	// transfer. Reserve time for cleanup and the lease-fenced result write.
	if parent, ok := ctx.Deadline(); ok && parent.Add(-30*time.Second).Before(deadline) {
		deadline = parent.Add(-30 * time.Second)
	}
	return context.WithDeadline(ctx, deadline)
}

func managedTransferFailure(stage string, err error) string {
	failure := filestorage.ClassifyError(err)
	reason := (delivery.TransferFailure{Stage: stage, Code: failure.Code, HTTPStatus: failure.HTTPStatus, Retryable: failure.Retryable}).Reason()
	if _, ok := delivery.ParseTransferFailure(reason); !ok {
		return (delivery.TransferFailure{Stage: stage, Code: "unknown"}).Reason()
	}
	return reason
}

type managedCopyError struct{ Reason string }

func (e *managedCopyError) Error() string { return e.Reason }
