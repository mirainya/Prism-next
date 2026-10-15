package runtime

import (
	"bytes"
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/execution"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

func TestRecoverUnknownTaskBindsIdentityAndSchedulesOnlyQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := repository.New(db)
	service, _ := NewWithReadiness(store, func(context.Context) error { return nil })
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT a.call_id,a.id`).WithArgs(uint64(332)).WillReturnRows(sqlmock.NewRows([]string{"call", "attempt"}).AddRow(1191, 910))
	mock.ExpectQuery(`SELECT id FROM billing_reservations`).WithArgs(uint64(1191)).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT status,COALESCE`).WithArgs(uint64(1191)).WillReturnRows(sqlmock.NewRows([]string{"state", "attempt"}).AddRow("indeterminate", 910))
	mock.ExpectQuery(`SELECT state FROM gw_api_call_attempts`).WithArgs(uint64(910)).WillReturnRows(sqlmock.NewRows([]string{"state"}).AddRow("terminated_unknown"))
	mock.ExpectQuery(`SELECT state,state_version FROM gw_async_executions`).WithArgs(uint64(332)).WillReturnRows(sqlmock.NewRows([]string{"state", "version"}).AddRow("terminated_unknown", 4))
	mock.ExpectQuery(`SELECT EXISTS`).WithArgs(uint64(332)).WillReturnRows(sqlmock.NewRows([]string{"pending"}).AddRow(false))
	mock.ExpectQuery(`SELECT encrypted_blob_id FROM gw_upstream_task_identities`).WithArgs(uint64(332)).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(`SELECT upstream_scope_kind,upstream_scope_key`).WithArgs(uint64(332)).WillReturnRows(sqlmock.NewRows([]string{"kind", "key"}).AddRow("credential", "76"))
	mock.ExpectQuery(`SELECT k.id FROM crypto_keyring_state`).WithArgs(uint64(1), uint32(1)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery(`SELECT id,encrypted_blob_id FROM gw_upstream_task_identities`).WithArgs(uint64(332)).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`INSERT INTO encrypted_blobs`).WillReturnResult(sqlmock.NewResult(101, 1))
	mock.ExpectExec(`UPDATE encrypted_blobs`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO encrypted_blob_key_wraps`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT upstream_scope_kind,upstream_scope_key`).WithArgs(uint64(332)).WillReturnRows(sqlmock.NewRows([]string{"kind", "key"}).AddRow("credential", "76"))
	mock.ExpectQuery(`SELECT id,scope_kind,scope_key,encrypted_blob_id`).WithArgs(uint64(332)).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`INSERT INTO gw_upstream_task_identities`).WillReturnResult(sqlmock.NewResult(102, 1))
	mock.ExpectExec(`INSERT INTO gw_upstream_task_id_aliases`).WillReturnResult(sqlmock.NewResult(103, 1))
	mock.ExpectExec(`INSERT INTO gw_async_tracking`).WithArgs(uint64(332)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE gw_async_tracking SET mode=`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO audit_events`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT mode,transient_window_seconds`).WithArgs(uint64(332)).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`UPDATE gw_async_executions SET state=`).WithArgs(execution.AsyncTerminatedUnknown, sqlmock.AnyArg(), uint64(332), execution.AsyncTerminatedUnknown, uint64(4)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO gw_state_transition_events`).WithArgs(uint64(332), execution.AsyncTerminatedUnknown, execution.AsyncTerminatedUnknown, uint64(5), "operator_7_bound_upstream_task", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO gw_async_outbox.*'query','pending'`).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), uint64(332), uint64(5)).WillReturnResult(sqlmock.NewResult(104, 1))
	mock.ExpectCommit()
	key := bytes.Repeat([]byte{11}, 32)
	err = service.RecoverAsyncTask(context.Background(), 332, 7, repository.BlobInput{KeyringID: 1, KEKVersion: 1, KEK: key, HMACKey: key, Plaintext: []byte("task_verified")})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAsyncRecoveryRejectsInconsistentOrAlreadyResolvedParents(t *testing.T) {
	for _, tc := range []struct {
		state   execution.AsyncState
		attempt execution.AttemptState
		call    execution.CallState
		allowed bool
	}{
		{execution.AsyncTerminatedUnknown, execution.AttemptTerminatedUnknown, execution.CallIndeterminate, true},
		{execution.AsyncSubmissionUnknown, execution.AttemptStarted, execution.CallInProgress, true},
		{execution.AsyncManualReview, execution.AttemptRecoveryPending, execution.CallInProgress, true},
		{execution.AsyncTerminatedUnknown, execution.AttemptCompleted, execution.CallCompleted, false},
		{execution.AsyncRunning, execution.AttemptStarted, execution.CallInProgress, true},
		{execution.AsyncSubmissionUnknown, execution.AttemptStarted, execution.CallCompleted, false},
	} {
		if got := asyncRecoveryAllowed(tc.state, tc.attempt, tc.call); got != tc.allowed {
			t.Fatalf("%+v: got %v", tc, got)
		}
	}
}
