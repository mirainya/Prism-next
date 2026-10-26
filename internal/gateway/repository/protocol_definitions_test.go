package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
)

func TestProtocolDigestIsStableAcrossJSONFormatting(t *testing.T) {
	first, err := CanonicalProtocolJSON([]byte(` {"kind":"availability","steps":[],"schema_version":1} `))
	if err != nil {
		t.Fatal(err)
	}
	second, err := CanonicalProtocolJSON([]byte(`{"schema_version": 1, "steps": [], "kind": "availability"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("formatting changed protocol identity")
	}
	digest := sha256.Sum256(first)
	input := ProtocolDefinitionInput{ProtocolCode: "health-v1", ProtocolVersion: 1, ProtocolKind: "availability", DisplayName: "Health", DefinitionJSON: first, DefinitionDigest: hex.EncodeToString(digest[:])}
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
	input.ProtocolKind = "catalog_discovery"
	if !errors.Is(input.Validate(), ErrInvalidInput) {
		t.Fatal("kind mismatch accepted")
	}
	if _, err := CanonicalProtocolJSON([]byte(`{} {}`)); err == nil {
		t.Fatal("second document accepted")
	}
}
func TestProtocolStatusChangeInvalidatesMonitorVersionsAndCache(t *testing.T) {
	for _, next := range []string{"disabled", "active"} {
		t.Run(next, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store, _ := New(db)
			current := "active"
			if next == "active" {
				current = "disabled"
			}
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT status,config_version FROM gw_protocol_definitions").WithArgs(9).WillReturnRows(sqlmock.NewRows([]string{"status", "version"}).AddRow(current, 2))
			mock.ExpectExec("UPDATE gw_protocol_definitions SET status").WithArgs(next, sqlmock.AnyArg(), uint64(9), uint64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("UPDATE gw_availability_sources SET config_version=config_version\\+1").WithArgs(sqlmock.AnyArg(), uint64(9)).WillReturnResult(sqlmock.NewResult(0, 2))
			mock.ExpectExec("DELETE a FROM gw_upstream_availability").WithArgs(uint64(9)).WillReturnResult(sqlmock.NewResult(0, 8))
			mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(1, 1))
			mock.ExpectCommit()
			err = store.WithTx(context.Background(), func(tx *sql.Tx) error {
				return store.TransitionProtocolDefinition(context.Background(), tx, 9, 2, next, 1)
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestLegacyContractsCannotIgnoreBoundProtocolDefinitions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectRollback()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(validateCatalogProtocolBinding(context.Background(), tx, CatalogSourceInput{ContractCode: "legacy_catalog_v1", ProtocolDefinitionID: 1}), ErrInvalidInput) {
		t.Fatal("legacy discovery ignored bound JSON")
	}
	if !errors.Is(validateAvailabilityProtocolBinding(context.Background(), tx, AvailabilitySourceInput{ContractCode: "legacy_health_v1", ProtocolDefinitionID: 1}), ErrInvalidInput) {
		t.Fatal("legacy monitor ignored bound JSON")
	}
	_ = tx.Rollback()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
