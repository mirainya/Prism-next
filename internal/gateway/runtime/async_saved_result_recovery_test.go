package runtime

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

func TestSavedVideoRecoveryRequiresLocalPolicyEvidenceAndZeroSettlement(t *testing.T) {
	valid := savedVideoEvidence{publicID: "video", callState: "failed", attemptState: "failed", asyncState: "failed",
		reason: "provider_result_host_not_allowed", settlement: "0.000000000000000000"}
	if !valid.allowed("video") {
		t.Fatal("valid saved evidence rejected")
	}
	for _, mutate := range []func(*savedVideoEvidence){
		func(e *savedVideoEvidence) { e.reason = "provider_task_failed" },
		func(e *savedVideoEvidence) { e.publicID = "another_video" },
		func(e *savedVideoEvidence) { e.settlement = "3.9" },
		func(e *savedVideoEvidence) { e.resultID = sql.NullInt64{Int64: 12, Valid: true} },
		func(e *savedVideoEvidence) { e.attemptState = "completed" },
		func(e *savedVideoEvidence) { e.callState = "in_progress" },
		func(e *savedVideoEvidence) { e.asyncState = "running" },
	} {
		e := valid
		mutate(&e)
		if e.allowed("video") {
			t.Fatalf("unsafe repair permitted: %+v", e)
		}
	}
}

func TestSavedVideoRecoveryRequiresExplicitZeroChargeApproval(t *testing.T) {
	d := &AsyncDispatcher{}
	if err := d.RestoreSavedVideoResult(context.Background(), 1, 2, 3, "video", "upstream", false); !errors.Is(err, repository.ErrInvalidInput) {
		t.Fatalf("missing approval: %v", err)
	}
}

func TestSavedVideoPreviewRejectsInconsistentEvidenceBeforeReadingPayload(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := repository.New(db)
	service, _ := NewWithReadiness(store, func(context.Context) error { return nil })
	d := &AsyncDispatcher{service: service}
	mock.ExpectQuery(`SELECT c.id,a.id,r.id`).WithArgs(uint64(29408), uint64(332)).
		WillReturnRows(sqlmock.NewRows([]string{"call", "attempt", "resource", "cv", "av", "xv", "cs", "as", "xs", "reason", "public", "amount", "result"}).
			AddRow(1191, 910, 619, 4, 3, 6, "failed", "failed", "failed", "provider_task_failed", "video", "0", nil))
	_, err = d.InspectSavedVideoResult(context.Background(), 332, 29408, "video", "upstream")
	if !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("invalid evidence: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSavedVideoRepairRollsBackAllChangesOnVersionConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := repository.New(db)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE gw_async_executions`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO gw_state_transition_events`).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(`UPDATE gw_api_call_attempts`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	err = store.WithTx(context.Background(), func(tx *sql.Tx) error {
		return restoreSavedVideoStates(context.Background(), tx, 332, 1, savedVideoEvidence{callID: 1191, attemptID: 910, resourceID: 619,
			callVersion: 4, attemptVersion: 3, asyncVersion: 6})
	})
	if !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("version conflict: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSavedVideoRepairCommitsAuditedStatesWithoutBillingWrites(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := repository.New(db)
	mock.ExpectBegin()
	for _, change := range []struct {
		table, column, target string
		id, version           uint64
	}{
		{"gw_async_executions", "async_execution_id", "succeeded", 332, 6},
		{"gw_api_call_attempts", "attempt_id", "completed", 910, 3},
		{"gw_api_calls", "call_id", "completed", 1191, 4},
	} {
		mock.ExpectExec(`UPDATE `+change.table).WithArgs(change.target, sqlmock.AnyArg(), change.id, change.version).WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`INSERT INTO gw_state_transition_events\(`+change.column).WithArgs(change.id, change.target, change.version+1,
			"operator_1_saved_result_zero_charge", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	}
	mock.ExpectExec(`UPDATE gw_video_tasks`).WithArgs(sqlmock.AnyArg(), uint64(619)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	err = store.WithTx(context.Background(), func(tx *sql.Tx) error {
		return restoreSavedVideoStates(context.Background(), tx, 332, 1, savedVideoEvidence{callID: 1191, attemptID: 910, resourceID: 619,
			callVersion: 4, attemptVersion: 3, asyncVersion: 6})
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSavedVideoRecoveryAllowsRepairedResponseWithZeroSettlement(t *testing.T) {
	evidence := savedVideoEvidence{publicID: "video", callState: "failed", attemptState: "failed", asyncState: "failed", reason: "invalid_provider_response", settlement: "0.000000000000000000"}
	if !evidence.allowed("video") {
		t.Fatal("repaired provider response rejected")
	}
	evidence.settlement = "1.5"
	if evidence.allowed("video") {
		t.Fatal("charged result must not be repaired through zero-charge recovery")
	}
}
