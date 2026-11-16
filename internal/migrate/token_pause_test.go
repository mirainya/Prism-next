package migrate

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// Execute the shipped CHECK expression against real rows. This catches the
// MySQL constraint missed by service fixtures that use GORM AutoMigrate.
// MySQL ALTER TABLE syntax is covered separately by the integration suite.
func TestTokenPauseMigrationAuthorizationStates(t *testing.T) {
	migrations, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	var updatedCheck string
	for _, migration := range migrations {
		if migration.Version == "20261107_180000" {
			_, updatedCheck, _ = strings.Cut(migration.SQL, "ADD ")
			updatedCheck = strings.TrimSuffix(strings.TrimSpace(updatedCheck), ";")
		}
	}
	if updatedCheck == "" {
		t.Fatal("pause migration is missing")
	}
	var originalCheck string
	for _, line := range strings.Split(migrations[0].SQL, "\n") {
		if strings.Contains(line, "CONSTRAINT `ck_tokens_auth_state`") {
			originalCheck = strings.TrimSuffix(strings.TrimSpace(line), ",")
			break
		}
	}
	for _, item := range []struct {
		name, check string
		paused      bool
	}{{"original", originalCheck, false}, {"updated", updatedCheck, true}} {
		t.Run(item.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, _ := db.DB()
			t.Cleanup(func() { _ = sqlDB.Close() })
			if err := db.Exec(`CREATE TABLE tokens(auth_version INTEGER NOT NULL,selector TEXT,secret_digest BLOB,status INTEGER,revoked_at DATETIME,` + item.check + `)`).Error; err != nil {
				t.Fatal(err)
			}
			for _, state := range []struct {
				name             string
				status           int
				revoked          *string
				version          int
				selector, digest any
				allowed          bool
			}{
				{"enabled", 1, nil, 1, "synthetic-selector", []byte("synthetic-digest"), true},
				{"paused", 0, nil, 2, "synthetic-selector", []byte("synthetic-digest"), item.paused},
				{"revoked", 0, ptrTestString("2026-11-07 00:00:00"), 3, "synthetic-selector", []byte("synthetic-digest"), true},
				{"enabled_revoked", 1, ptrTestString("2026-11-07 00:00:00"), 3, "synthetic-selector", []byte("synthetic-digest"), false},
				{"invalid_status", 2, nil, 1, "synthetic-selector", []byte("synthetic-digest"), false},
				{"zero_version", 1, nil, 0, "synthetic-selector", []byte("synthetic-digest"), false},
				{"missing_selector", 1, nil, 1, nil, []byte("synthetic-digest"), false},
				{"missing_digest", 1, nil, 1, "synthetic-selector", nil, false},
			} {
				err := db.Exec("INSERT INTO tokens(auth_version,selector,secret_digest,status,revoked_at) VALUES(?,?,?,?,?)", state.version, state.selector, state.digest, state.status, state.revoked).Error
				if (err == nil) != state.allowed {
					t.Fatalf("state=%s accepted=%v want=%v", state.name, err == nil, state.allowed)
				}
			}
		})
	}
}

func ptrTestString(value string) *string { return &value }
