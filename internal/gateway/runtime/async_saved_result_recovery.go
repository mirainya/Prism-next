package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/mirainya/Prism/internal/gateway/execution"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

// SavedVideoRecovery is an operator preview. The immutable request log and
// settlement remain intact; this repair never sends a generation request.
type SavedVideoRecovery struct {
	PublicID       string   `json:"public_id"`
	ProviderTaskID string   `json:"provider_task_id"`
	CallID         uint64   `json:"call_id"`
	RequestLogID   uint64   `json:"request_log_id"`
	ResultURLs     []string `json:"result_urls"`
	PolicyError    string   `json:"policy_error,omitempty"`
	AlreadyDone    bool     `json:"already_done"`
}

type savedVideoEvidence struct {
	callID, attemptID, resourceID, callVersion, attemptVersion, asyncVersion uint64
	callState, attemptState, asyncState, reason, publicID, settlement        string
	resultID                                                                 sql.NullInt64
}

func readSavedVideoEvidence(ctx context.Context, db repository.DB, asyncID, requestID uint64) (savedVideoEvidence, error) {
	var out savedVideoEvidence
	err := db.QueryRowContext(ctx, `SELECT c.id,a.id,r.id,c.state_version,a.state_version,x.state_version,
c.status,a.state,x.state,e.reason_code,r.public_id,bs.actual_amount,c.result_payload_id
FROM gw_async_executions x
JOIN gw_api_call_attempts a ON a.id=x.attempt_id
JOIN gw_api_calls c ON c.id=a.call_id AND c.final_attempt_id=a.id
JOIN gw_api_resources r ON r.call_id=c.id AND r.resource_kind='video_task' AND r.deleted_at IS NULL
JOIN gw_video_tasks v ON v.resource_id=r.id
JOIN gw_channel_request_logs l ON l.attempt_id=a.id AND l.id=?
JOIN gw_state_transition_events e ON e.async_execution_id=x.id AND e.state_version=x.state_version
JOIN billing_reservations br ON br.call_id=c.id AND br.state='settled'
JOIN billing_settlements bs ON bs.reservation_id=br.id
WHERE x.id=? AND c.current_attempt_id IS NULL AND c.callback_event_seq=0
AND NOT EXISTS(SELECT 1 FROM gw_callback_targets t WHERE t.call_id=c.id)
AND NOT EXISTS(SELECT 1 FROM gw_async_outbox o WHERE o.async_execution_id=x.id AND o.status IN ('pending','dispatching'))`, requestID, asyncID).
		Scan(&out.callID, &out.attemptID, &out.resourceID, &out.callVersion, &out.attemptVersion, &out.asyncVersion,
			&out.callState, &out.attemptState, &out.asyncState, &out.reason, &out.publicID, &out.settlement, &out.resultID)
	return out, err
}

func (e savedVideoEvidence) allowed(publicID string) bool {
	return e.publicID == publicID && e.callState == "failed" && e.attemptState == "failed" && e.asyncState == "failed" &&
		(e.reason == "provider_result_host_not_allowed" || e.reason == "invalid_provider_response") && !e.resultID.Valid && zeroSettlement(e.settlement)
}

func zeroSettlement(value string) bool {
	return value != "" && strings.Trim(value, "0.") == ""
}

func (e savedVideoEvidence) alreadyRestored(publicID string) bool {
	return e.publicID == publicID && e.callState == "completed" && e.attemptState == "completed" && e.asyncState == "succeeded" &&
		e.resultID.Valid && strings.HasPrefix(e.reason, "operator_") && strings.HasSuffix(e.reason, "_saved_result_zero_charge") && zeroSettlement(e.settlement)
}

func (d *AsyncDispatcher) savedVideoObservation(ctx context.Context, asyncID, requestID uint64, providerID string) (repository.AsyncDispatch, AsyncObservation, error) {
	fixed, err := d.service.Store.ReadAsyncDispatch(ctx, asyncID)
	if err != nil {
		return fixed, AsyncObservation{}, err
	}
	identity, err := d.openBlob(ctx, fixed.TaskIdentityBlobID, fmt.Sprintf("async:%d:task-identity", asyncID), "gateway-task-identity", false)
	if err != nil {
		return fixed, AsyncObservation{}, err
	}
	defer clear(identity)
	if string(identity) != providerID {
		return fixed, AsyncObservation{}, repository.ErrConflict
	}
	var blobID uint64
	var digest string
	err = d.service.Store.DB().QueryRowContext(ctx, `SELECT response_payload_blob_id,response_bytes_hmac
FROM gw_channel_request_logs WHERE id=? AND attempt_id=? AND action='query' AND status='response_recorded'
AND http_status BETWEEN 200 AND 299 AND request_bytes_complete=1 AND response_bytes_complete=1
AND error_code IN ('provider_result_host_not_allowed','invalid_provider_response')`, requestID, fixed.AttemptID).Scan(&blobID, &digest)
	if err != nil {
		return fixed, AsyncObservation{}, err
	}
	response, err := d.openBlob(ctx, blobID, fmt.Sprintf("request-log:%d:response", requestID), "gateway-upstream-response", false)
	if err != nil {
		return fixed, AsyncObservation{}, err
	}
	defer clear(response)
	if d.digest(response) != digest {
		return fixed, AsyncObservation{}, repository.ErrConflict
	}
	request, err := d.openBlob(ctx, fixed.RequestBlobID, fmt.Sprintf("call:%d:request", fixed.CallID), "gateway-payload", false)
	if err != nil {
		return fixed, AsyncObservation{}, err
	}
	defer clear(request)
	codec, ok := d.codecs[fmt.Sprintf("%s@%d", fixed.AdapterCode, fixed.AdapterVersion)]
	if !ok {
		return fixed, AsyncObservation{}, repository.ErrInvalidInput
	}
	var observation AsyncObservation
	if aware, ok := codec.(DispatchAwareAsyncCodec); ok {
		observation, err = aware.DecodeWithDispatch("query", fixed, request, response)
	} else {
		observation, err = codec.Decode("query", request, response)
	}
	if err != nil {
		return fixed, observation, err
	}
	if observation.State != execution.AsyncSucceeded || len(observation.Result) == 0 || len(observation.Sources) == 0 ||
		(observation.TaskID != "" && observation.TaskID != providerID) {
		clear(observation.Result)
		return fixed, AsyncObservation{}, repository.ErrConflict
	}
	return fixed, observation, nil
}

func (d *AsyncDispatcher) InspectSavedVideoResult(ctx context.Context, asyncID, requestID uint64, publicID, providerID string) (SavedVideoRecovery, error) {
	preview := SavedVideoRecovery{PublicID: publicID, ProviderTaskID: providerID, RequestLogID: requestID}
	if asyncID == 0 || requestID == 0 || publicID == "" || providerID == "" {
		return preview, repository.ErrInvalidInput
	}
	evidence, err := readSavedVideoEvidence(ctx, d.service.Store.DB(), asyncID, requestID)
	if err != nil {
		return preview, err
	}
	preview.CallID = evidence.callID
	if !evidence.allowed(publicID) && !evidence.alreadyRestored(publicID) {
		return preview, repository.ErrConflict
	}
	fixed, observation, err := d.savedVideoObservation(ctx, asyncID, requestID, providerID)
	if err != nil {
		return preview, err
	}
	defer clear(observation.Result)
	if evidence.alreadyRestored(publicID) {
		preview.AlreadyDone = true
		return preview, nil
	}
	for _, source := range observation.Sources {
		preview.ResultURLs = append(preview.ResultURLs, source.URL)
	}
	if err := d.validateResultSources(ctx, fixed, observation.Sources); err != nil {
		if err.Error() != "provider_result_host_not_allowed" {
			return preview, err
		}
		preview.PolicyError = err.Error()
	}
	return preview, nil
}

// RestoreSavedVideoResult is a narrowly scoped repair of a local policy error,
// not a general failed-to-success transition. Keeping the settled zero charge
// requires explicit operator approval. Immutable billing and HTTP evidence is
// preserved; no late debit or resubmission occurs.
func (d *AsyncDispatcher) RestoreSavedVideoResult(ctx context.Context, asyncID, requestID, operatorID uint64, publicID, providerID string, keepSettledZero bool) error {
	if operatorID == 0 || !keepSettledZero {
		return repository.ErrInvalidInput
	}
	if err := d.service.requireReadiness(ctx); err != nil {
		return err
	}
	preview, err := d.InspectSavedVideoResult(ctx, asyncID, requestID, publicID, providerID)
	if err != nil || preview.AlreadyDone {
		return err
	}
	if preview.PolicyError != "" {
		return &PermanentDispatchError{Code: preview.PolicyError}
	}
	fixed, observation, err := d.savedVideoObservation(ctx, asyncID, requestID, providerID)
	if err != nil {
		return err
	}
	defer clear(observation.Result)
	if err := d.validateResultSources(ctx, fixed, observation.Sources); err != nil {
		return err
	}
	for index := range observation.Sources {
		if err := d.bindAuthenticatedResult(ctx, fixed, &observation.Sources[index]); err != nil {
			return err
		}
	}
	blob, err := d.payloadBlob(ctx, observation.Result)
	if err != nil {
		return err
	}
	in := AsyncResultInput{Item: repository.OutboxItem{AsyncExecutionID: asyncID}, State: execution.AsyncSucceeded,
		Result: &blob, Sources: observation.Sources, SourceURLPolicy: fixed.SourceURLPolicy}
	prepared, err := d.service.prepareManagedCopies(ctx, in)
	if err != nil {
		return err
	}
	return d.service.Store.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := d.service.Store.LockCallBillingAncestors(ctx, tx, fixed.CallID); err != nil {
			return err
		}
		// Hold the same parent locks as normal completion before rechecking
		// evidence and versions. Concurrent/repeated repairs cannot double-write.
		for _, lock := range []struct {
			query string
			id    uint64
		}{
			{`SELECT id FROM gw_api_calls WHERE id=? FOR UPDATE`, fixed.CallID},
			{`SELECT id FROM gw_api_call_attempts WHERE id=? FOR UPDATE`, fixed.AttemptID},
			{`SELECT id FROM gw_async_executions WHERE id=? FOR UPDATE`, asyncID},
		} {
			var id uint64
			if err := tx.QueryRowContext(ctx, lock.query, lock.id).Scan(&id); err != nil {
				return err
			}
		}
		evidence, err := readSavedVideoEvidence(ctx, tx, asyncID, requestID)
		if err != nil {
			return err
		}
		if !evidence.allowed(publicID) {
			return repository.ErrConflict
		}
		var deliveries uint64
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM gw_result_deliveries WHERE call_id=?`, fixed.CallID).Scan(&deliveries); err != nil {
			return err
		}
		if deliveries != 0 {
			return repository.ErrConflict
		}
		if err := d.service.persistAsyncResult(ctx, tx, fixed.AttemptID, requestID, blob, observation.Sources, fixed.SourceURLPolicy, prepared); err != nil {
			return err
		}
		return restoreSavedVideoStates(ctx, tx, asyncID, operatorID, evidence)
	})
}

func restoreSavedVideoStates(ctx context.Context, tx *sql.Tx, asyncID, operatorID uint64, e savedVideoEvidence) error {
	now := time.Now().UTC()
	reason := fmt.Sprintf("operator_%d_saved_result_zero_charge", operatorID)
	for _, change := range []struct {
		query, parent, target string
		id, version           uint64
	}{
		{`UPDATE gw_async_executions SET state=?,state_version=state_version+1,action_seq=action_seq+1,updated_at=? WHERE id=? AND state='failed' AND state_version=?`, "async_execution_id", "succeeded", asyncID, e.asyncVersion},
		{`UPDATE gw_api_call_attempts SET state=?,state_version=state_version+1,updated_at=? WHERE id=? AND state='failed' AND state_version=?`, "attempt_id", "completed", e.attemptID, e.attemptVersion},
		{`UPDATE gw_api_calls SET status=?,state_version=state_version+1,updated_at=? WHERE id=? AND status='failed' AND state_version=?`, "call_id", "completed", e.callID, e.callVersion},
	} {
		result, err := tx.ExecContext(ctx, change.query, change.target, now, change.id, change.version)
		if err != nil {
			return err
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if rows != 1 {
			return repository.ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO gw_state_transition_events(`+change.parent+`,old_state,new_state,state_version,reason_code,created_at) VALUES (?,'failed',?,?,?,?)`, change.id, change.target, change.version+1, reason, now); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE gw_video_tasks SET status='completed',progress=100,updated_at=? WHERE resource_id=? AND status='failed'`, now, e.resourceID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return repository.ErrConflict
	}
	return nil
}
