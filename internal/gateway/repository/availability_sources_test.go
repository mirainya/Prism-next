package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAvailabilitySourceInputRequiresIndependentConnectionSettings(t *testing.T) {
	valid := AvailabilitySourceInput{
		ChannelID: 1, CredentialID: 2, SourceCode: "example-health",
		ContractCode: "example_success_rate_v1", BaseURL: "https://api.example.com", RequestTimeoutMS: 15000,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid source rejected: %v", err)
	}
	invalid := []AvailabilitySourceInput{
		{ChannelID: 1, CredentialID: 2, SourceCode: "bad source", ContractCode: valid.ContractCode, BaseURL: valid.BaseURL, RequestTimeoutMS: 15000},
		{ChannelID: 1, CredentialID: 2, SourceCode: valid.SourceCode, ContractCode: "bad contract", BaseURL: valid.BaseURL, RequestTimeoutMS: 15000},
		{ChannelID: 1, CredentialID: 2, SourceCode: valid.SourceCode, ContractCode: valid.ContractCode, BaseURL: "https://localhost", RequestTimeoutMS: 15000},
		{ChannelID: 1, CredentialID: 2, SourceCode: valid.SourceCode, ContractCode: valid.ContractCode, BaseURL: valid.BaseURL, RequestTimeoutMS: 999},
	}
	for index, item := range invalid {
		if err := item.Validate(); err == nil {
			t.Errorf("invalid source %d unexpectedly validated", index)
		}
	}
}

func TestAvailabilitySourceEnableChecksCurrentCredentialAndGrant(t *testing.T) {
	for _, test := range []struct {
		name              string
		credentialState   string
		versionState      string
		grantState        string
		channelState      string
		credentialChannel uint64
		allowed           bool
	}{
		{"valid replacement grant", "active", "active", "active", "active", 4, true},
		{"disabled credential", "disabled", "active", "active", "active", 4, false},
		{"retired version", "active", "retired", "active", "active", 4, false},
		{"revoked grant", "active", "active", "revoked", "active", 4, false},
		{"disabled channel", "active", "active", "active", "disabled", 4, false},
		{"other channel", "active", "active", "active", "active", 5, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store, _ := New(db)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT status,config_version,channel_id,credential_id,credential_purpose_grant_id,contract_code,COALESCE\\(protocol_definition_id,0\\) FROM gw_availability_sources").WithArgs(14).
				WillReturnRows(sqlmock.NewRows([]string{"status", "version", "channel", "credential", "grant", "contract", "protocol"}).AddRow("disabled", 3, 4, 8, 10, "legacy_health_v1", 0))
			mock.ExpectQuery("SELECT c.channel_id,c.status,v.status,g.id,g.status").WithArgs(8).
				WillReturnRows(sqlmock.NewRows([]string{"channel", "credential_status", "version_status", "grant", "grant_status"}).
					AddRow(test.credentialChannel, test.credentialState, test.versionState, 20, test.grantState))
			mock.ExpectQuery("SELECT status FROM gateway_channels").WithArgs(4).
				WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(test.channelState))
			if test.allowed {
				mock.ExpectExec("UPDATE gw_availability_sources SET status").
					WithArgs("active", uint64(20), sqlmock.AnyArg(), uint64(14), uint64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec("DELETE FROM gw_upstream_availability WHERE availability_source_id").WithArgs(14).WillReturnResult(sqlmock.NewResult(0, 2))
				mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			err = store.WithTx(context.Background(), func(tx *sql.Tx) error {
				return store.TransitionManagedAvailabilitySource(context.Background(), tx, 14, 3, "active", 1)
			})
			if test.allowed && err != nil || !test.allowed && !errors.Is(err, ErrConflict) {
				t.Fatalf("allowed=%v, error=%v", test.allowed, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAvailabilitySourceEditIsVersionedAndInvalidatesPreviousObservations(t *testing.T) {
	for _, stale := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		store, _ := New(db)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT channel_id,source_code,contract_code,COALESCE\\(protocol_definition_id,0\\),config_version FROM gw_availability_sources").WithArgs(14).
			WillReturnRows(sqlmock.NewRows([]string{"channel", "source", "contract", "protocol", "version"}).AddRow(4, "health-main", "example_success_rate_v1", 0, 3))
		input := AvailabilitySourceUpdate{ExpectedVersion: 3, CredentialID: 8, BaseURL: "https://new.example.com/", RequestTimeoutMS: 12000}
		if stale {
			input.ExpectedVersion = 2
			mock.ExpectRollback()
		} else {
			mock.ExpectQuery("SELECT c.channel_id,c.status,v.status,g.id,g.status").WithArgs(8).
				WillReturnRows(sqlmock.NewRows([]string{"channel", "credential_status", "version_status", "grant", "grant_status"}).AddRow(4, "active", "active", 20, "active"))
			mock.ExpectQuery("SELECT status FROM gateway_channels").WithArgs(4).
				WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
			mock.ExpectExec("UPDATE gw_availability_sources SET credential_id").
				WithArgs(uint64(8), uint64(20), "https://new.example.com", uint32(12000), "example_success_rate_v1", nil, sqlmock.AnyArg(), uint64(14), uint64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec("DELETE FROM gw_upstream_availability WHERE availability_source_id").WithArgs(14).WillReturnResult(sqlmock.NewResult(0, 2))
			mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
		}
		err = store.WithTx(context.Background(), func(tx *sql.Tx) error {
			return store.UpdateManagedAvailabilitySource(context.Background(), tx, 14, input, 1)
		})
		if stale && !errors.Is(err, ErrConflict) || !stale && err != nil {
			t.Fatalf("stale=%v, error=%v", stale, err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}

func TestAvailabilitySourceDisableInvalidatesCacheAtomically(t *testing.T) {
	cacheFailure := errors.New("cache unavailable")
	for _, failDelete := range []bool{false, true} {
		name := "cache cleared"
		if failDelete {
			name = "cache error rolls back status change"
		}
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store, _ := New(db)
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT status,config_version,channel_id,credential_id,credential_purpose_grant_id,contract_code,COALESCE\(protocol_definition_id,0\) FROM gw_availability_sources`).WithArgs(14).
				WillReturnRows(sqlmock.NewRows([]string{"status", "version", "channel", "credential", "grant", "contract", "protocol"}).AddRow("active", 3, 4, 8, 10, "declarative_availability_v1", 9))
			mock.ExpectExec("UPDATE gw_availability_sources SET status").
				WithArgs("disabled", uint64(10), sqlmock.AnyArg(), uint64(14), uint64(3)).WillReturnResult(sqlmock.NewResult(0, 1))
			deletion := mock.ExpectExec("DELETE FROM gw_upstream_availability WHERE availability_source_id").WithArgs(14)
			if failDelete {
				deletion.WillReturnError(cacheFailure)
				mock.ExpectRollback()
			} else {
				deletion.WillReturnResult(sqlmock.NewResult(0, 2))
				mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()
			}
			err = store.WithTx(context.Background(), func(tx *sql.Tx) error {
				return store.TransitionManagedAvailabilitySource(context.Background(), tx, 14, 3, "disabled", 1)
			})
			if failDelete && !errors.Is(err, cacheFailure) || !failDelete && err != nil {
				t.Fatalf("failDelete=%v error=%v", failDelete, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
