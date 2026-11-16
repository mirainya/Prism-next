package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mirainya/Prism/internal/domain"
	"github.com/mirainya/Prism/internal/gateway/repository"
	gatewayruntime "github.com/mirainya/Prism/internal/gateway/runtime"
	"github.com/mirainya/Prism/internal/gateway/security"
	"github.com/mirainya/Prism/internal/gateway/transport"
)

type unifiedRequestLog struct {
	service         *gatewayruntime.Service
	id              uint64
	requestComplete bool
}

func (l *callLifecycle) unifiedRequestOwner() *unifiedLifecycle {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.unified
}

func beginUnifiedRequestLog(owner *unifiedLifecycle, prepared transport.PreparedRequest, operation transport.Operation) (*unifiedRequestLog, error) {
	if owner == nil {
		return nil, nil
	}
	key, err := decodeGatewayHMACKey("PRISM_GATEWAY_PAYLOAD_HMAC_B64")
	if err != nil {
		return nil, err
	}
	defer clear(key)
	mapping, err := json.Marshal(struct {
		Operation transport.Operation
		Method    string
		URL       string
		Headers   map[string][]string
		Body      []byte
	}{operation, prepared.Method, prepared.URL, prepared.Headers, prepared.Body})
	if err != nil {
		return nil, err
	}
	defer clear(mapping)
	digest := security.HMACSHA256(key, mapping)
	service, err := gatewayruntime.New(owner.store)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	payload, cleanup, err := requestLogPayload(ctx, owner.store, prepared.Body)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	id, err := service.BeginRequestWithPayload(ctx, repository.RequestLogInput{
		AttemptID: &owner.attemptID, RequestSeq: 1, Action: "submit",
		MappingHMAC: fmt.Sprintf("%x", digest[:]), RequestBytesHMAC: requestLogBodyHMAC(key, prepared.Body),
	}, payload)
	if err != nil {
		return nil, unifiedAdmissionError(err)
	}
	return &unifiedRequestLog{service: service, id: id, requestComplete: len(prepared.Body) <= maxRequestLogPayloadBytes}, nil
}

func unifiedAdmissionError(err error) error {
	if errors.Is(err, repository.ErrConcurrencyLimit) {
		return &domain.AppError{HTTPStatus: http.StatusTooManyRequests, Code: "credential_capacity_exhausted", Message: "Upstream concurrency capacity is currently exhausted", Err: err}
	}
	return err
}

func (l *unifiedRequestLog) finish(hasResponse bool, status int, requestErr error, duration time.Duration) error {
	if l == nil {
		return nil
	}
	result := repository.RequestLogResult{}
	elapsed := uint64(max(duration.Milliseconds(), 0))
	result.DurationMS = &elapsed
	var providerStatus interface{ HTTPStatus() int }
	if status == 0 && errors.As(requestErr, &providerStatus) {
		status = providerStatus.HTTPStatus()
	}
	if status >= 100 && status <= 599 {
		value := uint16(status)
		result.HTTPStatus = &value
	}
	outcome := "unknown"
	if hasResponse || result.HTTPStatus != nil {
		outcome = "response_recorded"
	}
	if requestErr != nil {
		result.ErrorCode = "exchange_error"
	}
	// A canonical success is not wire evidence. HTTP errors preserve the actual
	// provider body, including non-JSON gateway errors, for the payload viewer.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result.RequestComplete = l.requestComplete && outcome == "response_recorded"
	var payloadErr error
	if details := transport.DetailsFromError(requestErr); details != nil && len(details.Raw) != 0 {
		payload, cleanup, err := requestLogPayload(ctx, l.service.Store, details.Raw)
		defer cleanup()
		payloadErr = err
		if err == nil {
			result.ResponsePayload = payload
			result.ResponseBytesHMAC = requestLogBodyHMAC(payload.HMACKey, details.Raw)
			result.ResponseComplete = len(details.Raw) <= maxRequestLogPayloadBytes
		}
	}
	// Evidence failures must not leave the HTTP capacity slot occupied.
	return errors.Join(payloadErr, l.service.FinishRequest(ctx, l.id, outcome, result))
}
