package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/mirainya/Prism/internal/gateway/execution"
)

// A repeated queued/running observation still consumes a unique action and
// advances the execution revision; an already-dispatched event is immutable.
func (s *Store) ScheduleAsyncQuery(ctx context.Context, tx *sql.Tx, asyncID uint64, from, to execution.AsyncState, version uint64, at time.Time) error {
	return s.scheduleAsyncQuery(ctx, tx, asyncID, from, to, version, at, "provider_progress")
}

func (s *Store) ScheduleAsyncRecoveryQuery(ctx context.Context, tx *sql.Tx, asyncID, version uint64, reason string) error {
	if reason == "" {
		return ErrInvalidInput
	}
	return s.scheduleAsyncQuery(ctx, tx, asyncID, execution.AsyncTerminatedUnknown, execution.AsyncTerminatedUnknown, version, nowUTC(), reason)
}

func (s *Store) scheduleAsyncQuery(ctx context.Context, tx *sql.Tx, asyncID uint64, from, to execution.AsyncState, version uint64, at time.Time, reason string) error {
	if tx == nil || asyncID == 0 || version == 0 || at.IsZero() {
		return ErrInvalidInput
	}
	// A terminated-unknown execution may be resolved only by terminal evidence.
	// Never let a late non-terminal query reopen it as accepted/running.
	if from == execution.AsyncTerminatedUnknown && to != from {
		return ErrConflict
	}
	if from != execution.AsyncTerminatedUnknown && (from != execution.AsyncAccepted && from != execution.AsyncRunning || to != execution.AsyncAccepted && to != execution.AsyncRunning || from == execution.AsyncRunning && to == execution.AsyncAccepted) {
		return ErrInvalidInput
	}
	queueQuery := true
	tracking, err := s.ReadAsyncTracking(ctx, tx, asyncID, true)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		if tracking.Mode == "paused" || tracking.Mode == "manual" {
			queueQuery = false
		}
		if tracking.Mode == "reconciling" {
			if tracking.ReconciliationUntil.Valid && !nowUTC().Before(tracking.ReconciliationUntil.Time) {
				if _, err := tx.ExecContext(ctx, `UPDATE gw_async_tracking SET mode='manual',updated_at=? WHERE async_execution_id=?`, nowUTC(), asyncID); err != nil {
					return err
				}
				queueQuery = false
			}
			at = nowUTC().Add(time.Duration(tracking.ReconciliationIntervalSeconds) * time.Second)
		}
		if tracking.Mode == "active" {
			if _, err := tx.ExecContext(ctx, `UPDATE gw_async_tracking SET first_error_at=NULL,reconciliation_until=NULL,last_error_code='',updated_at=? WHERE async_execution_id=?`, nowUTC(), asyncID); err != nil {
				return err
			}
		}
	}
	if err := requireOneRow(tx.ExecContext(ctx, `UPDATE gw_async_executions SET state=?,state_version=state_version+1,action_seq=action_seq+1,updated_at=? WHERE id=? AND state=? AND state_version=?`, to, nowUTC(), asyncID, from, version)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO gw_state_transition_events(async_execution_id,old_state,new_state,state_version,reason_code,created_at) VALUES (?,?,?,?,?,?)`, asyncID, from, to, version+1, reason, nowUTC()); err != nil {
		return err
	}
	if !queueQuery {
		return nil
	}
	return requireOneRow(tx.ExecContext(ctx, `INSERT INTO gw_async_outbox(async_execution_id,action_seq,action,status,state_version,available_at,attempt_count,created_at,updated_at) SELECT id,action_seq,'query','pending',state_version,?,0,?,? FROM gw_async_executions WHERE id=? AND state_version=?`, at.UTC(), nowUTC(), nowUTC(), asyncID, version+1))
}
