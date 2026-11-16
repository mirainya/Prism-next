package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func catalogRefreshMock(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	return store, mock
}

func expectActiveRefreshCatalog(mock sqlmock.Sqlmock, activeID uint64, status string) {
	mock.ExpectQuery("SELECT active_release_id FROM gw_catalog_runtime_state").WillReturnRows(sqlmock.NewRows([]string{"active_release_id"}).AddRow(activeID))
	if activeID == 7 {
		mock.ExpectQuery("SELECT status,config_version,content_hash,semantic_digest FROM gw_catalog_releases").WithArgs(uint64(7)).
			WillReturnRows(sqlmock.NewRows([]string{"status", "config_version", "content_hash", "semantic_digest"}).AddRow(status, 1, "hash", "digest"))
	}
}

func expectRefreshSource(mock sqlmock.Sqlmock, status string, authorized bool) {
	mock.ExpectQuery("SELECT s.status,s.state_version,s.credential_id").WithArgs(uint64(12)).WillReturnRows(sqlmock.NewRows([]string{
		"status", "state_version", "credential_id", "credential_version_id", "grant_id", "contract", "definition_id", "authorized",
	}).AddRow(status, 3, 40, 50, 60, "declarative_catalog_v1", 70, authorized))
	if status == "active" && authorized {
		mock.ExpectQuery("SELECT protocol_kind,status FROM gw_protocol_definitions").WithArgs(uint64(70)).
			WillReturnRows(sqlmock.NewRows([]string{"protocol_kind", "status"}).AddRow("catalog_discovery", "active"))
	}
}

func refreshCatalog(store *Store) (uint64, error) {
	var id uint64
	err := store.WithTx(context.Background(), func(tx *sql.Tx) error {
		var err error
		id, err = store.ScheduleActiveCatalogDiscovery(context.Background(), tx, 7, 12, 2)
		return err
	})
	return id, err
}

type discoveryFingerprintArgument struct{ values *[]string }

func (a discoveryFingerprintArgument) Match(value driver.Value) bool {
	digest, ok := value.(string)
	if !ok || !validHexDigest(digest, 32) {
		return false
	}
	*a.values = append(*a.values, digest)
	return true
}

func expectNewRefresh(mock sqlmock.Sqlmock, runID int64, fingerprints *[]string, insertErr error) {
	mock.ExpectQuery("SELECT r.id,r.state FROM gw_control_plane_runs").WithArgs(uint64(12)).WillReturnError(sql.ErrNoRows)
	// Existing snapshots and their release/source link must not be rewritten.
	mock.ExpectQuery("SELECT id FROM gw_catalog_release_sources").WithArgs(uint64(7), uint64(12)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(41))
	mock.ExpectExec("UPDATE gw_catalog_sources SET nonterminal_run_count=nonterminal_run_count\\+1").WithArgs(sqlmock.AnyArg(), uint64(12)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT c.id,c.status,cv.status,pg.status,pg.purpose").WithArgs(uint64(40), uint64(50), uint64(60)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "status", "version_status", "grant_status", "purpose",
	}).AddRow(40, "active", "active", "active", "catalog_discovery"))
	expectation := mock.ExpectExec("INSERT INTO gw_control_plane_runs").WithArgs("catalog_discovery", uint64(41), uint64(40), nil, discoveryFingerprintArgument{fingerprints}, uint64(3), uint64(50), uint64(60), sqlmock.AnyArg(), sqlmock.AnyArg())
	if insertErr != nil {
		expectation.WillReturnError(insertErr)
		return
	}
	expectation.WillReturnResult(sqlmock.NewResult(runID, 1))
	mock.ExpectExec("INSERT INTO gw_control_plane_run_events").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(1, 1))
}

func TestActiveCatalogRefreshKeepsSeparateRunsForIdenticalConfiguration(t *testing.T) {
	store, mock := catalogRefreshMock(t)
	var fingerprints []string
	for _, runID := range []int64{51, 52} {
		mock.ExpectBegin()
		expectActiveRefreshCatalog(mock, 7, "published")
		expectRefreshSource(mock, "active", true)
		expectNewRefresh(mock, runID, &fingerprints, nil)
		mock.ExpectCommit()
		id, err := refreshCatalog(store)
		if err != nil || id != uint64(runID) {
			t.Fatalf("id=%d error=%v", id, err)
		}
	}
	if len(fingerprints) != 2 || fingerprints[0] == fingerprints[1] {
		t.Fatalf("repeated refresh reused the old run: %v", fingerprints)
	}
}

func TestActiveCatalogRefreshReusesUnfinishedRunWithoutAnotherRequest(t *testing.T) {
	for _, state := range []string{"scheduled", "running", "manual_review"} {
		t.Run(state, func(t *testing.T) {
			store, mock := catalogRefreshMock(t)
			mock.ExpectBegin()
			expectActiveRefreshCatalog(mock, 7, "published")
			expectRefreshSource(mock, "active", true)
			mock.ExpectQuery("SELECT r.id,r.state FROM gw_control_plane_runs").WithArgs(uint64(12)).WillReturnRows(sqlmock.NewRows([]string{"id", "state"}).AddRow(51, state))
			if state == "manual_review" {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			id, err := refreshCatalog(store)
			if state == "manual_review" {
				if !errors.Is(err, ErrConflict) {
					t.Fatalf("error=%v", err)
				}
			} else if err != nil || id != 51 {
				t.Fatalf("id=%d error=%v", id, err)
			}
		})
	}
}

func TestActiveCatalogRefreshRejectsInactiveCatalogSourceOrCredential(t *testing.T) {
	for _, test := range []struct {
		name, releaseStatus, sourceStatus string
		activeID                          uint64
		authorized                        bool
	}{
		{"inactive_catalog", "published", "active", 8, true},
		{"draft_catalog", "draft", "active", 7, true},
		{"disabled_source", "published", "disabled", 7, true},
		{"invalid_key", "published", "active", 7, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, mock := catalogRefreshMock(t)
			mock.ExpectBegin()
			expectActiveRefreshCatalog(mock, test.activeID, test.releaseStatus)
			if test.activeID == 7 && test.releaseStatus == "published" {
				expectRefreshSource(mock, test.sourceStatus, test.authorized)
			}
			mock.ExpectRollback()
			if _, err := refreshCatalog(store); !errors.Is(err, ErrConflict) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestActiveCatalogRefreshRollsBackReservedRunCountOnFailure(t *testing.T) {
	store, mock := catalogRefreshMock(t)
	mock.ExpectBegin()
	expectActiveRefreshCatalog(mock, 7, "published")
	expectRefreshSource(mock, "active", true)
	var fingerprints []string
	failure := errors.New("test run insert failure")
	expectNewRefresh(mock, 51, &fingerprints, failure)
	mock.ExpectRollback()
	if _, err := refreshCatalog(store); !errors.Is(err, failure) {
		t.Fatalf("error=%v", err)
	}
}

func TestActiveCatalogRefreshRollsBackIfSourceLinkCannotBeCreated(t *testing.T) {
	store, mock := catalogRefreshMock(t)
	mock.ExpectBegin()
	expectActiveRefreshCatalog(mock, 7, "published")
	expectRefreshSource(mock, "active", true)
	mock.ExpectQuery("SELECT r.id,r.state FROM gw_control_plane_runs").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT id FROM gw_catalog_release_sources").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO gw_catalog_release_sources")).WithArgs(uint64(7), uint64(12), uint64(3), sqlmock.AnyArg()).WillReturnError(ErrConflict)
	mock.ExpectRollback()
	if _, err := refreshCatalog(store); !errors.Is(err, ErrConflict) {
		t.Fatalf("error=%v", err)
	}
}
