package runtime

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

type recordingAttemptDispatcher struct{ sends, recoveries int }

func (d *recordingAttemptDispatcher) DispatchAttempt(context.Context, repository.OutboxItem) error {
	d.sends++
	return nil
}
func (d *recordingAttemptDispatcher) RecoverAttempt(context.Context, repository.OutboxItem) error {
	d.recoveries++
	return nil
}

func TestBackgroundQueueDispatchesImagesAndResponsesToTheirOwners(t *testing.T) {
	for _, kind := range []string{"capability_task", "response"} {
		t.Run(kind, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			store, _ := repository.New(db)
			images, responses := &recordingAttemptDispatcher{}, &recordingAttemptDispatcher{}
			d := AttemptDispatcher{Store: store, Responses: responses, Capabilities: images}
			for range 2 {
				mock.ExpectQuery("SELECT r.resource_kind").WithArgs(uint64(12)).WillReturnRows(sqlmock.NewRows([]string{"kind"}).AddRow(kind))
			}
			if err := d.DispatchAttempt(context.Background(), testAttemptOutboxItem()); err != nil {
				t.Fatal(err)
			}
			if err := d.RecoverAttempt(context.Background(), testAttemptOutboxItem()); err != nil {
				t.Fatal(err)
			}
			active, idle := images, responses
			if kind == "response" {
				active, idle = responses, images
			}
			if active.sends != 1 || active.recoveries != 1 || idle.sends != 0 || idle.recoveries != 0 {
				t.Fatal("wrong queue owner")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBackgroundRecoveryNeverPreparesAnAuthorizedGeneration(t *testing.T) {
	for _, status := range []string{"dispatching", "sent", "unknown", "response_recorded"} {
		t.Run(status, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			store, _ := repository.New(db)
			service, _ := New(store)
			d := &CapabilityDispatcher{service: service, prepareBackground: func(context.Context, repository.BackgroundCapabilityDispatch, []byte, []byte, AsyncAssetLoader) (CapabilityDispatchInput, error) {
				t.Fatal("authorized generation was prepared again")
				return CapabilityDispatchInput{}, nil
			}}
			stop := errors.New("completion database unavailable")
			mock.ExpectQuery("SELECT id,status FROM gw_channel_request_logs").WithArgs(uint64(12), "submit").WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(41, status))
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT call_id FROM gw_api_call_attempts").WithArgs(uint64(12)).WillReturnError(stop)
			mock.ExpectRollback()
			if err := d.RecoverAttempt(context.Background(), testAttemptOutboxItem()); !errors.Is(err, stop) {
				t.Fatalf("err=%v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBackgroundRecoveryResumesOnlyUnsentWork(t *testing.T) {
	for _, status := range []string{"", "not_sent"} {
		t.Run(status, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			store, _ := repository.New(db)
			service, _ := New(store)
			d := &CapabilityDispatcher{service: service, prepareBackground: func(context.Context, repository.BackgroundCapabilityDispatch, []byte, []byte, AsyncAssetLoader) (CapabilityDispatchInput, error) {
				return CapabilityDispatchInput{}, nil
			}}
			rows := sqlmock.NewRows([]string{"id", "status"})
			if status != "" {
				rows.AddRow(41, status)
			}
			mock.ExpectQuery("SELECT id,status FROM gw_channel_request_logs").WithArgs(uint64(12), "submit").WillReturnRows(rows)
			stop := errors.New("reached fixed dispatch reconstruction")
			mock.ExpectQuery("SELECT c.id,a.id,a.credential_id,credential.secret").WithArgs(uint64(12)).WillReturnError(stop)
			if err := d.RecoverAttempt(context.Background(), testAttemptOutboxItem()); !errors.Is(err, stop) {
				t.Fatalf("err=%v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func expectBackgroundAttemptLease(mock sqlmock.Sqlmock, item repository.OutboxItem) {
	mock.ExpectQuery("SELECT id FROM gw_api_calls").WithArgs(item.CallLease.CallID, item.AttemptID, item.LeaseOwner, item.LeaseExpiresAt).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(item.CallLease.CallID))
	mock.ExpectQuery("SELECT call_id,state,state_version FROM gw_api_call_attempts WHERE id=\\? FOR UPDATE").WithArgs(item.AttemptID).WillReturnRows(sqlmock.NewRows([]string{"call_id", "state", "version"}).AddRow(item.CallLease.CallID, "started", item.StateVersion))
	mock.ExpectQuery("SELECT 1 FROM gw_async_outbox").WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
}

func TestBackgroundGenerationCannotAuthorizeASecondSubmit(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store, _ := repository.New(db)
	service, _ := New(store)
	item := testAttemptOutboxItem()
	mock.ExpectBegin()
	expectBackgroundAttemptLease(mock, item)
	mock.ExpectQuery("SELECT id,status FROM gw_channel_request_logs").WithArgs(item.AttemptID).WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(41, "dispatching"))
	mock.ExpectRollback()
	if _, err := service.BeginAttemptOutboxRequest(context.Background(), item, AttemptRequestEvidence{}); !errors.Is(err, ErrSubmissionUncertain) {
		t.Fatalf("err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBackgroundCompletionRejectsExpiredClaim(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store, _ := repository.New(db)
	service, _ := New(store)
	item := testAttemptOutboxItem()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM gw_api_calls").WithArgs(item.CallLease.CallID, item.AttemptID, item.LeaseOwner, item.LeaseExpiresAt).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()
	err := store.WithTx(context.Background(), func(tx *sql.Tx) error {
		return service.completeCapabilityOutbox(context.Background(), tx, item.AttemptID, &item)
	})
	if !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSynchronousRecoveryLeavesDurableBackgroundWorkForItsWorker(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store, _ := repository.New(db)
	service, _ := New(store)
	cutoff := time.Now().UTC().Add(-time.Hour)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT call_id FROM gw_api_call_attempts").WithArgs(uint64(12)).WillReturnRows(sqlmock.NewRows([]string{"call_id"}).AddRow(9))
	mock.ExpectQuery("SELECT id FROM billing_reservations").WithArgs(uint64(9)).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT status FROM gw_api_calls").WithArgs(uint64(9)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("in_progress"))
	mock.ExpectQuery("SELECT call_id,state,state_version,updated_at").WithArgs(uint64(12)).WillReturnRows(sqlmock.NewRows([]string{"call_id", "state", "version", "updated"}).AddRow(9, "started", 1, cutoff.Add(-time.Hour)))
	mock.ExpectQuery("SELECT r.id FROM gw_api_resources r[\\s\\S]*"+regexp.QuoteMeta("NOT EXISTS (SELECT 1 FROM gw_async_outbox")).WithArgs(uint64(12), uint64(9), uint64(12)).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()
	_, err := service.claimStaleCapability(context.Background(), 12, cutoff)
	if !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
