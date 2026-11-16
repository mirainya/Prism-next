package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

type isolatedPoolCodeArgument struct{}

func (isolatedPoolCodeArgument) Match(value driver.Value) bool {
	code, ok := value.(string)
	return ok && strings.HasPrefix(code, "key-") && uuid.Validate(strings.TrimPrefix(code, "key-")) == nil
}

func expectIsolatedPool(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT status FROM gateway_channels").WithArgs(uint64(7)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
	mock.ExpectExec("INSERT INTO gw_credential_pools").WithArgs(uint64(7), isolatedPoolCodeArgument{}, "test-key", nil, nil, sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(20, 1))
	mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT channel_id FROM gw_credential_pools").WithArgs(uint64(20)).WillReturnRows(sqlmock.NewRows([]string{"channel_id"}).AddRow(7))
	mock.ExpectQuery("SELECT status FROM gateway_channels").WithArgs(uint64(7)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
	mock.ExpectQuery("SELECT status FROM gw_credential_pools").WithArgs(uint64(20), uint64(7)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
	mock.ExpectQuery("SELECT id FROM gw_credential_secret_identities").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(30))
	mock.ExpectQuery("SELECT status FROM gw_credential_secret_identities").WithArgs(uint64(30)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
	// Existing pool names and model offerings are deliberately never read.
}

func TestIsolatedCredentialCreatesIndependentPoolAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := New(db)
	in := validManagedCredential()
	mock.ExpectBegin()
	expectIsolatedPool(mock)
	mock.ExpectQuery("SELECT id FROM gw_credentials WHERE channel_id=.*credential_code").WithArgs(uint64(7), in.Code).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT c.id FROM gw_credentials c WHERE c.channel_id=.*BINARY c.secret").WithArgs(uint64(7), string(in.Secret), uint64(30)).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT status FROM gw_credential_pools").WithArgs(uint64(20), uint64(7)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
	mock.ExpectExec("INSERT INTO gw_credentials").WithArgs(uint64(7), uint64(20), uint64(30), in.Code, string(in.Secret), nil, nil, uint64(1), sqlmock.AnyArg(), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(40, 1))
	mock.ExpectQuery("SELECT secret_identity_id,status FROM gw_credentials").WithArgs(uint64(40), uint64(7)).WillReturnRows(sqlmock.NewRows([]string{"secret_identity_id", "status"}).AddRow(30, "active"))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(version_no\\),0\\) FROM gw_credential_versions").WithArgs(uint64(40)).WillReturnRows(sqlmock.NewRows([]string{"version_no"}).AddRow(0))
	mock.ExpectExec("INSERT INTO gw_credential_versions").WillReturnResult(sqlmock.NewResult(50, 1))
	mock.ExpectQuery("SELECT id,channel_id,secret_identity_id,current_version_id,status FROM gw_credentials").WithArgs(uint64(40)).WillReturnRows(sqlmock.NewRows([]string{"id", "channel_id", "secret_identity_id", "current_version_id", "status"}).AddRow(40, 7, 30, nil, "active"))
	mock.ExpectQuery("SELECT status,channel_id,secret_identity_id,valid_until FROM gw_credential_versions").WithArgs(uint64(50), uint64(40)).WillReturnRows(sqlmock.NewRows([]string{"status", "channel_id", "secret_identity_id", "valid_until"}).AddRow("preparing", 7, 30, nil))
	mock.ExpectQuery("SELECT id FROM gw_credential_secret_identities").WithArgs(uint64(30)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(30))
	mock.ExpectQuery("SELECT id FROM gw_credential_versions").WithArgs(uint64(40)).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("UPDATE gw_credential_versions SET status='superseded'").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("UPDATE gw_credential_versions SET status='active'").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(state_version\\),0\\)\\+1 FROM gw_credential_version_state_events").WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
	mock.ExpectExec("INSERT INTO gw_credential_version_state_events").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE gw_credentials SET current_version_id").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT status FROM gw_credentials").WithArgs(uint64(40)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(grant_seq\\),0\\)").WillReturnRows(sqlmock.NewRows([]string{"seq"}).AddRow(0))
	mock.ExpectExec("INSERT INTO gw_credential_purpose_grants").WillReturnResult(sqlmock.NewResult(60, 1))
	mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()
	var result IsolatedCredentialResult
	err = store.WithTx(context.Background(), func(tx *sql.Tx) error {
		var err error
		result, err = store.CreateIsolatedManagedCredential(context.Background(), tx, 7, in, 3)
		return err
	})
	if err != nil || result.ID != 40 || result.PoolID != 20 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	body, _ := json.Marshal(result)
	if strings.Contains(string(body), string(in.Secret)) {
		t.Fatal("credential result exposed secret")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestIsolatedCredentialRollsBackPoolOnDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := New(db)
	mock.ExpectBegin()
	expectIsolatedPool(mock)
	mock.ExpectQuery("SELECT id FROM gw_credentials WHERE channel_id=.*credential_code").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT c.id FROM gw_credentials c WHERE c.channel_id=.*BINARY c.secret").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(99))
	mock.ExpectRollback()
	err = store.WithTx(context.Background(), func(tx *sql.Tx) error {
		result, err := store.CreateIsolatedManagedCredential(context.Background(), tx, 7, validManagedCredential(), 3)
		if result != (IsolatedCredentialResult{}) {
			t.Fatal("failed creation returned a partial pool")
		}
		return err
	})
	if !errors.Is(err, ErrDuplicateCredentialSecret) {
		t.Fatalf("error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestIsolatedCredentialRejectsInvalidInputBeforePoolCreation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := New(db)
	mock.ExpectBegin()
	mock.ExpectRollback()
	err = store.WithTx(context.Background(), func(tx *sql.Tx) error {
		in := validManagedCredential()
		in.Secret = nil
		_, err := store.CreateIsolatedManagedCredential(context.Background(), tx, 7, in, 3)
		return err
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
