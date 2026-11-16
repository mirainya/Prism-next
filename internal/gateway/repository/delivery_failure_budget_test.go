package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDeliveryFailureBudgetRequiresCurrentLease(t *testing.T) {
	for _, stage := range []string{"fenced", "increment_failed", "read_failed", "zero", "valid"} {
		t.Run(stage, func(t *testing.T) {
			store, mock, db := managedImportMock(t)
			tx := managedImportTx(t, db, mock)
			item := OutboxItem{ID: 11, ResultDeliveryID: 7, Action: "reconcile_delivery", ActionSeq: 2, StateVersion: 4, Attempts: 101, LeaseOwner: "worker"}
			// This package's helper uses exact query matching.
			leaseSQL := "SELECT 1 FROM gw_async_outbox WHERE id=? AND status='dispatching' AND lease_owner=? AND attempt_count=? AND action_seq=? AND state_version=? AND action=? AND (call_id <=> ?) AND (attempt_id <=> ?) AND (async_execution_id <=> ?) AND (callback_receipt_id <=> ?) AND (result_delivery_id <=> ?) AND lease_expires_at>CURRENT_TIMESTAMP(3) FOR UPDATE"
			lease := mock.ExpectQuery(leaseSQL).WithArgs(uint64(11), "worker", uint64(101), uint64(2), uint64(4), "reconcile_delivery", nil, nil, nil, nil, uint64(7))
			dbFailure := errors.New("simulated database error")
			if stage == "fenced" {
				lease.WillReturnRows(sqlmock.NewRows([]string{"valid"}))
			} else {
				lease.WillReturnRows(sqlmock.NewRows([]string{"valid"}).AddRow(1))
				insert := mock.ExpectExec("INSERT INTO gw_delivery_recovery_failures(outbox_id,failure_count) VALUES (?,1) ON DUPLICATE KEY UPDATE failure_count=failure_count+1").WithArgs(uint64(11))
				if stage == "increment_failed" {
					insert.WillReturnError(dbFailure)
				} else {
					insert.WillReturnResult(sqlmock.NewResult(0, 1))
					read := mock.ExpectQuery("SELECT failure_count FROM gw_delivery_recovery_failures WHERE outbox_id=?").WithArgs(uint64(11))
					if stage == "read_failed" {
						read.WillReturnError(dbFailure)
					} else {
						count := 1
						if stage == "zero" {
							count = 0
						}
						read.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
					}
				}
			}
			count, err := store.CountDeliveryRecoveryFailure(context.Background(), tx, item)
			if stage == "valid" {
				if err != nil || count != 1 {
					t.Fatalf("lease claims consumed the failure budget: count=%d error=%v", count, err)
				}
			} else if err == nil {
				t.Fatal("failed transaction was accepted")
			}
		})
	}
}

func TestLegacyURLImportClaimAllowsOnlyOneTransmission(t *testing.T) {
	for _, affected := range []int64{0, 1} {
		store, mock, db := managedImportMock(t)
		tx := managedImportTx(t, db, mock)
		in := managedImportFixture()
		in.Mode = URLImportLegacy
		mock.ExpectExec("UPDATE gw_result_import_jobs SET legacy_started=1 WHERE attempt_id=? AND result_ordinal=? AND task_key=? AND import_mode='legacy_url' AND legacy_started=0").
			WithArgs(in.AttemptID, in.Ordinal, in.TaskKey).WillReturnResult(sqlmock.NewResult(0, affected))
		claimed, err := store.ClaimLegacyURLImport(context.Background(), tx, in)
		if err != nil || claimed != (affected == 1) {
			t.Fatalf("claimed=%v error=%v", claimed, err)
		}
	}
}

func TestManagedImportModeSelectionPreservesConcurrentWinner(t *testing.T) {
	for _, winner := range []string{URLImportAsyncV1, URLImportLegacy} {
		t.Run(winner, func(t *testing.T) {
			store, mock, db := managedImportMock(t)
			tx := managedImportTx(t, db, mock)
			in := managedImportFixture()
			in.Mode = URLImportDetect
			mock.ExpectExec("UPDATE gw_result_import_jobs SET import_mode=? WHERE attempt_id=? AND result_ordinal=? AND task_key=? AND import_mode='detect'").
				WithArgs(URLImportAsyncV1, in.AttemptID, in.Ordinal, in.TaskKey).WillReturnResult(sqlmock.NewResult(0, 0))
			mock.ExpectQuery("SELECT import_mode,legacy_started FROM gw_result_import_jobs WHERE attempt_id=? AND result_ordinal=? AND task_key=? FOR UPDATE").
				WithArgs(in.AttemptID, in.Ordinal, in.TaskKey).
				WillReturnRows(sqlmock.NewRows([]string{"mode", "started"}).AddRow(winner, winner == URLImportLegacy))
			got, err := store.SelectManagedURLImportMode(context.Background(), tx, in, URLImportAsyncV1)
			if err != nil || got.Mode != winner || got.TaskKey != in.TaskKey || got.LegacyStarted != (winner == URLImportLegacy) || got.StoragePath != in.StoragePath {
				t.Fatalf("concurrent selection replaced the winner: %+v error=%v", got, err)
			}
		})
	}
}
