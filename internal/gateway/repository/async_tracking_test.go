package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/execution"
)

func TestPausedPollRecordsProgressWithoutSchedulingAnotherQuery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := New(db)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT mode,transient_window_seconds`).WithArgs(uint64(9)).WillReturnRows(sqlmock.NewRows([]string{"mode", "window", "reconcile_window", "max_retry", "interval", "first", "until", "error"}).AddRow("paused", 900, 86400, 300, 300, nil, nil, ""))
	mock.ExpectExec(`UPDATE gw_async_executions SET state=`).WithArgs(execution.AsyncRunning, sqlmock.AnyArg(), uint64(9), execution.AsyncAccepted, uint64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO gw_state_transition_events`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	err = store.WithTx(context.Background(), func(tx *sql.Tx) error {
		return store.ScheduleAsyncQuery(context.Background(), tx, 9, execution.AsyncAccepted, execution.AsyncRunning, 3, time.Now().UTC())
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
