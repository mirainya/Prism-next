//go:build integration

package migrate

import "testing"

func TestCleanBaselineDoesNotInheritDatabaseDefaults(t *testing.T) {
	// Mirror a fresh MySQL 8 installation rather than the legacy database's
	// unicode collation. This must not change the forward schema fingerprint.
	db, _ := openCatalogGuardDatabase(t, "utf8mb4_0900_ai_ci")
	for _, table := range []string{"gw_async_poll_policies", "gw_async_tracking"} {
		var collation string
		if err := db.Raw("SELECT TABLE_COLLATION FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", table).Scan(&collation).Error; err != nil {
			t.Fatal(err)
		}
		if collation != "utf8mb4_unicode_ci" {
			t.Fatalf("table %s inherited collation %s", table, collation)
		}
	}
}
