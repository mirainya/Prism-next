//go:build integration

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	migrationfiles "github.com/mirainya/Prism/database/migrations"
)

const importValidationDatabase = "prism_import_validation_20261107"

// Deliberately separate from production and all earlier integration fixtures.
// Run only after approval of the fixture, migration and repository SQL below.
func TestMySQLManagedImportRecovery(t *testing.T) {
	dsn := os.Getenv("PRISM_IMPORT_TEST_DSN")
	if dsn == "" {
		t.Skip("PRISM_IMPORT_TEST_DSN is not set")
	}
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid isolated test DSN")
	}
	host, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil || cfg.Net != "tcp" || !net.ParseIP(host).IsLoopback() || port != "13316" {
		t.Fatal("import verification requires isolated loopback MySQL on port 13316")
	}
	cfg.DBName, cfg.ParseTime, cfg.MultiStatements = "", true, true
	server, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("cannot initialize the isolated connection")
	}
	t.Cleanup(func() { _ = server.Close() })
	// No IF NOT EXISTS: an existing database must never be adopted or deleted.
	if _, err := server.Exec("CREATE DATABASE `" + importValidationDatabase + "`"); err != nil {
		t.Fatal("cannot create the dedicated empty test database")
	}
	t.Cleanup(func() {
		if _, err := server.Exec("DROP DATABASE `" + importValidationDatabase + "`"); err != nil {
			t.Error("cannot remove the dedicated test database")
		}
	})
	cfg.DBName = importValidationDatabase
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("cannot open the dedicated test database")
	}
	t.Cleanup(func() { _ = db.Close() })
	fixture := `CREATE TABLE gw_api_call_attempts(id BIGINT UNSIGNED PRIMARY KEY) ENGINE=InnoDB;
CREATE TABLE gw_async_outbox (
 id BIGINT UNSIGNED PRIMARY KEY,
 call_id BIGINT UNSIGNED NULL, attempt_id BIGINT UNSIGNED NULL,
 async_execution_id BIGINT UNSIGNED NULL, callback_receipt_id BIGINT UNSIGNED NULL,
 result_delivery_id BIGINT UNSIGNED NULL, status VARCHAR(16) NOT NULL,
 lease_owner VARCHAR(64) NOT NULL, attempt_count BIGINT UNSIGNED NOT NULL,
 action_seq BIGINT UNSIGNED NOT NULL, state_version BIGINT UNSIGNED NOT NULL,
 action VARCHAR(32) NOT NULL, lease_expires_at DATETIME(3) NOT NULL
) ENGINE=InnoDB;
INSERT INTO gw_api_call_attempts(id) VALUES (17);
INSERT INTO gw_async_outbox(id,result_delivery_id,status,lease_owner,attempt_count,action_seq,state_version,action,lease_expires_at)
VALUES (11,7,'dispatching','worker',101,2,4,'reconcile_delivery',DATE_ADD(CURRENT_TIMESTAMP(3),INTERVAL 10 MINUTE));`
	if _, err := db.Exec(fixture); err != nil {
		t.Fatal(err)
	}
	body, err := migrationfiles.Files.ReadFile("20261107_110000_add_result_import_jobs.sql")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
	}
	store, _ := New(db)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	concurrent := func(work func(int) error) {
		t.Helper()
		var group sync.WaitGroup
		failures := make(chan error, 8)
		for i := range 8 {
			group.Add(1)
			go func() { defer group.Done(); failures <- work(i) }()
		}
		group.Wait()
		close(failures)
		for err := range failures {
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	in := managedImportFixture()
	var winners sync.Map
	concurrent(func(i int) error {
		candidate := in
		if i%2 == 0 {
			candidate.Mode = URLImportLegacy
		}
		return store.WithTx(ctx, func(tx *sql.Tx) error {
			job, err := store.ReserveManagedURLImport(ctx, tx, candidate)
			if err == nil {
				winners.Store(job.TaskKey+":"+job.Mode, true)
			}
			return err
		})
	})
	winnerCount := 0
	winners.Range(func(_, _ any) bool { winnerCount++; return true })
	if winnerCount != 1 {
		t.Fatalf("concurrent reservations created %d identities", winnerCount)
	}
	var legacy, detected ManagedURLImport
	if err := store.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		in.Ordinal, in.Mode = 1, URLImportLegacy
		legacy, err = store.ReserveManagedURLImport(ctx, tx, in)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var claims atomic.Int32
	concurrent(func(int) error {
		return store.WithTx(ctx, func(tx *sql.Tx) error {
			won, err := store.ClaimLegacyURLImport(ctx, tx, legacy)
			if won {
				claims.Add(1)
			}
			return err
		})
	})
	if claims.Load() != 1 {
		t.Fatalf("legacy transmission claims=%d, want 1", claims.Load())
	}
	if err := store.WithTx(ctx, func(tx *sql.Tx) error {
		var err error
		in.Ordinal, in.Mode = 2, URLImportDetect
		detected, err = store.ReserveManagedURLImport(ctx, tx, in)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var selected sync.Map
	concurrent(func(i int) error {
		mode := URLImportAsyncV1
		if i%2 == 0 {
			mode = URLImportLegacy
		}
		return store.WithTx(ctx, func(tx *sql.Tx) error {
			job, err := store.SelectManagedURLImportMode(ctx, tx, detected, mode)
			if err == nil {
				selected.Store(job.Mode, true)
			}
			return err
		})
	})
	selectionCount := 0
	selected.Range(func(_, _ any) bool { selectionCount++; return true })
	if selectionCount != 1 {
		t.Fatal("capability selection changed after another worker committed")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("cannot reopen the isolated test database")
	}
	store, _ = New(db)
	if resumed, err := store.ReadManagedURLImport(ctx, 17, 1); err != nil || !resumed.LegacyStarted || resumed.TaskKey != legacy.TaskKey {
		t.Fatalf("reopening lost transmission identity: %v", err)
	}
	item := OutboxItem{ID: 11, ResultDeliveryID: 7, Action: "reconcile_delivery", ActionSeq: 2, StateVersion: 4, Attempts: 101, LeaseOwner: "worker"}
	for want := uint64(1); want <= 5; want++ {
		if err := store.WithTx(ctx, func(tx *sql.Tx) error {
			count, err := store.CountDeliveryRecoveryFailure(ctx, tx, item)
			if err == nil && count != want {
				return fmt.Errorf("failure count=%d, want %d", count, want)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	conflict := legacy
	conflict.SourceSHA256 = strings.Repeat("b", 64)
	if err := store.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := store.ReserveManagedURLImport(ctx, tx, conflict)
		return err
	}); err != ErrConflict {
		t.Fatalf("source conflict was accepted: %v", err)
	}
}
