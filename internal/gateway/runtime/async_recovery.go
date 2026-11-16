package runtime

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mirainya/Prism/internal/gateway/execution"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

// RecoverAsyncTask binds an operator-confirmed provider ID and schedules only
// a query. Unknown terminal parents and their billing holds remain unchanged
// until an authoritative provider result resolves them.
func (s *Service) RecoverAsyncTask(ctx context.Context, asyncID, operatorID uint64, identity repository.BlobInput) error {
	if asyncID == 0 || operatorID == 0 || len(identity.Plaintext) > 4096 {
		return repository.ErrInvalidInput
	}
	if err := s.requireReadiness(ctx); err != nil {
		return err
	}
	return s.Store.WithTx(ctx, func(tx *sql.Tx) error {
		var callID, attemptID, currentAttemptID uint64
		if err := tx.QueryRowContext(ctx, `SELECT a.call_id,a.id FROM gw_async_executions x JOIN gw_api_call_attempts a ON a.id=x.attempt_id WHERE x.id=?`, asyncID).Scan(&callID, &attemptID); err != nil {
			return err
		}
		if _, err := s.Store.LockCallBillingAncestors(ctx, tx, callID); err != nil {
			return err
		}
		var callState execution.CallState
		if err := tx.QueryRowContext(ctx, `SELECT status,COALESCE(current_attempt_id,final_attempt_id,0) FROM gw_api_calls WHERE id=? FOR UPDATE`, callID).Scan(&callState, &currentAttemptID); err != nil {
			return err
		}
		var attemptState execution.AttemptState
		if err := tx.QueryRowContext(ctx, `SELECT state FROM gw_api_call_attempts WHERE id=? FOR UPDATE`, attemptID).Scan(&attemptState); err != nil {
			return err
		}
		var state execution.AsyncState
		var version uint64
		if err := tx.QueryRowContext(ctx, `SELECT state,state_version FROM gw_async_executions WHERE id=? FOR UPDATE`, asyncID).Scan(&state, &version); err != nil {
			return err
		}
		if currentAttemptID != attemptID || !asyncRecoveryAllowed(state, attemptState, callState) {
			return repository.ErrConflict
		}
		var queryPending bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM gw_async_outbox WHERE async_execution_id=? AND action='query' AND status IN ('pending','dispatching'))`, asyncID).Scan(&queryPending); err != nil {
			return err
		}
		if queryPending {
			return repository.ErrConflict
		}
		if len(identity.Plaintext) != 0 {
			var existing uint64
			err := tx.QueryRowContext(ctx, `SELECT encrypted_blob_id FROM gw_upstream_task_identities WHERE async_execution_id=? AND status='bound'`, asyncID).Scan(&existing)
			// An already bound identity must never be silently replaced.
			if err == nil {
				return repository.ErrConflict
			}
			if err != sql.ErrNoRows {
				return err
			}
			if _, err := s.Store.PutTaskIdentity(ctx, tx, asyncID, identity, time.Now().UTC().Add(30*24*time.Hour)); err != nil {
				return err
			}
		} else {
			var existing uint64
			if err := tx.QueryRowContext(ctx, `SELECT encrypted_blob_id FROM gw_upstream_task_identities WHERE async_execution_id=? AND status='bound'`, asyncID).Scan(&existing); err != nil {
				return err
			}
		}
		if err := s.Store.EnsureAsyncTracking(ctx, tx, asyncID); err != nil {
			return err
		}
		if err := s.Store.SetAsyncTrackingMode(ctx, tx, asyncID, "active", operatorID); err != nil {
			return err
		}
		reason := fmt.Sprintf("operator_%d_bound_upstream_task", operatorID)
		if state == execution.AsyncTerminatedUnknown {
			// Continue reconciliation without reopening a terminal execution.
			return s.Store.ScheduleAsyncRecoveryQuery(ctx, tx, asyncID, version, reason)
		}
		if state == execution.AsyncAccepted || state == execution.AsyncRunning {
			return s.Store.ScheduleAsyncQuery(ctx, tx, asyncID, state, state, version, time.Now().UTC())
		}
		if _, err := s.Store.TransitionAsync(ctx, tx, asyncID, state, execution.AsyncAccepted, version, reason, "query"); err != nil {
			return err
		}
		return s.updateAsyncResourceProjection(ctx, tx, asyncID, execution.AsyncAccepted)
	})
}

func asyncRecoveryAllowed(state execution.AsyncState, attempt execution.AttemptState, call execution.CallState) bool {
	if state == execution.AsyncTerminatedUnknown {
		return attempt == execution.AttemptTerminatedUnknown && call == execution.CallIndeterminate
	}
	if state == execution.AsyncAccepted || state == execution.AsyncRunning {
		return attempt == execution.AttemptStarted && call == execution.CallInProgress
	}
	return (state == execution.AsyncSubmissionUnknown || state == execution.AsyncManualReview) &&
		(attempt == execution.AttemptStarted || attempt == execution.AttemptRecoveryPending) && call == execution.CallInProgress
}
