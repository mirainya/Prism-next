package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadIncludesCleanBaseline(t *testing.T) {
	migrations, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 1 {
		t.Fatalf("managed migrations=%d, want 1", len(migrations))
	}
	baseline := migrations[0]
	if baseline.Version != "20261026_120000" || baseline.Name != "clean_schema_baseline" {
		t.Fatalf("baseline=%#v", baseline)
	}
	hash := sha256.Sum256([]byte(strings.ReplaceAll(baseline.SQL, "\r\n", "\n")))
	if baseline.Checksum != hex.EncodeToString(hash[:]) {
		t.Fatalf("baseline checksum=%s, want %s", baseline.Checksum, hex.EncodeToString(hash[:]))
	}
}

func TestBaselineContainsRequiredApplicationTables(t *testing.T) {
	migrations, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	baseline := migrations[0].SQL
	for _, table := range requiredApplicationTables {
		statement := fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s`", table)
		if !strings.Contains(baseline, statement) {
			t.Errorf("baseline is missing %s", table)
		}
	}
	if count := strings.Count(baseline, "CREATE TABLE IF NOT EXISTS"); count != 185 {
		t.Fatalf("baseline table count=%d, want 185", count)
	}
	if !strings.Contains(baseline, "idx_conversations_canonical_match") {
		t.Fatal("baseline is missing the canonical conversation match index")
	}
	for _, table := range []string{
		"gw_async_poll_policies", "gw_async_tracking", "gw_price_display_settings",
	} {
		if !strings.Contains(baseline, "CREATE TABLE IF NOT EXISTS `"+table+"`") {
			t.Errorf("baseline is missing %s", table)
		}
	}
	if strings.Contains(baseline, "ALTER TABLE ") {
		t.Fatal("clean baseline must not depend on later ALTER TABLE statements")
	}
	if strings.Count(baseline, "TRIGGER `") != 14 {
		t.Fatalf("baseline trigger count=%d, want 14", strings.Count(baseline, "TRIGGER `"))
	}
}

func TestBaselineManifestMatchesTableSet(t *testing.T) {
	schema, err := loadBaselineSchema()
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.Tables) != 185 || len(schema.Triggers) != 14 {
		t.Fatalf("manifest tables=%d triggers=%d, want 185/14", len(schema.Tables), len(schema.Triggers))
	}
	for _, table := range []string{
		"gw_async_poll_policies", "gw_async_tracking", "gw_price_display_settings",
	} {
		if schema.Tables[table] == "" {
			t.Fatalf("manifest is missing %s", table)
		}
	}
}

func TestLoadSortsManagedMigrationsAndIgnoresRetiredFiles(t *testing.T) {
	filesystem := fstest.MapFS{
		"20261025_120000_retired.sql":               {Data: []byte("SELECT 1;")},
		"20261026_120000_clean_schema_baseline.sql": {Data: []byte("CREATE TABLE example (id BIGINT);")},
		"20261026_130000_add_example.sql":           {Data: []byte("ALTER TABLE example ADD name TEXT;")},
	}
	migrations, err := loadFromFS(filesystem)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 || migrations[0].Version != "20261026_120000" || migrations[1].Version != "20261026_130000" {
		t.Fatalf("unexpected migrations: %#v", migrations)
	}
}

func TestLoadRejectsMissingBaseline(t *testing.T) {
	filesystem := fstest.MapFS{
		"20261026_130000_add_example.sql": {Data: []byte("SELECT 1;")},
	}
	if _, err := loadFromFS(filesystem); err == nil {
		t.Fatal("expected missing baseline to fail")
	}
}

func TestPublicBaselineContainsNoDeploymentURLs(t *testing.T) {
	migrations, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(migrations[0].SQL), "http://") || strings.Contains(strings.ToLower(migrations[0].SQL), "https://") {
		t.Fatal("baseline contains a deployment URL")
	}
}
