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
	if strings.Count(baseline, "usage.billing_seconds") != 2 {
		t.Fatalf("baseline must allow billing seconds for both rate tables")
	}
	if strings.Contains(baseline, "ALTER TABLE gw_sell_rates") || strings.Contains(baseline, "ALTER TABLE gw_cost_rates") {
		t.Fatal("billing-seconds support must be part of the clean baseline")
	}
}
