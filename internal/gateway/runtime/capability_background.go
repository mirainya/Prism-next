package runtime

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/mirainya/Prism/internal/gateway/repository"
)

// CapabilityPrepare reconstructs a provider request from the committed input
// and immutable catalog snapshot, without performing a generation request.
type CapabilityPrepare func(context.Context, repository.BackgroundCapabilityDispatch, []byte, []byte, AsyncAssetLoader) (CapabilityDispatchInput, error)

func NewBackgroundCapabilityDispatcher(service *Service, client *http.Client, keys AsyncKeys, prepare CapabilityPrepare) (*CapabilityDispatcher, error) {
	if prepare == nil {
		return nil, repository.ErrInvalidInput
	}
	d, err := NewCapabilityDispatcher(service, client, keys)
	if err != nil {
		return nil, err
	}
	d.prepareBackground = prepare
	return d, nil
}

func (d *CapabilityDispatcher) DispatchAttempt(ctx context.Context, item repository.OutboxItem) error {
	if d.prepareBackground == nil || item.Action != "submit" {
		return repository.ErrInvalidInput
	}
	fixed, err := d.service.Store.ReadBackgroundCapabilityDispatch(ctx, item.AttemptID)
	if err != nil {
		return err
	}
	reject := func(reason string, cause error) error {
		return d.rejectBeforeDispatch(ctx, CapabilityDispatchInput{AttemptID: item.AttemptID, Outbox: &item}, reason, cause)
	}
	policy, err := d.service.Store.ReadRoutePolicy(ctx, fixed.ReleaseID, fixed.SKUID, fixed.ProductTransportID)
	if err != nil {
		return err
	}
	if policy.TaskScope != "none" && policy.TaskScope != "request" || policy.CancelMode != "none" {
		return reject("background_route_invalid", repository.ErrConflict)
	}
	body, err := d.network.openBlob(ctx, fixed.RequestBlobID, fmt.Sprintf("call:%d:request", fixed.CallID), "gateway-payload", false)
	if err != nil {
		return reject("background_request_unreadable", err)
	}
	defer clear(body)
	in, err := d.prepareBackground(ctx, fixed, body, policy.AdapterConfig, storedAsyncAssetLoader{store: d.service.Store, callID: fixed.CallID, maxBytes: 20 << 20})
	if err != nil {
		return reject("background_request_invalid", err)
	}
	defer clear(in.Prepared.Body)
	in.CallID, in.AttemptID, in.Outbox = fixed.CallID, item.AttemptID, &item
	return d.Dispatch(ctx, in)
}

func (d *CapabilityDispatcher) RecoverAttempt(ctx context.Context, item repository.OutboxItem) error {
	request, err := d.service.Store.ReadLatestAttemptRequest(ctx, item.AttemptID, item.Action)
	if errors.Is(err, repository.ErrNotFound) || err == nil && request.Status == "not_sent" {
		return d.DispatchAttempt(ctx, item)
	}
	if err != nil {
		return err
	}
	// An authorized synchronous operation has no provider task ID to query.
	// Preserve uncertainty instead of submitting and charging for it again.
	return d.service.FailCapability(context.WithoutCancel(ctx), CapabilityFailureInput{
		AttemptID: item.AttemptID, RequestID: request.RequestID, Outbox: &item,
		Exchange: repository.RequestLogResult{ErrorCode: "background_submission_unverified"},
		Reason:   "background_submission_unverified", Uncertain: true,
	})
}

// AttemptDispatcher routes the shared durable queue by its resource kind.
// Independent workers must not claim work belonging to a different dispatcher.
type AttemptDispatcher struct {
	Store                   *repository.Store
	Responses, Capabilities AttemptOutboxDispatcher
}

func (d AttemptDispatcher) handler(ctx context.Context, item repository.OutboxItem) (AttemptOutboxDispatcher, error) {
	kind, err := d.Store.ReadAttemptResourceKind(ctx, item.AttemptID)
	if err != nil {
		return nil, err
	}
	switch kind {
	case "response":
		if d.Responses != nil {
			return d.Responses, nil
		}
	case "capability_task":
		if d.Capabilities != nil {
			return d.Capabilities, nil
		}
	}
	return nil, repository.ErrConflict
}

func (d AttemptDispatcher) DispatchAttempt(ctx context.Context, item repository.OutboxItem) error {
	handler, err := d.handler(ctx, item)
	if err != nil {
		return err
	}
	return handler.DispatchAttempt(ctx, item)
}

func (d AttemptDispatcher) RecoverAttempt(ctx context.Context, item repository.OutboxItem) error {
	handler, err := d.handler(ctx, item)
	if err != nil {
		return err
	}
	return handler.RecoverAttempt(ctx, item)
}
