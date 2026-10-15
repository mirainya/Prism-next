package billing

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func expressionSchedule(t *testing.T, source, maxPrice string) (RateSchedule, map[string]bool) {
	t.Helper()
	declared := declare("seconds", "resolution")
	expression, err := ParseExpression(source, declared)
	if err != nil {
		t.Fatal(err)
	}
	return RateSchedule{
		Currency: Currency{Code: "USD", Version: 1, FractionDigits: 6, RoundingMode: "half_even", MaxAmount: "1000"},
		Components: []RateComponent{{
			ID: 7, Code: "video_seconds", Unit: "second",
			Source: QuantityGeneratedSeconds, Event: ChargeSucceeded,
			UnitPrice: "0", PricingMode: PricingModeExpression, PricingExpr: source, Expr: expression,
			MaxPrice: maxPrice, QuantityStep: "0", MaxQuantity: "30",
		}},
	}, declared
}

func TestExpressionScheduleReservesProvenBound(t *testing.T) {
	schedule, _ := expressionSchedule(t, "seconds * 0.10", "3")
	charge, err := schedule.Reserve()
	if err != nil {
		t.Fatal(err)
	}
	// Reservation must not evaluate the expression: at reservation time the
	// quantity is unknown, so the proven bound is the pre-authorization amount.
	if charge.Amount.String() != "3" {
		t.Fatalf("reserved=%s, want 3", charge.Amount.String())
	}
}

func TestExpressionScheduleEvaluatesLiveCharge(t *testing.T) {
	schedule, declared := expressionSchedule(t, "seconds * 0.10", "3")
	schedule.Components[0].ExpressionDeclared = declared
	facts := Facts{
		Events:     map[ChargeEvent]bool{ChargeSucceeded: true},
		Quantities: map[QuantitySource]string{QuantityGeneratedSeconds: "12"},
		Expr: ExprEnv{Declared: declared, Vars: map[string]ExprValue{
			"seconds": ExprNumber(mustAmount(t, "12")),
		}},
	}
	charge, err := schedule.Evaluate(facts)
	if err != nil {
		t.Fatal(err)
	}
	if charge.Amount.String() != "1.2" {
		t.Fatalf("charged=%s, want 1.2", charge.Amount.String())
	}
	if len(charge.Lines) != 1 || charge.Lines[0].ComponentCode != "video_seconds" {
		t.Fatalf("lines=%+v", charge.Lines)
	}
}

func TestExpressionSnapshotAndAuditKeepTypedFacts(t *testing.T) {
	schedule, declared := expressionSchedule(t, "seconds * (resolution=='720p' ? tier('seconds',seconds) : 0.3)", "15")
	schedule.Components[0].ExpressionDeclared = declared
	schedule.Components[0].ExpressionDeclared["resolution"] = true
	schedule.Components[0].Expr, _ = ParseExpression(schedule.Components[0].PricingExpr, schedule.Components[0].ExpressionDeclared)
	schedule.Components[0].ExprTiers = map[string][]ExprTier{"seconds": {{UpTo: "10", Value: "0.5"}, {UpTo: "", Value: "0.4"}}}

	snapshot, err := json.Marshal(schedule)
	if err != nil {
		t.Fatal(err)
	}
	var restored RateSchedule
	if err := json.Unmarshal(snapshot, &restored); err != nil {
		t.Fatal(err)
	}
	component := &restored.Components[0]
	component.Expr, err = ParseExpression(component.PricingExpr, component.ExpressionDeclared)
	if err != nil {
		t.Fatal(err)
	}
	facts := Facts{
		Events:     map[ChargeEvent]bool{ChargeSucceeded: true},
		Quantities: map[QuantitySource]string{QuantityGeneratedSeconds: "10"},
		Expr: ExprEnv{Vars: map[string]ExprValue{
			"seconds":    ExprNumber(mustAmount(t, "10")),
			"resolution": ExprString("720p"),
		}},
	}
	charge, err := restored.Evaluate(facts)
	if err != nil || charge.Amount.String() != "5" {
		t.Fatalf("charge=%+v err=%v, want 5", charge, err)
	}
	audit, err := MarshalChargeAudit(restored, charge, facts)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"resolution":{"type":"string","value":"720p"}`, `"seconds":{"type":"number","value":"10"}`, `"amount":"5"`, `"pricing_expr":"seconds * (resolution=='720p' ? tier('seconds',seconds) : 0.3)"`, `"up_to":"10","value":"0.5"`} {
		if !strings.Contains(string(audit), expected) {
			t.Fatalf("audit JSON omitted %s: %s", expected, audit)
		}
	}
}

func TestExpressionScheduleUsesPublishedTierTable(t *testing.T) {
	expression, err := ParseExpression("seconds * tier('res', seconds)", map[string]bool{"seconds": true})
	if err != nil {
		t.Fatal(err)
	}
	schedule := RateSchedule{
		Currency: Currency{Code: "CNY", Version: 1, FractionDigits: 8, RoundingMode: "half_even", MaxAmount: "1000000"},
		Components: []RateComponent{{Code: "video", Unit: "second", Source: QuantityGeneratedSeconds,
			Event: ChargeSucceeded, UnitPrice: "0", PricingMode: PricingModeExpression, Expr: expression,
			ExprTiers: map[string][]ExprTier{"res": {{UpTo: "5", Value: "1"}, {UpTo: "", Value: "2"}}},
			MaxPrice:  "20", QuantityStep: "0", MaxQuantity: "10"}},
	}
	facts := Facts{Events: map[ChargeEvent]bool{ChargeSucceeded: true}, Quantities: map[QuantitySource]string{QuantityGeneratedSeconds: "6"}, Expr: ExprEnv{Declared: map[string]bool{"seconds": true}, Vars: map[string]ExprValue{"seconds": ExprNumber(mustAmount(t, "6"))}}}
	charge, err := schedule.Evaluate(facts)
	if err != nil || charge.Amount.String() != "12" {
		t.Fatalf("charge=%v err=%v", charge, err)
	}
}

func TestExpressionScheduleRefusesAmountAboveProvenBound(t *testing.T) {
	// The bound claims 1, but the expression yields 3 for this quantity. A
	// disagreement between proof and expression must not be charged.
	schedule, declared := expressionSchedule(t, "seconds * 0.10", "1")
	facts := Facts{
		Events:     map[ChargeEvent]bool{ChargeSucceeded: true},
		Quantities: map[QuantitySource]string{QuantityGeneratedSeconds: "30"},
		Expr: ExprEnv{Declared: declared, Vars: map[string]ExprValue{
			"seconds": ExprNumber(mustAmount(t, "30")),
		}},
	}
	if _, err := schedule.Evaluate(facts); !errors.Is(err, ErrAmountOverflow) {
		t.Fatalf("err=%v, want ErrAmountOverflow", err)
	}
}

func TestRateComponentPricingModePairing(t *testing.T) {
	base, _ := expressionSchedule(t, "seconds * 0.10", "3")
	component := base.Components[0]

	missingBound := component
	missingBound.MaxPrice = ""
	if err := missingBound.Validate(); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("expression without max_price err=%v, want ErrInvalidRate", err)
	}

	missingExpr := component
	missingExpr.Expr = nil
	if err := missingExpr.Validate(); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("expression without expr err=%v, want ErrInvalidRate", err)
	}

	flatWithExpr := component
	flatWithExpr.PricingMode = PricingModeFlat
	flatWithExpr.MaxPrice = ""
	if err := flatWithExpr.Validate(); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("flat carrying an expr err=%v, want ErrInvalidRate", err)
	}

	unknownMode := component
	unknownMode.PricingMode = "tiered"
	if err := unknownMode.Validate(); !errors.Is(err, ErrInvalidRate) {
		t.Fatalf("unknown mode err=%v, want ErrInvalidRate", err)
	}

	// An empty mode stays flat so existing rows keep validating unchanged.
	legacy := component
	legacy.PricingMode, legacy.Expr, legacy.MaxPrice = "", nil, ""
	legacy.UnitPrice = "0.10"
	if err := legacy.Validate(); err != nil {
		t.Fatalf("legacy flat component err=%v", err)
	}
}
