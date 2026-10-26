package runtime

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

func trackingRows(mode string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"mode", "window", "reconcile_window", "max_retry", "interval", "first", "until", "error"}).AddRow(mode, 900, 86400, 300, 300, nil, nil, "")
}

func expectActiveTracking(mock sqlmock.Sqlmock, asyncID uint64) {
	mock.ExpectExec(`INSERT INTO gw_async_tracking`).WithArgs(asyncID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT mode,transient_window_seconds`).WithArgs(asyncID).WillReturnRows(trackingRows("active"))
}

func TestQueryRetryPlanSurvivesThreeFailuresAndHasTimeBound(t *testing.T) {
	first := time.Date(2026, 10, 15, 9, 5, 0, 0, time.UTC)
	policy := repository.AsyncTracking{Mode: "active", TransientWindowSeconds: 900, ReconciliationWindowSeconds: 86400, MaxRetrySeconds: 300, ReconciliationIntervalSeconds: 300}
	for _, tc := range []struct {
		elapsed time.Duration
		attempt uint64
		mode    string
		delay   time.Duration
	}{
		{30 * time.Second, 3, "active", 40 * time.Second},
		{10 * time.Minute, 10, "active", 5 * time.Minute},
		{15 * time.Minute, 11, "reconciling", 5 * time.Minute},
		{24 * time.Hour, 300, "manual", 0},
	} {
		mode, delay, _ := queryRetryPlan(policy, first, first.Add(tc.elapsed), tc.attempt)
		if mode != tc.mode || delay != tc.delay {
			t.Fatalf("%+v: got %s/%s", tc, mode, delay)
		}
	}
	policy.Mode = "reconciling"
	policy.ReconciliationUntil = sql.NullTime{Time: first.Add(time.Hour), Valid: true}
	mode, _, _ := queryRetryPlan(policy, first.Add(59*time.Minute), first.Add(time.Hour), 1)
	if mode != "manual" {
		t.Fatal("reconciliation deadline was extended")
	}
}

func TestThirdQueryFailureRetriesWithoutFinalizingOrBilling(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := repository.New(db)
	service, _ := NewWithReadiness(store, func(context.Context) error { return nil })
	item := repository.OutboxItem{ID: 31, AsyncExecutionID: 9, Action: "query", ActionSeq: 4, StateVersion: 3, Attempts: 3, LeaseOwner: "worker"}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT attempt_id FROM gw_async_executions`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(12))
	mock.ExpectQuery(`SELECT id FROM gw_api_call_attempts`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(12))
	mock.ExpectQuery(`SELECT state,state_version,action_seq`).WillReturnRows(sqlmock.NewRows([]string{"state", "version", "seq"}).AddRow("running", 3, 4))
	mock.ExpectQuery(`SELECT 1 FROM gw_async_outbox`).WillReturnRows(sqlmock.NewRows([]string{"valid"}).AddRow(1))
	expectActiveTracking(mock, 9)
	mock.ExpectExec(`UPDATE gw_async_tracking SET mode=`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT 1 FROM gw_async_outbox`).WillReturnRows(sqlmock.NewRows([]string{"valid"}).AddRow(1))
	mock.ExpectExec(`UPDATE gw_async_outbox SET status='pending'`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	err = store.WithTx(context.Background(), func(tx *sql.Tx) error {
		return service.retryTrackedQueryTx(context.Background(), tx, item, "provider_http_error", time.Now().UTC())
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStopTrackingPreservesInflightExchangeAndDoesNotTouchBilling(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := repository.New(db)
	service, _ := NewWithReadiness(store, func(context.Context) error { return nil })
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT attempt_id FROM gw_async_executions`).WithArgs(uint64(9)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(12))
	mock.ExpectQuery(`SELECT state FROM gw_api_call_attempts`).WithArgs(uint64(12)).WillReturnRows(sqlmock.NewRows([]string{"state"}).AddRow("started"))
	mock.ExpectQuery(`SELECT state,state_version FROM gw_async_executions`).WithArgs(uint64(9)).WillReturnRows(sqlmock.NewRows([]string{"state", "version"}).AddRow("running", 3))
	mock.ExpectExec(`INSERT INTO gw_async_tracking`).WithArgs(uint64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE gw_async_tracking SET mode=`).WithArgs("paused", uint64(7), sqlmock.AnyArg(), uint64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO audit_events`).WillReturnResult(sqlmock.NewResult(0, 1))
	// The predicate must only retire pending work, leaving dispatching leases intact.
	mock.ExpectExec(`UPDATE gw_async_outbox .* AND status='pending'`).WithArgs(sqlmock.AnyArg(), uint64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := service.StopAsyncTracking(context.Background(), 9, 7); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestContinueBoundTaskSchedulesQueryWithoutRebindingOrCharging(t *testing.T) {
	for _, pending := range []bool{false, true} {
		t.Run(map[bool]string{false: "resume", true: "duplicate"}[pending], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store, _ := repository.New(db)
			service, _ := NewWithReadiness(store, func(context.Context) error { return nil })
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT a.call_id,a.id`).WithArgs(uint64(9)).WillReturnRows(sqlmock.NewRows([]string{"call", "attempt"}).AddRow(11, 12))
			mock.ExpectQuery(`SELECT id FROM billing_reservations`).WithArgs(uint64(11)).WillReturnError(sql.ErrNoRows)
			mock.ExpectQuery(`SELECT status,COALESCE`).WillReturnRows(sqlmock.NewRows([]string{"state", "attempt"}).AddRow("in_progress", 12))
			mock.ExpectQuery(`SELECT state FROM gw_api_call_attempts`).WillReturnRows(sqlmock.NewRows([]string{"state"}).AddRow("started"))
			mock.ExpectQuery(`SELECT state,state_version FROM gw_async_executions`).WillReturnRows(sqlmock.NewRows([]string{"state", "version"}).AddRow("running", 3))
			mock.ExpectQuery(`SELECT EXISTS`).WillReturnRows(sqlmock.NewRows([]string{"pending"}).AddRow(pending))
			if pending {
				mock.ExpectRollback()
			} else {
				mock.ExpectQuery(`SELECT encrypted_blob_id FROM gw_upstream_task_identities`).WillReturnRows(sqlmock.NewRows([]string{"blob"}).AddRow(19))
				mock.ExpectExec(`INSERT INTO gw_async_tracking`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`UPDATE gw_async_tracking SET mode=`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`INSERT INTO audit_events`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectQuery(`SELECT mode,transient_window_seconds`).WillReturnRows(trackingRows("active"))
				mock.ExpectExec(`UPDATE gw_async_tracking SET first_error_at=NULL`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`UPDATE gw_async_executions SET state=`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`INSERT INTO gw_state_transition_events`).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`INSERT INTO gw_async_outbox.*'query','pending'`).WillReturnResult(sqlmock.NewResult(20, 1))
				mock.ExpectCommit()
			}
			err = service.RecoverAsyncTask(context.Background(), 9, 7, repository.BlobInput{})
			if pending && err != repository.ErrConflict {
				t.Fatalf("duplicate: %v", err)
			}
			if !pending && err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
