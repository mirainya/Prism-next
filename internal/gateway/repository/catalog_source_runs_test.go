package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestValidateCatalogDiscoveryItemRequiresSelectedGroupMembership(t *testing.T) {
	run := CatalogDiscoveryRun{ExternalGroup: "target-group"}
	item := CatalogDiscoveryItem{Code: "model-a", Groups: []string{"other-group"}, SelectedGroupEnabled: true}
	if err := validateCatalogDiscoveryItem(run, item); err != ErrInvalidInput {
		t.Fatalf("mismatched selected group error=%v", err)
	}
	item.Groups = append(item.Groups, run.ExternalGroup)
	if err := validateCatalogDiscoveryItem(run, item); err != nil {
		t.Fatalf("matching selected group error=%v", err)
	}
}

func TestSaveCatalogSnapshotPreservesAValidEmptyModelList(t *testing.T) {
	store, mock := catalogRefreshMock(t)
	now := time.Now().UTC()
	run := CatalogDiscoveryRun{ID: 51, ReleaseID: 7, ReleaseSourceID: 41, SourceID: 12, StateVersion: 2, LeaseOwner: "test-worker", ContractCode: "declarative_catalog_v1"}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM gw_catalog_discovery_snapshots").WithArgs(uint64(51)).WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("SELECT rs.release_id,rs.id,rs.catalog_source_id,r.state").WithArgs(uint64(51)).WillReturnRows(sqlmock.NewRows([]string{
		"release_id", "release_source_id", "source_id", "state", "state_version", "lease_owner", "contract_code",
	}).AddRow(7, 41, 12, "running", 2, "test-worker", "declarative_catalog_v1"))
	mock.ExpectExec("INSERT INTO gw_catalog_discovery_snapshots").WithArgs(uint64(7), uint64(41), uint64(51), "declarative_catalog_v1", now, strings.Repeat("a", 64), uint32(1), 0, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(61, 1))
	// There are deliberately no model-item or catalog-model writes.
	mock.ExpectExec("INSERT INTO gw_catalog_discovery_review_events").WillReturnResult(sqlmock.NewResult(71, 1))
	mock.ExpectExec("INSERT INTO gw_catalog_discovery_review_state").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT rs.catalog_source_id,r.state,r.state_version,r.lease_owner").WithArgs(uint64(51)).WillReturnRows(sqlmock.NewRows([]string{"source_id", "state", "version", "owner"}).AddRow(12, "running", 2, "test-worker"))
	mock.ExpectExec("UPDATE gw_control_plane_runs SET state=").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO gw_control_plane_run_events").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE gw_catalog_sources SET nonterminal_run_count=nonterminal_run_count-1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	id, err := store.SaveCatalogDiscoverySnapshot(context.Background(), CatalogDiscoverySnapshotInput{
		Run: run, ResponseHMAC: strings.Repeat("a", 64), ResultSchemaVersion: 1, ObservedAt: now, Items: []CatalogDiscoveryItem{},
	})
	if err != nil || id != 61 {
		t.Fatalf("id=%d error=%v", id, err)
	}
}

func TestClaimCatalogDiscoveryCarriesAuthorizationDecisionToWorker(t *testing.T) {
	for _, authorized := range []bool{true, false} {
		store, mock := catalogRefreshMock(t)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT r.id,rs.release_id,rs.id,rs.catalog_source_id").WithArgs(sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{
			"id", "release_id", "release_source_id", "source_id", "state", "state_version", "credential_id", "secret", "blob_id", "contract", "base_url", "group", "definition_id", "definition", "timeout", "authorized",
		}).AddRow(51, 7, 41, 12, "scheduled", 1, 40, "test-only-secret", 0, "declarative_catalog_v1", "https://provider.invalid", "default", 70, `{}`, 10000, authorized))
		mock.ExpectExec("UPDATE gw_control_plane_runs SET state='running'").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("INSERT INTO gw_control_plane_run_events").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()
		run, found, err := store.ClaimCatalogDiscoveryRun(context.Background(), "test-worker", time.Minute)
		if err != nil || !found || run.AuthorizationValid != authorized || run.StateVersion != 2 {
			t.Fatalf("found=%v authorized=%v version=%d error=%v", found, run.AuthorizationValid, run.StateVersion, err)
		}
	}
}
