package runtime

import (
	"testing"

	"github.com/mirainya/Prism/internal/gateway/billing"
)

func testVideoExpressionSchedule(t *testing.T, source string, maxPrice string) billing.RateSchedule {
	t.Helper()
	declared := map[string]bool{"seconds": true, "resolution": true, "duration_ms": true, "priority": true}
	expression, err := billing.ParseExpression(source, declared)
	if err != nil {
		t.Fatal(err)
	}
	return billing.RateSchedule{
		Currency: billing.Currency{Code: "CNY", Version: 1, FractionDigits: 2, RoundingMode: "half_even", MaxAmount: "1000"},
		Components: []billing.RateComponent{{
			Code: "video_generation", Unit: "second", Source: billing.QuantityGeneratedSeconds,
			Event: billing.ChargeSucceeded, UnitPrice: "0", PricingMode: billing.PricingModeExpression,
			PricingExpr: source, ExpressionDeclared: declared, Expr: expression, MaxPrice: maxPrice,
			UnitScale: 0, QuantityStep: "0", MaxQuantity: "30",
		}},
	}
}

func TestEstimateVideoChargeUsesRequestedResolutionAndDuration(t *testing.T) {
	schedule := testVideoExpressionSchedule(t, "seconds * (resolution=='720p' ? 0.5 : 0.3)", "15")
	charge, upperBound, err := EstimateVideoCharge(schedule, []byte(`{"duration":10,"resolution":"720p"}`))
	if err != nil || upperBound || charge.Amount.String() != "5" {
		t.Fatalf("charge=%s upper_bound=%t err=%v, want request estimate 5", charge.Amount.String(), upperBound, err)
	}
}

func TestEstimateVideoChargeUsesEffectiveTierParameter(t *testing.T) {
	schedule := testVideoExpressionSchedule(t, "priority * (resolution=='720p' ? 1 : 0.5)", "30")
	charge, upperBound, err := EstimateVideoCharge(schedule, []byte(`{"duration":10,"resolution":"720p","params":{"priority":4}}`))
	if err != nil || upperBound || charge.Amount.String() != "4" {
		t.Fatalf("charge=%s upper_bound=%t err=%v, want 4", charge.Amount.String(), upperBound, err)
	}
}

func TestEstimateVideoChargeFallsBackToReservationForUnknownFacts(t *testing.T) {
	schedule := testVideoExpressionSchedule(t, "seconds * (duration_ms > 0 ? 0.5 : 0.3)", "15")
	charge, upperBound, err := EstimateVideoCharge(schedule, []byte(`{"duration":10,"resolution":"720p"}`))
	if err != nil || !upperBound || charge.Amount.String() != "15" {
		t.Fatalf("charge=%s upper_bound=%t err=%v, want maximum reservation 15", charge.Amount.String(), upperBound, err)
	}
}

func TestEstimateVideoChargeFallsBackToReservationWhenDurationIsOmitted(t *testing.T) {
	schedule := testVideoExpressionSchedule(t, "seconds * (resolution=='720p' ? 0.5 : 0.3)", "15")
	charge, upperBound, err := EstimateVideoCharge(schedule, []byte(`{"resolution":"720p"}`))
	if err != nil || !upperBound || charge.Amount.String() != "15" {
		t.Fatalf("charge=%s upper_bound=%t err=%v, want maximum reservation 15", charge.Amount.String(), upperBound, err)
	}
}

func TestVideoPricingRequestFactsKeepsUnavailableFactsUnknown(t *testing.T) {
	facts, err := videoPricingRequestFacts([]byte(`{"duration":10}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := facts.Expr.Vars["resolution"]; exists {
		t.Fatal("missing resolution was converted into a billing fact")
	}
}
