package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

func billingAdminFixture(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	for _, statement := range []string{
		`CREATE TABLE users(id INTEGER PRIMARY KEY,status INTEGER,deleted_at DATETIME)`,
		`CREATE TABLE billing_accounts(id INTEGER PRIMARY KEY,user_id INTEGER,currency_code TEXT,currency_version INTEGER,posted_balance TEXT,held_amount TEXT,credit_limit TEXT,status TEXT,state_version INTEGER,updated_at DATETIME)`,
		`CREATE TABLE billing_account_state_events(id INTEGER PRIMARY KEY AUTOINCREMENT,billing_account_id INTEGER,state_version INTEGER,old_state TEXT,new_state TEXT,reason_code TEXT,created_at DATETIME,UNIQUE(billing_account_id,state_version))`,
		`CREATE TABLE tokens(id INTEGER PRIMARY KEY,user_id INTEGER,status INTEGER,deleted_at DATETIME,revoked_at DATETIME)`,
		`CREATE TABLE token_budget_policy_activations(id INTEGER PRIMARY KEY,token_id INTEGER,activation_seq INTEGER,effective_at DATETIME)`,
		`CREATE TABLE token_budget_windows(id INTEGER PRIMARY KEY,token_id INTEGER,activation_id INTEGER,limit_amount TEXT,used_amount TEXT,held_amount TEXT,window_start DATETIME,window_end DATETIME)`,
		`CREATE TABLE gw_api_calls(id INTEGER PRIMARY KEY,public_id TEXT,token_id INTEGER)`,
		`CREATE TABLE billing_reservations(id INTEGER PRIMARY KEY,call_id INTEGER,billing_account_id INTEGER)`,
		`CREATE TABLE billing_settlements(reservation_id INTEGER PRIMARY KEY,authorized_amount TEXT,actual_amount TEXT,authorization_excess TEXT,debt_after TEXT,budget_excess TEXT,created_at DATETIME)`,
		`CREATE TABLE audit_events(id INTEGER PRIMARY KEY AUTOINCREMENT,actor_type TEXT,actor_user_id INTEGER,action TEXT,resource_type TEXT,resource_id TEXT,outcome TEXT,http_status INTEGER,metadata TEXT,created_at DATETIME)`,
		`INSERT INTO users VALUES (1,1,NULL)`,
		`INSERT INTO billing_accounts VALUES (7,1,'CREDIT',1,'-20.25','3','100','frozen',10,NULL)`,
		`INSERT INTO tokens VALUES (42,1,1,NULL,NULL)`,
		`INSERT INTO gw_api_calls VALUES (31,'call-example',42)`,
		`INSERT INTO billing_reservations VALUES (9,31,7)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	for _, item := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO billing_account_state_events VALUES (20,7,7,'open','frozen','settlement_overrun',?)`, []any{start}},
		{`INSERT INTO token_budget_policy_activations VALUES (1,42,1,?)`, []any{start}},
		{`INSERT INTO token_budget_windows VALUES (2,42,1,'100','20','2',?,NULL)`, []any{start}},
		{`INSERT INTO billing_settlements VALUES (9,'9.57','9.9','0.33','20.25','0',?)`, []any{start}},
	} {
		if _, err := db.Exec(item.query, item.args...); err != nil {
			t.Fatal(err)
		}
	}
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func recoverBillingFixture(store *Store, eventID uint64) error {
	return store.WithTx(context.Background(), func(tx *sql.Tx) error {
		return store.UnfreezeBillingAccount(context.Background(), tx, 1, BillingUnfreezeInput{ExpectedFreezeEventID: eventID, Note: "已核查结算差额"}, 5)
	})
}

func TestBillingAccountDetailShowsFundsAndSettlementEvidence(t *testing.T) {
	store := billingAdminFixture(t)
	detail, err := store.GetBillingAccountDetail(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !detail.CanUnfreeze || detail.Account.Available != "76.75" || detail.Account.StateEventID != 20 || detail.Account.StateChangedAt == nil {
		t.Fatalf("unexpected account: %+v", detail)
	}
	if len(detail.RecentAnomalies) != 1 || detail.RecentAnomalies[0].AuthorizationExcess != "0.33" || detail.RecentAnomalies[0].TokenID != 42 {
		t.Fatalf("settlement evidence=%+v", detail.RecentAnomalies)
	}
	accounts, err := store.LoadBillingAccountSummaries(context.Background(), []uint64{1, 99})
	if err != nil || len(accounts) != 1 {
		t.Fatalf("missing accounts should remain absent: %+v, %v", accounts, err)
	}
}

func TestBillingAccountUnfreezePreservesMoneyAndAuditsActor(t *testing.T) {
	store := billingAdminFixture(t)
	if err := recoverBillingFixture(store, 20); err != nil {
		t.Fatal(err)
	}
	var status, posted, held, credit string
	var version uint64
	if err := store.db.QueryRow(`SELECT status,state_version,posted_balance,held_amount,credit_limit FROM billing_accounts WHERE id=7`).Scan(&status, &version, &posted, &held, &credit); err != nil {
		t.Fatal(err)
	}
	if status != "open" || version != 11 || posted != "-20.25" || held != "3" || credit != "100" {
		t.Fatalf("unexpected account mutation: %s %d %s %s %s", status, version, posted, held, credit)
	}
	var actor uint64
	var metadata string
	if err := store.db.QueryRow(`SELECT actor_user_id,metadata FROM audit_events WHERE action='billing.account.unfreeze'`).Scan(&actor, &metadata); err != nil {
		t.Fatal(err)
	}
	var audit struct {
		FreezeEventID uint64 `json:"freeze_event_id"`
		Note          string `json:"note"`
	}
	if err := json.Unmarshal([]byte(metadata), &audit); err != nil || actor != 5 || audit.FreezeEventID != 20 || audit.Note != "已核查结算差额" {
		t.Fatalf("audit actor=%d metadata=%s err=%v", actor, metadata, err)
	}
	if err := recoverBillingFixture(store, 20); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate recovery should conflict: %v", err)
	}
}

func TestBillingAccountUnfreezeRejectsUnsafeStates(t *testing.T) {
	for _, test := range []struct {
		name, mutation string
	}{
		{"closed", `UPDATE billing_accounts SET status='closed'`},
		{"already_open", `UPDATE billing_accounts SET status='open'`},
		{"disabled_user", `UPDATE users SET status=0`},
		{"unreviewed_reason", `UPDATE billing_account_state_events SET reason_code='reconciliation_failure'`},
		{"missing_evidence", `DELETE FROM billing_account_state_events`},
		{"insufficient_credit", `UPDATE billing_accounts SET credit_limit='20'`},
		{"overspent_budget", `UPDATE token_budget_windows SET used_amount='99'`},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := billingAdminFixture(t)
			if _, err := store.db.Exec(test.mutation); err != nil {
				t.Fatal(err)
			}
			detail, err := store.GetBillingAccountDetail(context.Background(), 1)
			if err != nil || detail.CanUnfreeze || detail.UnfreezeBlockReason == "" {
				t.Fatalf("unsafe state was offered recovery: %+v, %v", detail, err)
			}
			if err := recoverBillingFixture(store, 20); !errors.Is(err, ErrConflict) {
				t.Fatalf("unsafe state accepted: %v", err)
			}
			var count int
			if err := store.db.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("rejected mutation wrote an audit: %d %v", count, err)
			}
		})
	}
}

func TestBillingAccountUnfreezeIgnoresInactiveAndExpiredBudgets(t *testing.T) {
	for _, mutation := range []string{
		`UPDATE tokens SET status=0`,
		`UPDATE tokens SET revoked_at='2020-01-01 00:00:00'`,
		`UPDATE tokens SET deleted_at='2020-01-01 00:00:00'`,
		`UPDATE token_budget_windows SET window_end='2020-01-01 00:00:00'`,
		`INSERT INTO token_budget_policy_activations VALUES (2,42,2,'2020-01-01 00:00:00')`,
	} {
		t.Run(mutation, func(t *testing.T) {
			store := billingAdminFixture(t)
			if _, err := store.db.Exec(`UPDATE token_budget_windows SET used_amount='101'`); err != nil {
				t.Fatal(err)
			}
			if _, err := store.db.Exec(mutation); err != nil {
				t.Fatal(err)
			}
			if err := recoverBillingFixture(store, 20); err != nil {
				t.Fatalf("inactive budget blocked recovery: %v", err)
			}
		})
	}
}

func TestBillingAccountUnfreezeRollsBackWhenAuditFails(t *testing.T) {
	store := billingAdminFixture(t)
	if _, err := store.db.Exec(`CREATE TRIGGER reject_billing_audit BEFORE INSERT ON audit_events BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if err := recoverBillingFixture(store, 20); err == nil {
		t.Fatal("recovery committed without an audit")
	}
	var status string
	var count, version int
	if err := store.db.QueryRow(`SELECT status,state_version FROM billing_accounts WHERE id=7`).Scan(&status, &version); err != nil || status != "frozen" || version != 10 {
		t.Fatalf("account was not rolled back: %s %d %v", status, version, err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM billing_account_state_events`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("state event was not rolled back: %d %v", count, err)
	}
}

func TestBillingAccountUnfreezeUsesExactMoneyAtBudgetBoundary(t *testing.T) {
	t.Run("zero available and exactly used budget remain valid", func(t *testing.T) {
		store := billingAdminFixture(t)
		for _, query := range []string{
			`UPDATE billing_accounts SET credit_limit='23.25'`,
			`UPDATE token_budget_windows SET used_amount='98'`,
		} {
			if _, err := store.db.Exec(query); err != nil {
				t.Fatal(err)
			}
		}
		if err := recoverBillingFixture(store, 20); err != nil {
			t.Fatalf("exact boundary rejected: %v", err)
		}
	})
	t.Run("fractional budget excess must not round away", func(t *testing.T) {
		store := billingAdminFixture(t)
		if _, err := store.db.Exec(`UPDATE token_budget_windows SET limit_amount='100000000.000000000000000001',used_amount='100000000',held_amount='0.000000000000000002'`); err != nil {
			t.Fatal(err)
		}
		if err := recoverBillingFixture(store, 20); !errors.Is(err, ErrConflict) {
			t.Fatalf("fractional excess was accepted: %v", err)
		}
	})
}

func TestBillingAccountUnfreezeRejectsAnOldFreezeAfterRefreezing(t *testing.T) {
	store := billingAdminFixture(t)
	if err := recoverBillingFixture(store, 20); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE billing_accounts SET status='frozen',state_version=12`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO billing_account_state_events(billing_account_id,state_version,old_state,new_state,reason_code,created_at) VALUES (7,12,'open','frozen','settlement_overrun',?)`, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := recoverBillingFixture(store, 20); !errors.Is(err, ErrConflict) {
		t.Fatalf("old incident reopened a newer freeze: %v", err)
	}
}

func TestBillingAccountConcurrentUnfreezeRecordsOneTransition(t *testing.T) {
	store := billingAdminFixture(t)
	var workers sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			results <- recoverBillingFixture(store, 20)
		}()
	}
	workers.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	var audits int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM audit_events`).Scan(&audits); err != nil || successes != 1 || audits != 1 {
		t.Fatalf("successes=%d audits=%d err=%v", successes, audits, err)
	}
}

func TestBillingUnfreezeInputRejectsMissingOrUnboundedEvidence(t *testing.T) {
	for _, input := range []BillingUnfreezeInput{
		{Note: "已核查"}, {ExpectedFreezeEventID: 20},
		{ExpectedFreezeEventID: 20, Note: " \n\t "},
		{ExpectedFreezeEventID: 20, Note: strings.Repeat("字", 501)},
		{ExpectedFreezeEventID: 20, Note: "contains\x00nul"},
	} {
		if input.Validate() == nil {
			t.Fatal("invalid recovery evidence accepted")
		}
	}
}
