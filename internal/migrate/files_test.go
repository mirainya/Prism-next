package migrate

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadCleanBaselineAndForwardMigration(t *testing.T) {
	migrations, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 || migrations[0].Version != "20261015_200000" || migrations[1].Version != "20261015_210000" {
		t.Fatalf("unexpected clean lineage: %#v", migrations)
	}
	history, err := parseRetiredHistory(retiredHistoryJSON)
	if err != nil || len(history.ForwardMigrations) != 1 || migrations[1].Checksum != history.ForwardMigrations[0].ForwardChecksum {
		t.Fatal("forward migration differs from its approved checksum")
	}
	schema, err := loadBaselineSchema()
	if err != nil {
		t.Fatal(err)
	}
	if len(schema.Tables) != 182 || len(schema.Triggers) != 14 {
		t.Fatalf("unexpected schema: tables=%d triggers=%d", len(schema.Tables), len(schema.Triggers))
	}
	for table := range schema.Tables {
		if !strings.Contains(migrations[0].SQL, "CREATE TABLE IF NOT EXISTS `"+table+"`") {
			t.Fatalf("missing baseline table %s", table)
		}
	}
	for _, migration := range migrations {
		if strings.Contains(migration.SQL, "http://") || strings.Contains(migration.SQL, "https://") || strings.Contains(migration.SQL, "DEFINER=") || strings.Contains(migration.SQL, "DELIMITER ") {
			t.Fatal("public SQL contains deployment details or client syntax")
		}
		for _, line := range strings.Split(migration.SQL, "\n") {
			if strings.HasPrefix(line, "INSERT INTO `video_channels`") || strings.HasPrefix(line, "INSERT INTO `gateway_channels`") || strings.HasPrefix(line, "INSERT INTO `gw_credential_pools`") {
				t.Fatal("baseline seeds a private channel")
			}
		}
	}
}

func TestLoadSortsManagedMigrationsAndIgnoresLegacyFiles(t *testing.T) {
	filesystem := fstest.MapFS{
		"20260718_120000_legacy.sql":                {Data: []byte("SELECT 1;")},
		"20261015_200000_clean_schema_baseline.sql": {Data: []byte("CREATE TABLE example (id BIGINT);")},
		"20261016_100000_add_example.sql":           {Data: []byte("ALTER TABLE example ADD name TEXT;")},
	}
	migrations, err := loadFromFS(filesystem)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 || migrations[0].Version != "20261015_200000" || migrations[1].Version != "20261016_100000" {
		t.Fatalf("unexpected migrations: %#v", migrations)
	}
}

func TestLoadRejectsMissingBaseline(t *testing.T) {
	filesystem := fstest.MapFS{"20261016_100000_add_example.sql": {Data: []byte("SELECT 1;")}}
	if _, err := loadFromFS(filesystem); err == nil {
		t.Fatal("expected missing baseline to fail")
	}
}

func TestRetiredManifestContainsOnlyVerifiedHashes(t *testing.T) {
	history, err := parseRetiredHistory(retiredHistoryJSON)
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Migrations) != 118 || len(history.CompatibleThroughVersions) != 1 || history.CompatibleThroughVersions[0] != "20261013_160000" {
		t.Fatal("unexpected retired history checkpoints")
	}
	if strings.Contains(string(retiredHistoryJSON), "https://") || strings.Contains(string(retiredHistoryJSON), "CREATE TABLE") {
		t.Fatal("retired history contains private SQL")
	}
}
