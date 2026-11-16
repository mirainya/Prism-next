package main

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestCatalogSourceWorkerStartupSupportsIndependentMonitorsAndLaterSources(t *testing.T) {
	for _, test := range []struct {
		name       string
		sources    uint64
		evidence   string
		legacy     bool
		legacyKeys bool
		wantWorker bool
		wantError  bool
	}{
		{name: "monitor without discovery", sources: 1, evidence: "valid", wantWorker: true},
		{name: "no sources yet with ready keys", evidence: "valid", wantWorker: true},
		{name: "empty bootstrap without keys"},
		{name: "empty bootstrap with invalid key", evidence: "invalid"},
		{name: "configured source without key", sources: 1, wantError: true},
		{name: "configured source with invalid key", sources: 1, evidence: "invalid", wantError: true},
		{name: "empty bootstrap with missing legacy keys", evidence: "valid", legacy: true},
		{name: "configured source with missing legacy keys", sources: 1, evidence: "valid", legacy: true, wantError: true},
		{name: "configured source with ready legacy keys", sources: 1, evidence: "valid", legacy: true, legacyKeys: true, wantWorker: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("PRISM_GATEWAY_PAYLOAD_HMAC_B64", "")
			t.Setenv("PRISM_GATEWAY_KEK_B64", "")
			t.Setenv("PRISM_GATEWAY_HMAC_B64", "")
			if test.evidence == "valid" {
				t.Setenv("PRISM_GATEWAY_PAYLOAD_HMAC_B64", base64.StdEncoding.EncodeToString(make([]byte, 32)))
			} else if test.evidence == "invalid" {
				t.Setenv("PRISM_GATEWAY_PAYLOAD_HMAC_B64", "invalid")
			}
			if test.legacyKeys {
				key := base64.StdEncoding.EncodeToString(make([]byte, 32))
				t.Setenv("PRISM_GATEWAY_KEK_B64", key)
				t.Setenv("PRISM_GATEWAY_HMAC_B64", key)
			}
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			// The startup query must count independent monitoring sources as well
			// as discovery sources and unfinished discovery work.
			mock.ExpectQuery(`SELECT .*gw_catalog_sources.*nonterminal_run_count>0.*gw_availability_sources WHERE status='active'`).
				WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(test.sources))
			if test.evidence == "valid" {
				mock.ExpectQuery(`SELECT EXISTS .*FROM gw_credentials`).
					WillReturnRows(sqlmock.NewRows([]string{"required"}).AddRow(test.legacy))
			}
			keys, err := catalogSourceWorkerKeys(context.Background(), db)
			if (err != nil) != test.wantError || (len(keys.EvidenceHMAC) != 0) != test.wantWorker {
				t.Fatalf("worker=%v error=%v; want worker=%v error=%v", len(keys.EvidenceHMAC) != 0, err, test.wantWorker, test.wantError)
			}
			clear(keys.EvidenceHMAC)
			clear(keys.CredentialKEK)
			clear(keys.CredentialHMAC)
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
