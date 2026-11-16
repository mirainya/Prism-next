package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mirainya/Prism/internal/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func fundsOperationsFixture(t *testing.T) (*gorm.DB, uint, uint) {
	t.Helper()
	db := setupTestDB(t)
	setupUnifiedFundsSchema(t, db)
	if err := db.AutoMigrate(&model.AuditEvent{}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE billing_currency_definitions(currency_code TEXT,definition_version INTEGER,fraction_digits INTEGER,max_amount TEXT,status TEXT)`,
		`INSERT INTO billing_currency_definitions VALUES ('CNY',1,18,'99999999999999999999','active')`,
		`CREATE TABLE billing_events(id INTEGER PRIMARY KEY AUTOINCREMENT,billing_account_id INTEGER,reservation_id INTEGER,call_id INTEGER,event_key TEXT,event_type TEXT,amount TEXT,currency_code TEXT,currency_version INTEGER,state_version INTEGER,posting_rule_id INTEGER,created_at DATETIME,UNIQUE(billing_account_id,event_key))`,
		`CREATE TABLE billing_posting_rules(id INTEGER PRIMARY KEY,event_type TEXT,rule_version INTEGER,currency_code TEXT,currency_version INTEGER,amount_source TEXT,user_direction TEXT,system_account_code TEXT,budget_effect TEXT)`,
		`INSERT INTO billing_posting_rules VALUES (1,'funding_credit',1,'CNY',1,'event_amount','credit','funding_clearing','none'),(2,'reservation_settled',1,'CNY',1,'event_amount','debit','gateway_revenue','consume')`,
		`CREATE TABLE ledger_transactions(id INTEGER PRIMARY KEY AUTOINCREMENT,billing_event_id INTEGER UNIQUE,currency_code TEXT,currency_version INTEGER,state TEXT,created_at DATETIME,posted_at DATETIME)`,
		`CREATE TABLE ledger_entries(id INTEGER PRIMARY KEY AUTOINCREMENT,ledger_transaction_id INTEGER,ledger_account_id INTEGER,currency_code TEXT,currency_version INTEGER,entry_code TEXT,direction TEXT,amount TEXT,created_at DATETIME)`,
		`CREATE TABLE gw_api_calls(id INTEGER PRIMARY KEY,public_id TEXT,user_id INTEGER,token_id INTEGER)`,
		`INSERT INTO ledger_accounts(system_account_code,currency_code,currency_version,balance,created_at) VALUES ('funding_clearing','CNY',1,'0',CURRENT_TIMESTAMP)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	owner := &model.User{Username: "finance-owner", Status: 1}
	if err := db.Create(owner).Error; err != nil {
		t.Fatal(err)
	}
	created, err := NewTokenService().CreateToken(owner.ID, &CreateTokenReq{Name: "finance-key", Balance: decimal.NewFromInt(5)})
	if err != nil {
		t.Fatal(err)
	}
	return db, owner.ID, created["id"].(uint)
}

func fundsRequest(amount string) FundsAdjustmentRequest {
	return FundsAdjustmentRequest{Amount: decimal.RequireFromString(amount), RequestID: uuid.NewString(), Note: "已核查的测试操作"}
}

func TestFundsAdjustmentRejectsInvalidInputBeforeOpeningDatabase(t *testing.T) {
	for _, amount := range []string{"0", "-1", "0.0000000000000000001", "100000000000000000000"} {
		if err := fundsRequest(amount).validate(); !errors.Is(err, ErrInvalidBalanceAmount) {
			t.Fatalf("amount %s error=%v", amount, err)
		}
	}
	for _, change := range []string{"missing_id", "nil_id", "whitespace_note"} {
		request := fundsRequest("1")
		switch change {
		case "missing_id":
			request.RequestID = ""
		case "nil_id":
			request.RequestID = uuid.Nil.String()
		case "whitespace_note":
			request.Note = "  "
		}
		if err := request.validate(); !errors.Is(err, ErrInvalidFundsOperation) {
			t.Fatalf("%s error=%v", change, err)
		}
	}
}

func assertStoredFunds(t *testing.T, db *gorm.DB, query, want string, args ...any) {
	t.Helper()
	var actual string
	if err := db.Raw(query, args...).Scan(&actual).Error; err != nil {
		t.Fatal(err)
	}
	if !decimal.RequireFromString(actual).Equal(decimal.RequireFromString(want)) {
		t.Fatalf("stored amount=%s, want %s", actual, want)
	}
}

func TestFundingRequestReplayDoesNotCreditTwice(t *testing.T) {
	db, userID, _ := fundsOperationsFixture(t)
	svc, ctx := NewUserService(), context.Background()
	request := fundsRequest("10.25")
	for range 2 {
		if err := svc.ApplyUserCredit(ctx, 99, userID, request); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"billing_events", "ledger_transactions", "audit_events"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("%s records=%d error=%v", table, count, err)
		}
	}
	assertStoredFunds(t, db, "SELECT posted_balance FROM billing_accounts WHERE user_id=?", "10.25", userID)
	for _, change := range []string{"amount", "note"} {
		conflict := request
		if change == "amount" {
			conflict.Amount = decimal.NewFromInt(20)
		} else {
			conflict.Note = "其他操作"
		}
		if err := svc.ApplyUserCredit(ctx, 99, userID, conflict); !errors.Is(err, ErrFundsRequestConflict) {
			t.Fatalf("changed %s error=%v", change, err)
		}
	}
	assertStoredFunds(t, db, "SELECT posted_balance FROM billing_accounts WHERE user_id=?", "10.25", userID)
	var audit model.AuditEvent
	if err := db.First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	var details map[string]any
	if err := json.Unmarshal(audit.Metadata, &details); err != nil {
		t.Fatal(err)
	}
	if audit.ActorUserID != 99 || details["before"] != "0" || details["after"] != "10.25" || details["request_id"] != request.RequestID {
		t.Fatalf("business audit=%#v %#v", audit, details)
	}
}

func TestBudgetAdjustmentReplayAndPolicyChangesDoNotAddFunds(t *testing.T) {
	db, userID, tokenID := fundsOperationsFixture(t)
	svc, ctx := NewTokenService(), context.Background()
	request := fundsRequest("2")
	for range 2 {
		if _, err := svc.AdjustTokenBudget(ctx, userID, tokenID, request); err != nil {
			t.Fatal(err)
		}
	}
	assertStoredFunds(t, db, "SELECT limit_amount FROM token_budget_windows WHERE token_id=?", "7", tokenID)
	assertStoredFunds(t, db, "SELECT posted_balance FROM billing_accounts WHERE user_id=?", "0", userID)
	conflict := request
	conflict.Amount = decimal.NewFromInt(3)
	if _, err := svc.AdjustTokenBudget(ctx, userID, tokenID, conflict); !errors.Is(err, ErrFundsRequestConflict) {
		t.Fatalf("different amount error=%v", err)
	}
	conflict = request
	conflict.Note = "不同备注"
	if _, err := svc.AdjustTokenBudget(ctx, userID, tokenID, conflict); !errors.Is(err, ErrFundsRequestConflict) {
		t.Fatalf("different note error=%v", err)
	}
	// Replace the effective policy and its window, then replay the old request.
	for _, query := range []string{
		`INSERT INTO token_budget_policy_activations(token_id,policy_id,activation_seq,effective_at,created_at) SELECT token_id,policy_id,2,effective_at,created_at FROM token_budget_policy_activations LIMIT 1`,
		`INSERT INTO token_budget_windows(token_id,policy_id,activation_id,window_start,limit_amount,created_at) SELECT token_id,policy_id,2,window_start,'50',created_at FROM token_budget_windows LIMIT 1`,
	} {
		if err := db.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}
	result, err := svc.AdjustTokenBudget(ctx, userID, tokenID, request)
	if err != nil || !result.Balance.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("policy replay=%#v error=%v", result, err)
	}
	var events int64
	if err := db.Table("token_budget_adjustment_events").Count(&events).Error; err != nil || events != 1 {
		t.Fatalf("budget events=%d error=%v", events, err)
	}
}

func TestFundsOperationRollsBackWhenBusinessAuditCannotBeWritten(t *testing.T) {
	for _, kind := range []string{"account", "budget"} {
		t.Run(kind, func(t *testing.T) {
			db, userID, tokenID := fundsOperationsFixture(t)
			if err := db.Exec("DROP TABLE audit_events").Error; err != nil {
				t.Fatal(err)
			}
			request := fundsRequest("2")
			var err error
			if kind == "account" {
				err = NewUserService().ApplyUserCredit(context.Background(), 99, userID, request)
			} else {
				_, err = NewTokenService().AdjustTokenBudget(context.Background(), userID, tokenID, request)
			}
			if err == nil {
				t.Fatal("missing business audit must reject the transaction")
			}
			assertStoredFunds(t, db, "SELECT posted_balance FROM billing_accounts WHERE user_id=?", "0", userID)
			assertStoredFunds(t, db, "SELECT limit_amount FROM token_budget_windows WHERE token_id=?", "5", tokenID)
		})
	}
}

func TestFundsHistoryUsesCurrentEventsWithScopeAndSnapshots(t *testing.T) {
	db, userID, tokenID := fundsOperationsFixture(t)
	ctx := context.Background()
	for range 3 {
		if err := NewUserService().ApplyUserCredit(ctx, 99, userID, fundsRequest("1")); err != nil {
			t.Fatal(err)
		}
	}
	service := NewObservabilityService()
	scope := ObservabilityScope{ActorUserID: userID}
	request := &ListFundsEventsRequest{ObservabilityListFilter: ObservabilityListFilter{PageSize: 1}}
	first, err := service.ListFundsEvents(request, scope)
	if err != nil || first.Total != 3 || len(first.Items) != 1 || first.Items[0].Note == "" || first.Items[0].Currency != "CNY" || first.Items[0].Direction != "credit" {
		t.Fatalf("funding history=%#v error=%v", first, err)
	}
	if err := NewUserService().ApplyUserCredit(ctx, 99, userID, fundsRequest("1")); err != nil {
		t.Fatal(err)
	}
	request.SnapshotID, request.Page = &first.SnapshotID, 2
	second, err := service.ListFundsEvents(request, scope)
	if err != nil || second.Total != 3 || second.Items[0].ID >= first.Items[0].ID {
		t.Fatalf("unstable funds snapshot=%#v error=%v", second, err)
	}
	denied := &ListFundsEventsRequest{ObservabilityListFilter: ObservabilityListFilter{UserID: userID + 1}}
	if _, err := service.ListFundsEvents(denied, scope); !errors.Is(err, ErrObservabilityAccessDenied) {
		t.Fatalf("scope escalation error=%v", err)
	}
	other, err := service.ListFundsEvents(&ListFundsEventsRequest{}, ObservabilityScope{ActorUserID: userID + 1})
	if err != nil || other.Total != 0 {
		t.Fatalf("other user funds=%#v error=%v", other, err)
	}
	if _, err := NewTokenService().AdjustTokenBudget(ctx, userID, tokenID, fundsRequest("0.25")); err != nil {
		t.Fatal(err)
	}
	budgets, err := service.ListFundsEvents(&ListFundsEventsRequest{AccountType: "token"}, scope)
	if err != nil || budgets.Total != 1 || budgets.Items[0].Category != "budget_adjustment" || budgets.Items[0].TokenID != tokenID || budgets.Items[0].Before == nil || *budgets.Items[0].Before != "5" {
		t.Fatalf("budget history=%#v error=%v", budgets, err)
	}
	// Holds and settlement are distinct events; only settlement is a debit.
	if err := db.Exec("INSERT INTO gw_api_calls VALUES (42,?,?,?)", "test-call", userID, tokenID).Error; err != nil {
		t.Fatal(err)
	}
	for index, event := range []string{"reservation_created", "reservation_settled", "reservation_released"} {
		if err := db.Exec(`INSERT INTO billing_events(billing_account_id,call_id,event_key,event_type,amount,currency_code,posting_rule_id,created_at) SELECT id,42,?,?,'0.5','CNY',2,? FROM billing_accounts WHERE user_id=?`, fmt.Sprint(index), event, time.Now(), userID).Error; err != nil {
			t.Fatal(err)
		}
	}
	call, err := service.ListFundsEvents(&ListFundsEventsRequest{CallID: "test-call", Direction: "debit"}, scope)
	if err != nil || call.Total != 1 || call.Items[0].Category != "reservation_settled" {
		t.Fatalf("call funds=%#v error=%v", call, err)
	}
}
