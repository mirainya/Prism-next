package runtime

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mirainya/Prism/internal/gateway/delivery"
	"github.com/mirainya/Prism/internal/gateway/execution"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

// savedManagedCopySource decodes retained, authenticated HTTP evidence. It
// never prepares or sends an upstream request, including a generation request.
func (d *AsyncDispatcher) savedManagedCopySource(ctx context.Context, target repository.DeliveryRecoveryTarget) (delivery.RemoteResult, error) {
	fixed, err := d.service.Store.ReadAsyncDispatch(ctx, target.AsyncExecutionID)
	if err != nil {
		return delivery.RemoteResult{}, err
	}
	defer clear(fixed.CredentialSecret)
	if fixed.AttemptID != target.AttemptID || fixed.DeliveryMode != "managed_copy" {
		return delivery.RemoteResult{}, &PermanentDispatchError{Code: "delivery_dispatch_mismatch"}
	}
	var requestID, blobID uint64
	var action, digest string
	err = d.service.Store.DB().QueryRowContext(ctx, `SELECT l.id,l.response_payload_blob_id,l.response_bytes_hmac,l.action
FROM gw_channel_request_logs l JOIN gw_api_call_attempts a ON a.id=l.attempt_id
JOIN gw_api_calls c ON c.id=a.call_id JOIN gw_async_executions x ON x.attempt_id=a.id
WHERE l.attempt_id=? AND c.status='completed' AND a.state='completed' AND x.id=? AND x.state='succeeded'
AND l.action IN ('submit','query') AND l.status='response_recorded' AND l.http_status BETWEEN 200 AND 299
AND l.request_bytes_complete=1 AND l.response_bytes_complete=1 AND l.response_payload_blob_id IS NOT NULL
AND COALESCE(l.error_code,'')='' ORDER BY l.request_seq DESC,l.id DESC LIMIT 1`, target.AttemptID, target.AsyncExecutionID).Scan(&requestID, &blobID, &digest, &action)
	if err == sql.ErrNoRows {
		return delivery.RemoteResult{}, &PermanentDispatchError{Code: "managed_copy_source_unavailable"}
	}
	if err != nil {
		return delivery.RemoteResult{}, err
	}
	response, err := d.openBlob(ctx, blobID, fmt.Sprintf("request-log:%d:response", requestID), "gateway-upstream-response", false)
	if err != nil {
		return delivery.RemoteResult{}, &PermanentDispatchError{Code: "managed_copy_source_unavailable"}
	}
	defer clear(response)
	if d.digest(response) != digest {
		return delivery.RemoteResult{}, &PermanentDispatchError{Code: "managed_copy_evidence_invalid"}
	}
	request, err := d.openBlob(ctx, fixed.RequestBlobID, fmt.Sprintf("call:%d:request", fixed.CallID), "gateway-payload", false)
	if err != nil {
		return delivery.RemoteResult{}, &PermanentDispatchError{Code: "managed_copy_source_unavailable"}
	}
	defer clear(request)
	codec, ok := d.codecs[fmt.Sprintf("%s@%d", fixed.AdapterCode, fixed.AdapterVersion)]
	if !ok {
		return delivery.RemoteResult{}, &PermanentDispatchError{Code: "unsupported_async_protocol"}
	}
	var observation AsyncObservation
	if aware, ok := codec.(DispatchAwareAsyncCodec); ok {
		observation, err = aware.DecodeWithDispatch(action, fixed, request, response)
	} else {
		observation, err = codec.Decode(action, request, response)
	}
	defer clear(observation.Result)
	if err != nil || observation.State != execution.AsyncSucceeded ||
		delivery.ValidateSourcesForResource(target.ResourceKind, observation.Sources) != nil ||
		uint64(target.Ordinal) >= uint64(len(observation.Sources)) {
		for _, source := range observation.Sources {
			clear(source.InlineData)
		}
		return delivery.RemoteResult{}, &PermanentDispatchError{Code: "managed_copy_evidence_invalid"}
	}
	source := observation.Sources[target.Ordinal]
	for index, other := range observation.Sources {
		if uint32(index) != target.Ordinal {
			clear(other.InlineData)
		}
	}
	if delivery.SourceKind(source) != "inline_response" {
		clear(source.InlineData)
		return delivery.RemoteResult{}, &PermanentDispatchError{Code: "managed_copy_evidence_invalid"}
	}
	return source, nil
}
