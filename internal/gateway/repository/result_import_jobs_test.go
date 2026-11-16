package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
)

const managedImportSelect = "SELECT " + resultImportColumns + " FROM gw_result_import_jobs WHERE attempt_id=? AND result_ordinal=?"
const managedImportAttemptLock = "SELECT id FROM gw_api_call_attempts WHERE id=? FOR UPDATE"
const managedImportInsert = "INSERT INTO gw_result_import_jobs(attempt_id,result_ordinal,import_mode,task_key,storage_path,original_filename,source_sha256,storage_binding_sha256,legacy_started,created_at) VALUES (?,?,?,?,?,?,?,?,?,?)"

func managedImportFixture() ManagedURLImport {
	return ManagedURLImport{
		AttemptID: 17, Ordinal: 0, Mode: URLImportAsyncV1, TaskKey: "prism-78034fab-1ed8-4630-96d9-b10cd6650394",
		StoragePath: "prism/gateway-results/17/", OriginalFilename: "video.mp4", SourceSHA256: strings.Repeat("a", 64),
		StorageBindingSHA256: strings.Repeat("c", 64), CreatedAt: time.Now().UTC(),
	}
}

func managedImportRows(in ManagedURLImport) *sqlmock.Rows {
	return sqlmock.NewRows(strings.Split(resultImportColumns, ",")).AddRow(
		in.AttemptID, in.Ordinal, in.Mode, in.TaskKey, in.StoragePath, in.OriginalFilename, in.SourceSHA256, in.StorageBindingSHA256, in.LegacyStarted, in.CreatedAt,
	)
}

func managedImportMock(t *testing.T) (*Store, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		_ = db.Close()
	})
	return store, mock, db
}

func managedImportTx(t *testing.T, db *sql.DB, mock sqlmock.Sqlmock) *sql.Tx {
	t.Helper()
	mock.ExpectBegin()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		mock.ExpectRollback()
		if err := tx.Rollback(); err != nil {
			t.Error(err)
		}
	})
	return tx
}

type managedImportGeneratedKey struct{ captured *string }

func (a managedImportGeneratedKey) Match(value driver.Value) bool {
	key, ok := value.(string)
	if !ok || !strings.HasPrefix(key, "prism-") {
		return false
	}
	id, err := uuid.Parse(strings.TrimPrefix(key, "prism-"))
	if err != nil || id.Version() != 4 || key != "prism-"+id.String() {
		return false
	}
	*a.captured = key
	return true
}

func TestManagedURLImportReserveCreatesImmutableIdentity(t *testing.T) {
	for _, mode := range []string{URLImportAsyncV1, URLImportLegacy} {
		t.Run(mode, func(t *testing.T) {
			store, mock, db := managedImportMock(t)
			tx := managedImportTx(t, db, mock)
			in := managedImportFixture()
			in.Mode, in.TaskKey = mode, "caller-cannot-select-this-key"
			mock.ExpectQuery(managedImportAttemptLock).WithArgs(in.AttemptID).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(in.AttemptID))
			mock.ExpectQuery(managedImportSelect+" FOR UPDATE").WithArgs(in.AttemptID, in.Ordinal).
				WillReturnRows(sqlmock.NewRows(strings.Split(resultImportColumns, ",")))
			var generatedKey string
			mock.ExpectExec(managedImportInsert).WithArgs(in.AttemptID, in.Ordinal, in.Mode,
				managedImportGeneratedKey{&generatedKey}, in.StoragePath, in.OriginalFilename, in.SourceSHA256, in.StorageBindingSHA256, in.LegacyStarted, sqlmock.AnyArg()).
				WillReturnResult(sqlmock.NewResult(0, 1))
			got, err := store.ReserveManagedURLImport(context.Background(), tx, in)
			if err != nil || generatedKey == "" || got.TaskKey != generatedKey {
				t.Fatalf("reservation did not create a stable key: %v", err)
			}
			in.TaskKey, in.CreatedAt = generatedKey, got.CreatedAt
			if !reflect.DeepEqual(got, in) {
				t.Fatal("reservation changed immutable source or destination")
			}
		})
	}
}

func TestManagedURLImportReserveReusesWinnerAcrossCapabilityChanges(t *testing.T) {
	for _, storedMode := range []string{URLImportAsyncV1, URLImportLegacy} {
		t.Run(storedMode, func(t *testing.T) {
			store, mock, db := managedImportMock(t)
			tx := managedImportTx(t, db, mock)
			stored := managedImportFixture()
			stored.Mode = storedMode
			in := stored
			if storedMode == URLImportAsyncV1 {
				in.Mode = URLImportLegacy
			} else {
				in.Mode = URLImportAsyncV1
			}
			in.TaskKey = "prism-" + uuid.NewString()
			mock.ExpectQuery(managedImportAttemptLock).WithArgs(in.AttemptID).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(in.AttemptID))
			mock.ExpectQuery(managedImportSelect+" FOR UPDATE").WithArgs(in.AttemptID, in.Ordinal).
				WillReturnRows(managedImportRows(stored))
			got, err := store.ReserveManagedURLImport(context.Background(), tx, in)
			if err != nil || !reflect.DeepEqual(got, stored) {
				t.Fatalf("reservation replaced the winner: %v", err)
			}
		})
	}
}

func TestManagedURLImportReserveRejectsSourceOrDestinationChanges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ManagedURLImport)
	}{
		{"source", func(v *ManagedURLImport) { v.SourceSHA256 = strings.Repeat("b", 64) }},
		{"storage_binding", func(v *ManagedURLImport) { v.StorageBindingSHA256 = strings.Repeat("d", 64) }},
		{"directory", func(v *ManagedURLImport) { v.StoragePath = "another/directory/" }},
		{"original_filename", func(v *ManagedURLImport) { v.OriginalFilename = "another.mp4" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, mock, db := managedImportMock(t)
			tx := managedImportTx(t, db, mock)
			stored := managedImportFixture()
			in := stored
			tc.mutate(&in)
			mock.ExpectQuery(managedImportAttemptLock).WithArgs(in.AttemptID).
				WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(in.AttemptID))
			mock.ExpectQuery(managedImportSelect+" FOR UPDATE").WithArgs(in.AttemptID, in.Ordinal).
				WillReturnRows(managedImportRows(stored))
			if _, err := store.ReserveManagedURLImport(context.Background(), tx, in); !errors.Is(err, ErrConflict) {
				t.Fatalf("conflict error=%v", err)
			}
		})
	}
}

func TestManagedURLImportReadValidatesStoredIdentity(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ManagedURLImport)
	}{
		{"valid", func(v *ManagedURLImport) {}},
		{"wrong_attempt", func(v *ManagedURLImport) { v.AttemptID++ }},
		{"wrong_ordinal", func(v *ManagedURLImport) { v.Ordinal++ }},
		{"wrong_mode", func(v *ManagedURLImport) { v.Mode = "synchronous" }},
		{"wrong_key", func(v *ManagedURLImport) { v.TaskKey = "not-a-task-key" }},
		{"no_prefix", func(v *ManagedURLImport) { v.TaskKey = strings.TrimPrefix(v.TaskKey, "prism-") }},
		{"noncanonical_uuid", func(v *ManagedURLImport) { v.TaskKey = strings.ToUpper(v.TaskKey) }},
		{"invalid_digest", func(v *ManagedURLImport) { v.SourceSHA256 = strings.Repeat("z", 64) }},
		{"invalid_storage_binding", func(v *ManagedURLImport) { v.StorageBindingSHA256 = "bad" }},
		{"invalid_target", func(v *ManagedURLImport) { v.StoragePath = "../files/" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, mock, _ := managedImportMock(t)
			original := managedImportFixture()
			stored := original
			tc.mutate(&stored)
			mock.ExpectQuery(managedImportSelect).WithArgs(original.AttemptID, original.Ordinal).WillReturnRows(managedImportRows(stored))
			got, err := store.ReadManagedURLImport(context.Background(), original.AttemptID, original.Ordinal)
			if tc.name == "valid" {
				if err != nil || !reflect.DeepEqual(got, original) {
					t.Fatalf("read error=%v", err)
				}
			} else if !errors.Is(err, ErrConflict) {
				t.Fatalf("malformed identity error=%v", err)
			}
		})
	}
}

func TestManagedURLImportReadDatabaseFailures(t *testing.T) {
	dbFailure := errors.New("simulated database unavailable")
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"not_found", ErrNotFound}, {"query_failed", dbFailure}, {"scan_failed", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, mock, _ := managedImportMock(t)
			in := managedImportFixture()
			query := mock.ExpectQuery(managedImportSelect).WithArgs(in.AttemptID, in.Ordinal)
			switch tc.name {
			case "not_found":
				query.WillReturnRows(sqlmock.NewRows(strings.Split(resultImportColumns, ",")))
			case "query_failed":
				query.WillReturnError(dbFailure)
			case "scan_failed":
				query.WillReturnRows(sqlmock.NewRows(strings.Split(resultImportColumns, ",")).AddRow("not-an-id", 0, in.Mode, in.TaskKey, in.StoragePath, in.OriginalFilename, in.SourceSHA256, in.StorageBindingSHA256, in.LegacyStarted, in.CreatedAt))
			}
			if _, err := store.ReadManagedURLImport(context.Background(), in.AttemptID, in.Ordinal); err == nil || tc.err != nil && !errors.Is(err, tc.err) {
				t.Fatalf("read error=%v", err)
			}
		})
	}
}

func TestManagedURLImportReserveDatabaseFailures(t *testing.T) {
	dbFailure := errors.New("simulated database unavailable")
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"attempt_missing", ErrNotFound}, {"attempt_query_failed", dbFailure}, {"wrong_attempt", ErrConflict},
		{"intent_query_failed", dbFailure}, {"malformed_winner", ErrConflict}, {"insert_failed", dbFailure}, {"insert_zero_rows", ErrConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, mock, db := managedImportMock(t)
			tx := managedImportTx(t, db, mock)
			in := managedImportFixture()
			attempt := mock.ExpectQuery(managedImportAttemptLock).WithArgs(in.AttemptID)
			switch tc.name {
			case "attempt_missing":
				attempt.WillReturnRows(sqlmock.NewRows([]string{"id"}))
			case "attempt_query_failed":
				attempt.WillReturnError(dbFailure)
			case "wrong_attempt":
				attempt.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(in.AttemptID + 1))
			default:
				attempt.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(in.AttemptID))
				intent := mock.ExpectQuery(managedImportSelect+" FOR UPDATE").WithArgs(in.AttemptID, in.Ordinal)
				switch tc.name {
				case "intent_query_failed":
					intent.WillReturnError(dbFailure)
				case "malformed_winner":
					stored := in
					stored.TaskKey = "not-a-uuid"
					intent.WillReturnRows(managedImportRows(stored))
				default:
					intent.WillReturnRows(sqlmock.NewRows(strings.Split(resultImportColumns, ",")))
					var key string
					insert := mock.ExpectExec(managedImportInsert).WithArgs(in.AttemptID, in.Ordinal, in.Mode,
						managedImportGeneratedKey{&key}, in.StoragePath, in.OriginalFilename, in.SourceSHA256, in.StorageBindingSHA256, in.LegacyStarted, sqlmock.AnyArg())
					if tc.name == "insert_failed" {
						insert.WillReturnError(dbFailure)
					} else {
						insert.WillReturnResult(sqlmock.NewResult(0, 0))
					}
				}
			}
			if _, err := store.ReserveManagedURLImport(context.Background(), tx, in); !errors.Is(err, tc.err) {
				t.Fatalf("reservation error=%v want=%v", err, tc.err)
			}
		})
	}
}

func TestManagedURLImportInvalidInputDoesNotQuery(t *testing.T) {
	store, mock, db := managedImportMock(t)
	tx := managedImportTx(t, db, mock)
	for _, mutate := range []func(*ManagedURLImport){
		func(v *ManagedURLImport) { v.AttemptID = 0 },
		func(v *ManagedURLImport) { v.Mode = "unknown" },
		func(v *ManagedURLImport) { v.SourceSHA256 = "bad" },
		func(v *ManagedURLImport) { v.StorageBindingSHA256 = "" },
		func(v *ManagedURLImport) { v.StoragePath = "" },
		func(v *ManagedURLImport) { v.StoragePath = "/absolute/" },
		func(v *ManagedURLImport) { v.StoragePath = "missing-trailing-slash" },
		func(v *ManagedURLImport) { v.StoragePath = strings.Repeat("a", 1024) + "/" },
		func(v *ManagedURLImport) { v.StoragePath = "files\\nested/" },
		func(v *ManagedURLImport) { v.StoragePath = "files/ ../nested/" },
		func(v *ManagedURLImport) { v.StoragePath = "files/\n/" },
		func(v *ManagedURLImport) { v.OriginalFilename = "" },
		func(v *ManagedURLImport) { v.OriginalFilename = " ../file.mp4" },
		func(v *ManagedURLImport) { v.OriginalFilename = "file.mp4 " },
		func(v *ManagedURLImport) { v.OriginalFilename = ".." },
		func(v *ManagedURLImport) { v.OriginalFilename = "nested/file.mp4" },
		func(v *ManagedURLImport) { v.OriginalFilename = "file.mp4\x00" },
	} {
		in := managedImportFixture()
		mutate(&in)
		if _, err := store.ReserveManagedURLImport(context.Background(), tx, in); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid input error=%v", err)
		}
	}
	if _, err := store.ReserveManagedURLImport(context.Background(), nil, managedImportFixture()); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil transaction error=%v", err)
	}
	for _, invalid := range []*Store{nil, {}} {
		if _, err := invalid.ReadManagedURLImport(context.Background(), 17, 0); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("unconfigured store error=%v", err)
		}
	}
	if _, err := store.ReadManagedURLImport(context.Background(), 0, 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero attempt error=%v", err)
	}
}
