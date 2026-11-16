//go:build integration

package service

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	migrationfiles "github.com/mirainya/Prism/database/migrations"
	"github.com/mirainya/Prism/internal/gateway/billing"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
	"github.com/shopspring/decimal"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This suite owns a newly created schema and never adopts an existing one.
func tokenManagementMySQLFixture(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	dsn := os.Getenv("PRISM_TOKEN_MANAGEMENT_TEST_DSN")
	if dsn == "" {
		t.Skip("PRISM_TOKEN_MANAGEMENT_TEST_DSN is not set")
	}
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid MySQL test DSN")
	}
	host, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil || cfg.Net != "tcp" || host != "127.0.0.1" || port != "13316" || cfg.DBName != "" {
		t.Fatal("token management tests require 127.0.0.1:13316 and no existing database")
	}
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 5*time.Second, 30*time.Second, 30*time.Second
	server, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("open isolated MySQL server")
	}
	t.Cleanup(func() { _ = server.Close() })
	name := "prism_test_token_management_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := server.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatalf("create isolated schema: %v", err)
	}
	t.Cleanup(func() {
		if _, err := server.Exec("DROP DATABASE `" + name + "`"); err != nil {
			t.Errorf("remove owned test schema: %v", err)
		}
	})
	cfg.DBName, cfg.ParseTime, cfg.MultiStatements, cfg.Loc = name, true, true, time.UTC
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("open isolated schema")
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	var previous *gorm.DB
	if model.HasDB() {
		previous = model.DB()
	}
	model.SetDB(db)
	t.Cleanup(func() { model.SetDB(previous) })
	baseline, err := migrationfiles.Files.ReadFile("20261026_120000_clean_schema_baseline.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(string(baseline)); err != nil {
		t.Fatalf("prepare shipped baseline: %v", err)
	}
	pauseSQL, err := migrationfiles.Files.ReadFile("20261107_180000_allow_recoverable_token_pause.sql")
	if err != nil {
		t.Fatal(err)
	}
	store, err := repository.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WithTx(context.Background(), func(tx *sql.Tx) error {
		return store.InitializeBilling(context.Background(), tx, billing.Currency{
			Code: "CNY", Version: 1, FractionDigits: 18, RoundingMode: "half_up", MaxAmount: "99999999999999999999",
		})
	}); err != nil {
		t.Fatal(err)
	}
	return db, string(pauseSQL)
}

func TestMySQLTokenManagementLifecycle(t *testing.T) {
	db, pauseSQL := tokenManagementMySQLFixture(t)
	ctx, svc := context.Background(), NewTokenService()
	owner := model.User{Username: "synthetic-owner", Password: "not-a-login-password", Status: 1, Role: "user"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	expiry, rate := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond), 7
	created, err := svc.CreateToken(owner.ID, &CreateTokenReq{Name: "synthetic-key", Balance: decimal.NewFromInt(5), RateLimit: &rate, ExpiresAt: &expiry})
	if err != nil {
		t.Fatal(err)
	}
	id, scope := created["id"].(uint), ObservabilityScope{ActorUserID: owner.ID}
	readToken := func() model.Token {
		t.Helper()
		var token model.Token
		if err := db.First(&token, id).Error; err != nil {
			t.Fatal(err)
		}
		return token
	}
	original := readToken()
	if original.RateLimit != rate || original.ExpiresAt == nil || !original.ExpiresAt.Equal(expiry) {
		t.Fatal("creation did not preserve rate limit and millisecond expiration")
	}
	// Prove the original production constraint rejects recoverable pauses.
	err = db.Model(&model.Token{}).Where("id=?", id).Update("status", 0).Error
	var checkError *mysqldriver.MySQLError
	if !errors.As(err, &checkError) || checkError.Number != 3819 {
		t.Fatalf("old CHECK must reject pause, got %v", err)
	}
	for range 2 {
		if err := db.Exec(pauseSQL).Error; err != nil {
			t.Fatalf("apply/reapply shipped pause migration: %v", err)
		}
	}
	if token := readToken(); token.Status != original.Status || token.AuthVersion != original.AuthVersion || token.Selector != original.Selector || string(token.SecretDigest) != string(original.SecretDigest) {
		t.Fatal("constraint migration changed the existing credential")
	}
	if err := svc.ChangeTokenAccess(ctx, scope, id, TokenAccessRequest{Action: "pause", ExpectedVersion: "1", Note: "synthetic pause"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateTokenPolicy(ctx, scope, id, tokenPolicy(0, nil, "2")); err != nil {
		t.Fatal(err)
	}
	if token := readToken(); token.Status != 0 || token.AuthVersion != 3 || token.ExpiresAt != nil || token.RateLimit != 0 {
		t.Fatal("policy edit restored the paused key or lost policy values")
	}
	if err := db.Exec("UPDATE billing_accounts SET status='frozen' WHERE user_id=?", owner.ID).Error; err != nil {
		t.Fatal(err)
	}
	resume := TokenAccessRequest{Action: "resume", ExpectedVersion: "3", Note: "synthetic resume"}
	if err := svc.ChangeTokenAccess(ctx, scope, id, resume); !errors.Is(err, ErrTokenResumeBlocked) {
		t.Fatalf("frozen account resumed: %v", err)
	}
	if err := db.Exec("UPDATE billing_accounts SET status='open' WHERE user_id=?", owner.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangeTokenAccess(ctx, scope, id, resume); err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangeTokenAccess(ctx, scope, id, resume); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("stale version was accepted: %v", err)
	}
	history, err := svc.TokenAccessHistory(ctx, scope, id, 1, 20)
	if err != nil || history.Total != 3 || len(history.Items) != 3 || history.Items[0].Note != resume.Note || history.Items[1].Note == "" {
		t.Fatalf("authorization history or audit notes missing: %v", err)
	}
	managed, err := svc.ListManagedTokens(ctx, scope, TokenManagementQuery{TokenID: id})
	if err != nil || managed.Total != 1 || managed.Items[0].State != "active" || managed.Items[0].Budget == nil {
		t.Fatalf("real schema management query failed: %v", err)
	}
	// Audit failure must roll back both the credential and its state event.
	if err := db.Exec("RENAME TABLE audit_events TO test_hidden_audit_events").Error; err != nil {
		t.Fatal(err)
	}
	for _, operation := range []func() error{
		func() error { return svc.UpdateTokenPolicy(ctx, scope, id, tokenPolicy(9, nil, "4")) },
		func() error {
			return svc.ChangeTokenAccess(ctx, scope, id, TokenAccessRequest{Action: "pause", ExpectedVersion: "4", Note: "synthetic failed audit"})
		},
	} {
		if err := operation(); err == nil {
			t.Fatal("operation succeeded without an audit table")
		}
	}
	if err := db.Exec("RENAME TABLE test_hidden_audit_events TO audit_events").Error; err != nil {
		t.Fatal(err)
	}
	var events int64
	if err := db.Table("token_auth_state_events").Where("token_id=?", id).Count(&events).Error; err != nil {
		t.Fatal(err)
	}
	if token := readToken(); token.AuthVersion != 4 || token.Status != 1 || token.RateLimit != 0 || events != 3 {
		t.Fatal("failed audit left partially committed changes")
	}
	// MySQL row locks must allow exactly one request with this version.
	var start sync.WaitGroup
	start.Add(1)
	results := make(chan error, 2)
	go func() {
		start.Wait()
		results <- svc.UpdateTokenPolicy(ctx, scope, id, tokenPolicy(11, nil, "4"))
	}()
	go func() {
		start.Wait()
		results <- svc.ChangeTokenAccess(ctx, scope, id, TokenAccessRequest{Action: "pause", ExpectedVersion: "4", Note: "synthetic concurrent pause"})
	}()
	start.Done()
	first, second := <-results, <-results
	if !((first == nil && errors.Is(second, repository.ErrConflict)) || (second == nil && errors.Is(first, repository.ErrConflict))) {
		t.Fatalf("concurrent changes: first=%v second=%v", first, second)
	}
	if err := svc.ChangeTokenAccess(ctx, scope, id, TokenAccessRequest{Action: "revoke", ExpectedVersion: "5", Note: "synthetic permanent revocation"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangeTokenAccess(ctx, scope, id, TokenAccessRequest{Action: "resume", ExpectedVersion: "6", Note: "synthetic invalid restoration"}); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("permanently revoked key was restored: %v", err)
	}
	if token := readToken(); token.Status != 0 || token.RevokedAt == nil || token.AuthVersion != 6 {
		t.Fatal("revocation state was not preserved")
	}
}
