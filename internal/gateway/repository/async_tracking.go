package repository

import (
	"context"
	"database/sql"
	"time"
)

type AsyncTracking struct {
	Mode                                                string
	TransientWindowSeconds, ReconciliationWindowSeconds uint64
	MaxRetrySeconds, ReconciliationIntervalSeconds      uint64
	FirstErrorAt, ReconciliationUntil                   sql.NullTime
	LastErrorCode                                       string
}

// Snapshot once; later channel edits must not change an existing execution.
func (s *Store) EnsureAsyncTracking(ctx context.Context, tx *sql.Tx, asyncID uint64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO gw_async_tracking
 (async_execution_id,transient_window_seconds,reconciliation_window_seconds,max_retry_seconds,reconciliation_interval_seconds)
 SELECT x.id,COALESCE(p.transient_window_seconds,900),COALESCE(p.reconciliation_window_seconds,86400),
 COALESCE(p.max_retry_seconds,300),COALESCE(p.reconciliation_interval_seconds,300)
 FROM gw_async_executions x JOIN gw_api_call_attempts a ON a.id=x.attempt_id
 JOIN gw_credentials c ON c.id=a.credential_id
 LEFT JOIN gw_async_poll_policies p ON p.channel_id=c.channel_id WHERE x.id=?
 ON DUPLICATE KEY UPDATE async_execution_id=gw_async_tracking.async_execution_id`, asyncID)
	return err
}

func (s *Store) ReadAsyncTracking(ctx context.Context, db DB, asyncID uint64, lock bool) (AsyncTracking, error) {
	var t AsyncTracking
	query := `SELECT mode,transient_window_seconds,reconciliation_window_seconds,max_retry_seconds,reconciliation_interval_seconds,first_error_at,reconciliation_until,last_error_code FROM gw_async_tracking WHERE async_execution_id=?`
	if lock {
		query += ` FOR UPDATE`
	}
	err := db.QueryRowContext(ctx, query, asyncID).Scan(&t.Mode, &t.TransientWindowSeconds, &t.ReconciliationWindowSeconds, &t.MaxRetrySeconds, &t.ReconciliationIntervalSeconds, &t.FirstErrorAt, &t.ReconciliationUntil, &t.LastErrorCode)
	return t, err
}

func (s *Store) SetAsyncTrackingMode(ctx context.Context, tx *sql.Tx, asyncID uint64, mode string, operatorID uint64) error {
	if mode != "active" && mode != "paused" {
		return ErrInvalidInput
	}
	if tx == nil || asyncID == 0 || operatorID == 0 {
		return ErrInvalidInput
	}
	if err := requireOneRow(tx.ExecContext(ctx, `UPDATE gw_async_tracking SET mode=?,first_error_at=NULL,reconciliation_until=NULL,last_error_code='',operator_id=?,updated_at=? WHERE async_execution_id=?`, mode, operatorID, time.Now().UTC(), asyncID)); err != nil {
		return err
	}
	return recordCatalogAdminChange(ctx, tx, operatorID, "unified.video.tracking."+mode, "async_execution", asyncID, map[string]string{"mode": mode})
}
