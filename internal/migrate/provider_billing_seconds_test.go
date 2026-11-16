package migrate

import (
	"strings"
	"testing"
)

func TestProviderBillingSecondsMigrationPreservesExistingSources(t *testing.T) {
	migrations, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	baseline := migrations[0].SQL
	if migrations[0].Version == "20260718_150000" {
		// Scoped deployments can retain the legacy lineage while the public
		// source uses the clean baseline. Verify its original forward DDL.
		for _, migration := range migrations {
			if migration.Version != "20261024_160000" {
				continue
			}
			if strings.Count(migration.SQL, "usage.billing_seconds") != 2 ||
				!strings.Contains(migration.SQL, "ALTER TABLE gw_sell_rates") ||
				!strings.Contains(migration.SQL, "ALTER TABLE gw_cost_rates") ||
				strings.Contains(strings.ToUpper(migration.SQL), "UPDATE ") {
				t.Fatal("forward billing-seconds migration must extend both enums without rewriting rates")
			}
			return
		}
		t.Fatal("legacy lineage is missing the billing-seconds migration")
	}
	if strings.Count(baseline, "usage.billing_seconds") != 2 {
		t.Fatalf("baseline must allow billing seconds for both rate tables")
	}
	if strings.Contains(baseline, "ALTER TABLE gw_sell_rates") || strings.Contains(baseline, "ALTER TABLE gw_cost_rates") {
		t.Fatal("billing-seconds support must be part of the clean baseline")
	}
}
