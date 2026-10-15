package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func credentialCatalogInput() CredentialCatalogSourceInput {
	return CredentialCatalogSourceInput{BaseURL: "https://provider.invalid", ExternalGroup: "default", RequestTimeoutMS: 15000, ProtocolDefinitionID: 70,
		Adapter: CatalogAdapterInput{Code: "declarative_catalog_v1", Version: 1, Protocol: "catalog_discovery", ImplementationDigest: strings.Repeat("a", 64), MinimumSemanticVersion: "1.0.0"}}
}

func expectCredentialCatalogBase(mock sqlmock.Sqlmock, active bool) {
	mock.ExpectQuery("SELECT c.channel_id").WithArgs(uint64(40)).WillReturnRows(sqlmock.NewRows([]string{"channel_id", "authorized"}).AddRow(7, active))
	if active {
		mock.ExpectQuery("SELECT protocol_kind,status FROM gw_protocol_definitions").WithArgs(uint64(70)).WillReturnRows(sqlmock.NewRows([]string{"kind", "status"}).AddRow("catalog_discovery", "active"))
	}
}

func prepareCatalogSource(store *Store, in CredentialCatalogSourceInput) (uint64, error) {
	var id uint64
	err := store.WithTx(context.Background(), func(tx *sql.Tx) error {
		var err error
		id, err = store.PrepareCredentialCatalogSource(context.Background(), tx, 40, in, 2)
		return err
	})
	return id, err
}

func TestCredentialCatalogPreparationReusesOnlyThisKeysIdenticalSource(t *testing.T) {
	store, mock := catalogRefreshMock(t)
	for range 2 {
		mock.ExpectBegin()
		expectCredentialCatalogBase(mock, true)
		mock.ExpectQuery("SELECT id,grant_seq,status FROM gw_credential_purpose_grants").WithArgs(uint64(40)).WillReturnRows(sqlmock.NewRows([]string{"id", "seq", "status"}).AddRow(60, 1, "active"))
		mock.ExpectQuery("SELECT s.id FROM gw_catalog_sources").WithArgs(uint64(40), uint64(60), uint64(70), "https://provider.invalid", "default", uint32(15000)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(12))
		mock.ExpectCommit()
		id, err := prepareCatalogSource(store, credentialCatalogInput())
		if err != nil || id != 12 {
			t.Fatalf("id=%d error=%v", id, err)
		}
	}
}

func TestCredentialCatalogPreparationAuthorizesAndCreatesAtomically(t *testing.T) {
	for _, test := range []struct {
		previousSequence uint64
		failProfile      bool
	}{{0, false}, {2, false}, {0, true}} {
		store, mock := catalogRefreshMock(t)
		mock.ExpectBegin()
		expectCredentialCatalogBase(mock, true)
		if test.previousSequence == 0 {
			mock.ExpectQuery("SELECT id,grant_seq,status FROM gw_credential_purpose_grants").WillReturnError(sql.ErrNoRows)
		} else {
			mock.ExpectQuery("SELECT id,grant_seq,status FROM gw_credential_purpose_grants").WillReturnRows(sqlmock.NewRows([]string{"id", "seq", "status"}).AddRow(59, test.previousSequence, "revoked"))
		}
		mock.ExpectQuery("SELECT status FROM gw_credentials").WithArgs(uint64(40)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
		mock.ExpectQuery("SELECT COALESCE\\(MAX\\(grant_seq\\),0\\)").WithArgs(uint64(40), "catalog_discovery").WillReturnRows(sqlmock.NewRows([]string{"seq"}).AddRow(test.previousSequence))
		mock.ExpectExec("INSERT INTO gw_credential_purpose_grants").WithArgs(uint64(40), "catalog_discovery", test.previousSequence+1, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(60, 1))
		mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectQuery("SELECT s.id FROM gw_catalog_sources").WillReturnError(sql.ErrNoRows)
		mock.ExpectQuery("SELECT protocol_kind,status FROM gw_protocol_definitions").WillReturnRows(sqlmock.NewRows([]string{"kind", "status"}).AddRow("catalog_discovery", "active"))
		mock.ExpectQuery("SELECT c.channel_id,c.status,c.current_version_id,cv.status,g.id,g.status,g.purpose").WithArgs(uint64(40)).WillReturnRows(sqlmock.NewRows([]string{"channel_id", "status", "version_id", "version_status", "grant_id", "grant_status", "purpose"}).AddRow(7, "active", 50, "active", 60, "active", "catalog_discovery"))
		mock.ExpectQuery("SELECT status FROM gateway_channels").WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
		mock.ExpectQuery("SELECT id,implementation_digest,minimum_semantic_version FROM gw_adapter_implementations").WillReturnRows(sqlmock.NewRows([]string{"id", "digest", "minimum"}).AddRow(80, strings.Repeat("a", 64), "1.0.0"))
		mock.ExpectExec("INSERT INTO gw_catalog_sources").WillReturnResult(sqlmock.NewResult(12, 1))
		if test.failProfile {
			mock.ExpectExec("INSERT INTO gw_catalog_source_profiles").WillReturnError(ErrConflict)
			mock.ExpectRollback()
		} else {
			mock.ExpectExec("INSERT INTO gw_catalog_source_profiles").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(2, 1))
			mock.ExpectCommit()
		}
		id, err := prepareCatalogSource(store, credentialCatalogInput())
		if test.failProfile {
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("error=%v", err)
			}
		} else if err != nil || id != 12 {
			t.Fatalf("id=%d error=%v", id, err)
		}
	}
}

func TestCredentialCatalogPreparationRejectsInactiveKeyBeforeGranting(t *testing.T) {
	store, mock := catalogRefreshMock(t)
	mock.ExpectBegin()
	expectCredentialCatalogBase(mock, false)
	mock.ExpectRollback()
	if _, err := prepareCatalogSource(store, credentialCatalogInput()); !errors.Is(err, ErrConflict) {
		t.Fatalf("error=%v", err)
	}
}

func TestCredentialCatalogPreparationValidatesConnectionBeforeWriting(t *testing.T) {
	for _, base := range []string{"", "https://user:secret@provider.invalid", "https://provider.invalid/v1", "https://provider.invalid?key=secret", "file:///tmp/key"} {
		store, mock := catalogRefreshMock(t)
		mock.ExpectBegin()
		mock.ExpectRollback()
		in := credentialCatalogInput()
		in.BaseURL = base
		if _, err := prepareCatalogSource(store, in); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("base=%s error=%v", base, err)
		}
	}
}
