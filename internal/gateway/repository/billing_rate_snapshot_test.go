package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/billing"
)

func TestCallRateSnapshotRoundTripsExpressionAndRequestFacts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{"seconds": true, "resolution": true}
	source := "seconds * (resolution=='720p' ? 0.5 : 0.3)"
	expression, err := billing.ParseExpression(source, declared)
	if err != nil {
		t.Fatal(err)
	}
	schedule := billing.RateSchedule{
		Currency: billing.Currency{Code: "CNY", Version: 1, FractionDigits: 2, RoundingMode: "half_even", MaxAmount: "1000"},
		Components: []billing.RateComponent{{
			Code: "video_generation", Unit: "second", Source: billing.QuantityGeneratedSeconds,
			Event: billing.ChargeSucceeded, UnitPrice: "0", PricingMode: billing.PricingModeExpression,
			PricingExpr: source, ExpressionDeclared: declared, Expr: expression, MaxPrice: "15",
			QuantityStep: "0", MaxQuantity: "30",
		}},
	}
	requestFacts := billing.Facts{Expr: billing.ExprEnv{Vars: map[string]billing.ExprValue{
		"seconds":    billing.ExprNumber(mustRepositoryAmount(t, "10")),
		"resolution": billing.ExprString("720p"),
	}}}
	scheduleBody, err := json.Marshal(schedule)
	if err != nil {
		t.Fatal(err)
	}
	factsBody, err := json.Marshal(requestFacts)
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO billing_call_rate_snapshots(reservation_id,sell_schedule,request_facts,created_at) VALUES (?,?,?,?)")).
		WithArgs(uint64(9), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT sell_schedule,request_facts FROM billing_call_rate_snapshots WHERE reservation_id=?")).
		WithArgs(uint64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"sell_schedule", "request_facts"}).AddRow(scheduleBody, factsBody))
	mock.ExpectCommit()

	var restored billing.RateSchedule
	var restoredFacts billing.Facts
	if err := store.WithTx(context.Background(), func(tx *sql.Tx) error {
		if err := store.StoreCallRateSnapshot(context.Background(), tx, 9, schedule, requestFacts); err != nil {
			return err
		}
		var found bool
		var loadErr error
		restored, restoredFacts, found, loadErr = store.loadCallRateSnapshot(context.Background(), tx, 9)
		if loadErr != nil {
			return loadErr
		}
		if !found {
			t.Fatal("snapshot was not found")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if restored.Components[0].Expr == nil || restored.Components[0].PricingExpr != source {
		t.Fatalf("restored expression component=%+v", restored.Components[0])
	}
	if got := restoredFacts.Expr.Vars["resolution"]; got != billing.ExprString("720p") {
		t.Fatalf("restored resolution fact=%+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func mustRepositoryAmount(t *testing.T, value string) billing.Amount {
	t.Helper()
	amount, err := billing.ParseAmount(value, 18, true)
	if err != nil {
		t.Fatal(err)
	}
	return amount
}
