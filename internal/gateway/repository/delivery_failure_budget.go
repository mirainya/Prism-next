package repository

import (
	"context"
	"database/sql"
)

// CountDeliveryRecoveryFailure only counts failed work, never a pending poll.
// The lease fence and increment share the caller's transaction.
func (s *Store) CountDeliveryRecoveryFailure(ctx context.Context, tx *sql.Tx, item OutboxItem) (uint64, error) {
	if err := s.AssertDeliveryOutboxLease(ctx, tx, item); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO gw_delivery_recovery_failures(outbox_id,failure_count) VALUES (?,1) ON DUPLICATE KEY UPDATE failure_count=failure_count+1`, item.ID); err != nil {
		return 0, err
	}
	var count uint64
	if err := tx.QueryRowContext(ctx, `SELECT failure_count FROM gw_delivery_recovery_failures WHERE outbox_id=?`, item.ID).Scan(&count); err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, ErrConflict
	}
	return count, nil
}
