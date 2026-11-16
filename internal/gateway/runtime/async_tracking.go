package runtime

import (
	"context"
	"database/sql"
	"time"

	"github.com/mirainya/Prism/internal/gateway/execution"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

// StopAsyncTracking pauses local queries only. A received terminal result and
// callbacks may still complete the execution through the ordinary billing path.
func (s *Service) StopAsyncTracking(ctx context.Context, asyncID, operatorID uint64) error {
	if asyncID == 0 || operatorID == 0 {
		return repository.ErrInvalidInput
	}
	if err := s.requireReadiness(ctx); err != nil {
		return err
	}
	return s.Store.WithTx(ctx, func(tx *sql.Tx) error {
		var attemptID uint64
		if err := tx.QueryRowContext(ctx, `SELECT attempt_id FROM gw_async_executions WHERE id=?`, asyncID).Scan(&attemptID); err != nil {
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
		switch state {
		case execution.AsyncAccepted, execution.AsyncRunning, execution.AsyncSubmissionUnknown, execution.AsyncManualReview, execution.AsyncTerminatedUnknown:
		default:
			return repository.ErrConflict
		}
		if err := s.Store.EnsureAsyncTracking(ctx, tx, asyncID); err != nil {
			return err
		}
		if err := s.Store.SetAsyncTrackingMode(ctx, tx, asyncID, "paused", operatorID); err != nil {
			return err
		}
		// Leave a live exchange leased so its received result can still be committed.
		if _, err := tx.ExecContext(ctx, `UPDATE gw_async_outbox SET status='dead_letter',last_error_code='operator_paused_tracking',updated_at=? WHERE async_execution_id=? AND action IN ('query','recover') AND status='pending'`, time.Now().UTC(), asyncID); err != nil {
			return err
		}
		return nil
	})
}

// Polling failures use elapsed time, never the number of HTTP attempts.
func (s *Service) retryTrackedQueryTx(ctx context.Context, tx *sql.Tx, item repository.OutboxItem, code string, now time.Time) error {
	action, err := s.lockAsyncAction(ctx, tx, item, false)
	if err != nil {
		return err
	}
	if action.version != item.StateVersion || action.sequence != item.ActionSeq || !asyncRequestAllowed(action.state, "query") {
		return s.Store.DeadLetterAsyncOutbox(ctx, tx, item, "stale_provider_poll")
	}
	if err := s.Store.EnsureAsyncTracking(ctx, tx, item.AsyncExecutionID); err != nil {
		return err
	}
	t, err := s.Store.ReadAsyncTracking(ctx, tx, item.AsyncExecutionID, true)
	if err != nil {
		return err
	}
	if t.Mode == "paused" || t.Mode == "manual" {
		return s.Store.DeadLetterAsyncOutbox(ctx, tx, item, "tracking_stopped")
	}
	first := now
	if t.FirstErrorAt.Valid {
		first = t.FirstErrorAt.Time
	}
	mode, delay, until := queryRetryPlan(t, first, now, item.Attempts)
	if _, err := tx.ExecContext(ctx, `UPDATE gw_async_tracking SET mode=?,first_error_at=?,reconciliation_until=?,last_error_code=?,updated_at=? WHERE async_execution_id=?`, mode, first, until, code, now, item.AsyncExecutionID); err != nil {
		return err
	}
	if mode == "manual" {
		return s.Store.DeadLetterAsyncOutbox(ctx, tx, item, "provider_poll_reconciliation_expired")
	}
	return s.Store.RetryAsyncOutbox(ctx, tx, item, code, now.Add(delay))
}

func queryRetryPlan(t repository.AsyncTracking, first, now time.Time, attempts uint64) (string, time.Duration, time.Time) {
	until := first.Add(time.Duration(t.ReconciliationWindowSeconds) * time.Second)
	if t.ReconciliationUntil.Valid {
		until = t.ReconciliationUntil.Time
	}
	if !now.Before(until) {
		return "manual", 0, until
	}
	if t.Mode == "reconciling" || now.Sub(first) >= time.Duration(t.TransientWindowSeconds)*time.Second {
		return "reconciling", time.Duration(t.ReconciliationIntervalSeconds) * time.Second, until
	}
	delay := outboxRetryDelay(10*time.Second, attempts)
	maximum := time.Duration(t.MaxRetrySeconds) * time.Second
	if delay > maximum {
		delay = maximum
	}
	return "active", delay, until
}
