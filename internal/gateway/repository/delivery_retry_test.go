package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestManagedCopyRetryRejectsForeignCallBeforeScheduling(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store, _ := New(db)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT d.attempt_id FROM gw_result_deliveries d JOIN gw_api_calls c").
		WithArgs(uint64(7), "call", false, uint64(20)).WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()
	_, err := store.RequestManagedCopyRetry(context.Background(), "call", 7, 20, false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign call: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestManagedCopyRetryReusesPendingActionWithoutBillingWrites(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store, _ := New(db)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT d.attempt_id FROM gw_result_deliveries d JOIN gw_api_calls c").
		WithArgs(uint64(7), "call", false, uint64(10)).WillReturnRows(sqlmock.NewRows([]string{"attempt"}).AddRow(3))
	mock.ExpectQuery("SELECT id FROM gw_api_call_attempts").WithArgs(uint64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	mock.ExpectQuery("SELECT delivery_mode,state,state_version FROM gw_result_deliveries").WithArgs(uint64(7), uint64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"mode", "state", "version"}).AddRow("managed_copy", "delivery_failed", 2))
	mock.ExpectQuery("SELECT id,status FROM gw_async_outbox").WithArgs(uint64(7), uint64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(11, "pending"))
	mock.ExpectCommit()
	id, err := store.RequestManagedCopyRetry(context.Background(), "call", 7, 10, false)
	if err != nil || id != 11 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInlineTransferFailureCanScheduleBeforeAsyncTerminalTransition(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store, _ := New(db)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT d.attempt_id,d.state,d.state_version").WithArgs(uint64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"attempt", "state", "version", "seq", "mode", "source", "policy", "reason", "source_available"}).
			AddRow(3, "delivery_failed", 2, 0, "managed_copy", "inline_response", "fixed", "managed_copy_upload_retry_http_503", false))
	// The response is already committed in this transaction; the async parent
	// may still be submitting. No succeeded-state predicate is permitted here.
	mock.ExpectQuery("SELECT EXISTS \\(SELECT 1 FROM gw_channel_request_logs l WHERE l.attempt_id=\\?").
		WithArgs(uint64(3)).WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT id FROM gw_async_outbox").WithArgs(uint64(7), uint64(2)).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("UPDATE gw_result_deliveries SET action_seq=action_seq\\+1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO gw_async_outbox").WillReturnResult(sqlmock.NewResult(11, 1))
	mock.ExpectCommit()
	err := store.WithTx(context.Background(), func(tx *sql.Tx) error {
		_, err := store.ScheduleDeliveryReconciliation(context.Background(), tx, 7, nowUTC())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoppedTransferCanBeManuallyRetriedWithANewImmutableAction(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store, _ := New(db)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT d.attempt_id FROM gw_result_deliveries d JOIN gw_api_calls c").
		WithArgs(uint64(7), "call", true, uint64(1)).WillReturnRows(sqlmock.NewRows([]string{"attempt"}).AddRow(3))
	mock.ExpectQuery("SELECT id FROM gw_api_call_attempts").WithArgs(uint64(3)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	mock.ExpectQuery("SELECT delivery_mode,state,state_version FROM gw_result_deliveries").WithArgs(uint64(7), uint64(3)).
		WillReturnRows(sqlmock.NewRows([]string{"mode", "state", "version"}).AddRow("managed_copy", "delivery_failed", 2))
	mock.ExpectQuery("SELECT id,status FROM gw_async_outbox").WithArgs(uint64(7), uint64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(11, "dead_letter"))
	mock.ExpectExec("UPDATE gw_result_deliveries SET state_version=state_version\\+1").WithArgs(sqlmock.AnyArg(), uint64(7), uint64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO gw_state_transition_events").WithArgs(uint64(7), uint64(3), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(20, 1))
	mock.ExpectQuery("SELECT d.attempt_id,d.state,d.state_version").WithArgs(uint64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"attempt", "state", "version", "seq", "mode", "source", "policy", "reason", "source_available"}).
			AddRow(3, "delivery_failed", 3, 1, "managed_copy", "remote_url", "fixed", "managed_copy_upload_stop_http_403", true))
	mock.ExpectQuery("SELECT id FROM gw_async_outbox").WithArgs(uint64(7), uint64(3)).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("UPDATE gw_result_deliveries SET action_seq=action_seq\\+1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO gw_async_outbox").WillReturnResult(sqlmock.NewResult(12, 1))
	mock.ExpectCommit()
	id, err := store.RequestManagedCopyRetry(context.Background(), "call", 7, 1, true)
	if err != nil || id != 12 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
