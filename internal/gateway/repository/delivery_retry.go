package repository

import (
	"context"
	"database/sql"

	"github.com/mirainya/Prism/internal/gateway/delivery"
)

// RequestManagedCopyRetry only creates a delivery action. It cannot change a
// Call, generation attempt or billing reservation. Ownership is rechecked
// while holding the same attempt/delivery locks as the recovery worker.
func (s *Store) RequestManagedCopyRetry(ctx context.Context, publicID string, deliveryID, actor uint64, admin bool) (uint64, error) {
	if s == nil || publicID == "" || deliveryID == 0 || actor == 0 {
		return 0, ErrInvalidInput
	}
	var outboxID uint64
	err := s.WithTx(ctx, func(tx *sql.Tx) error {
		var attemptID uint64
		err := tx.QueryRowContext(ctx, `SELECT d.attempt_id FROM gw_result_deliveries d JOIN gw_api_calls c ON c.id=d.call_id AND c.user_id=d.user_id AND c.token_id=d.token_id WHERE d.id=? AND c.public_id=? AND (? OR c.user_id=?)`, deliveryID, publicID, admin, actor).Scan(&attemptID)
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var lockedID uint64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM gw_api_call_attempts WHERE id=? FOR UPDATE`, attemptID).Scan(&lockedID); err != nil {
			return err
		}
		var mode, state string
		var version uint64
		if err := tx.QueryRowContext(ctx, `SELECT delivery_mode,state,state_version FROM gw_result_deliveries WHERE id=? AND attempt_id=? FOR UPDATE`, deliveryID, attemptID).Scan(&mode, &state, &version); err != nil {
			return err
		}
		if mode != "managed_copy" || state != "delivery_failed" {
			return ErrConflict
		}
		var status string
		err = tx.QueryRowContext(ctx, `SELECT id,status FROM gw_async_outbox WHERE result_delivery_id=? AND state_version=? AND action='reconcile_delivery' LIMIT 1 FOR UPDATE`, deliveryID, version).Scan(&outboxID, &status)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if status == "pending" || status == "dispatching" {
			return nil
		}
		if status != "" {
			// Old actions remain immutable; a manual retry gets a new revision.
			if version == ^uint64(0) {
				return ErrConflict
			}
			if err := requireOneRow(tx.ExecContext(ctx, `UPDATE gw_result_deliveries SET state_version=state_version+1,retry_at=NULL,updated_at=? WHERE id=? AND state='delivery_failed' AND state_version=?`, nowUTC(), deliveryID, version)); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO gw_state_transition_events(result_delivery_id,old_state,new_state,state_version,reason_code,created_at) VALUES (?,'delivery_failed','delivery_failed',?,'operator_delivery_retry',?)`, deliveryID, version+1, nowUTC()); err != nil {
				return err
			}
		}
		outboxID, err = s.scheduleDeliveryReconciliation(ctx, tx, deliveryID, nowUTC(), true)
		return err
	})
	return outboxID, err
}

func (s *Store) RecordManagedCopyRecoveryFailure(ctx context.Context, tx *sql.Tx, item OutboxItem, reason string) error {
	if !delivery.ManagedCopyFailure(reason) || len(reason) > 64 {
		return ErrInvalidInput
	}
	if err := s.AssertDeliveryOutboxLease(ctx, tx, item); err != nil {
		return err
	}
	return requireOneRow(tx.ExecContext(ctx, `UPDATE gw_result_deliveries SET reason_code=?,updated_at=? WHERE id=? AND state='delivery_failed' AND delivery_mode='managed_copy' AND state_version=? AND action_seq=?`, reason, nowUTC(), item.ResultDeliveryID, item.StateVersion, item.ActionSeq))
}
