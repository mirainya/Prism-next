//go:build integration

package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/mirainya/Prism/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const reentrantMigrationStart = "20260906_140000"

var nonReentrantMigrations = map[string]string{
	// This migration is already part of managed history. Migration files are
	// immutable once applied, so keep the exception explicit.
	"20260917_160000": "adds columns and a foreign key without existence guards",
	"20260922_090000": "adds the API access log error column without an existence guard",
	"20261013_110000": "creates the singleton error-rule table without existence guards",
	"20261013_160000": "adds the availability timeline column without an existence guard",
}

func TestMySQLMigrationLifecycle(t *testing.T) {
	dsn := os.Getenv("PRISM_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("PRISM_MIGRATION_TEST_DSN is not set")
	}
	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid MySQL test DSN")
	}
	host, _, err := net.SplitHostPort(config.Addr)
	if err != nil || config.Net != "tcp" || !net.ParseIP(host).IsLoopback() {
		t.Fatal("migration integration tests require an isolated loopback MySQL server")
	}
	config.DBName = ""
	server, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	databaseName := fmt.Sprintf("prism_test_migrations_%d", time.Now().UnixNano())
	if _, err := server.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := server.Exec("DROP DATABASE `" + databaseName + "`"); err != nil {
			t.Errorf("clean test database: %v", err)
		}
	})
	config.DBName, config.ParseTime, config.MultiStatements = databaseName, true, true
	dsn = config.FormatDSN()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	var previousModelDB *gorm.DB
	if model.HasDB() {
		previousModelDB = model.DB()
	}
	model.SetDB(db)
	t.Cleanup(func() { model.SetDB(previousModelDB) })
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	migrations, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	applied, err := Up(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied) == 0 || applied[len(applied)-1].Version != migrations[len(migrations)-1].Version {
		t.Fatalf("applied=%#v", applied)
	}
	if err := EnsureCurrent(ctx, db); err != nil {
		t.Fatal(err)
	}
	status, err := Inspect(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if status.Legacy || len(status.Pending) != 0 || len(status.Applied) != len(migrations) || status.Applied[0].Dirty {
		t.Fatalf("status=%#v", status)
	}
	for _, table := range requiredApplicationTables {
		if !db.Migrator().HasTable(table) {
			t.Errorf("missing table %s", table)
		}
	}
	var gatewayKeyrings int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM crypto_keyring_state k
JOIN crypto_key_versions v ON v.keyring_id=k.id AND v.key_version=k.current_version AND v.status='current'
WHERE (k.purpose='gateway-credential' AND v.provider_key_ref='env:PRISM_GATEWAY_KEK_B64')
   OR (k.purpose='gateway-payload' AND v.provider_key_ref='env:PRISM_GATEWAY_PAYLOAD_KEK_B64')`).Scan(&gatewayKeyrings); err != nil {
		t.Fatal(err)
	}
	if gatewayKeyrings != 2 {
		t.Fatalf("gateway keyrings=%d want=2", gatewayKeyrings)
	}
	if !db.Migrator().HasTable(migrationTableName) {
		t.Fatal("missing migration history table")
	}
	if !db.Migrator().HasIndex("conversations", "idx_conversations_canonical_match") {
		t.Fatal("missing canonical conversation match index")
	}
	if _, err := Adopt(ctx, db); err != nil {
		t.Fatalf("adopt already managed database: %v", err)
	}

	reapplied, err := Up(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(reapplied) != 0 {
		t.Fatalf("second migration run applied %#v", reapplied)
	}
	verifyLegacyImportInitializedRuntime(t, db)
	verifyLegacyVideoAssetImport(t, sqlDB)
	verifyLegacyAIFileImport(t, sqlDB)
	verifyChannelAdministration(t, sqlDB)
	verifyGatewaySlotAdmission(t, db)
	verifyLatestMigrationsAreReentrant(t, sqlDB, migrations)
}

func verifyLatestMigrationsAreReentrant(t *testing.T, db *sql.DB, migrations []Migration) {
	t.Helper()
	var enabledBefore, eventsBefore, versionsBefore int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM tokens WHERE status=1`).Scan(&enabledBefore); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM token_auth_state_events`).Scan(&eventsBefore); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COALESCE(SUM(auth_version),0) FROM tokens`).Scan(&versionsBefore); err != nil {
		t.Fatal(err)
	}
	var reapplied int
	for _, migration := range migrations {
		if migration.Version < reentrantMigrationStart {
			continue
		}
		if _, known := nonReentrantMigrations[migration.Version]; known {
			continue
		}
		if _, err := db.Exec(migration.SQL); err != nil {
			t.Fatalf("reapply migration %s: %v", migration.Version, err)
		}
		reapplied++
	}
	if reapplied == 0 {
		t.Fatalf("no migrations found at or after %s", reentrantMigrationStart)
	}
	var enabledAfter, eventsAfter, versionsAfter int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM tokens WHERE status=1`).Scan(&enabledAfter); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM token_auth_state_events`).Scan(&eventsAfter); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT COALESCE(SUM(auth_version),0) FROM tokens`).Scan(&versionsAfter); err != nil {
		t.Fatal(err)
	}
	if enabledAfter != enabledBefore || eventsAfter != eventsBefore || versionsAfter != versionsBefore {
		t.Fatalf("reapplying latest migrations changed auth state: enabled %d->%d events %d->%d versions %d->%d", enabledBefore, enabledAfter, eventsBefore, eventsAfter, versionsBefore, versionsAfter)
	}
}
