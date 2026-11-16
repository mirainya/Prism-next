package engine

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/mirainya/Prism/internal/gateway/billing"
	"github.com/mirainya/Prism/internal/gateway/canonical"
	"github.com/mirainya/Prism/internal/gateway/routing"
)

func canonicalPriceFixture() *routing.RouteResult {
	return &routing.RouteResult{Currency: "CNY", CurrencyVersion: 2, SellSchedule: &billing.RateSchedule{
		Currency: billing.Currency{Code: "CNY", Version: 2, FractionDigits: 8, RoundingMode: "half_even", MaxAmount: "1000000"},
		Components: []billing.RateComponent{{Code: "request", Unit: "request", Source: billing.QuantityOne,
			Event: billing.ChargeSucceeded, UnitPrice: "0.3", QuantityStep: "0", MaxQuantity: "1"}},
	}}
}

func TestCanonicalPricingRequiresExplicitSupportedSchedule(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*routing.RouteResult)
	}{
		{"no_price", func(r *routing.RouteResult) { r.SellSchedule = nil }},
		{"no_currency", func(r *routing.RouteResult) { r.Currency = "" }},
		{"wrong_version", func(r *routing.RouteResult) { r.CurrencyVersion = 1 }},
		{"seconds", func(r *routing.RouteResult) {
			r.SellSchedule.Components[0].Source = billing.QuantityGeneratedSeconds
			r.SellSchedule.Components[0].Unit = "second"
		}},
		{"delivery", func(r *routing.RouteResult) { r.SellSchedule.Components[0].Event = billing.ChargeDelivered }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := canonicalPriceFixture()
			tc.change(r)
			if _, err := canonicalSellSchedule(r); !errors.Is(err, billing.ErrInvalidRate) {
				t.Fatalf("expected explicit pricing rejection, got %v", err)
			}
		})
	}
}

func TestCanonicalPricingSnapshotAndMissingUsage(t *testing.T) {
	r := canonicalPriceFixture()
	schedule, err := canonicalSellSchedule(r)
	if err != nil {
		t.Fatal(err)
	}
	r.SellSchedule.Components[0].UnitPrice = "9"
	facts, err := canonicalBillingFacts(nil)
	if err != nil {
		t.Fatal(err)
	}
	charge, err := schedule.Evaluate(facts)
	if err != nil || charge.Amount.String() != "0.3" {
		t.Fatalf("charge=%v err=%v", charge, err)
	}
	schedule.Components[0].Unit = "token"
	schedule.Components[0].Source = billing.QuantityInputTokens
	if _, err := schedule.Evaluate(facts); !errors.Is(err, billing.ErrMissingFact) {
		t.Fatalf("missing tokens are not zero: %v", err)
	}
}

func TestCanonicalBillingValidatesCachedTokenSubset(t *testing.T) {
	facts, err := canonicalBillingFacts(&canonical.Usage{InputTokens: 20, CachedInputTokens: 7, OutputTokens: 4})
	if err != nil || facts.Quantities[billing.QuantityUncachedTokens] != "13" || facts.Quantities[billing.QuantityCachedTokens] != "7" {
		t.Fatalf("facts=%v err=%v", facts, err)
	}
	for _, usage := range []*canonical.Usage{{InputTokens: -1}, {OutputTokens: -1}, {InputTokens: 2, CachedInputTokens: 3}, {CachedInputTokens: -1}} {
		if _, err := canonicalBillingFacts(usage); !errors.Is(err, billing.ErrInvalidAmount) {
			t.Fatalf("invalid usage accepted: %v", usage)
		}
	}
}

func TestCanonicalBillingFactsPopulateExpressionVariables(t *testing.T) {
	facts, err := canonicalBillingFacts(&canonical.Usage{
		InputTokens: 20, CachedInputTokens: 7, OutputTokens: 4, ReasoningOutputTokens: 3,
		Extra: map[string]json.RawMessage{"cache_creation_input_tokens": json.RawMessage("2")},
	})
	if err != nil {
		t.Fatal(err)
	}
	expression, err := billing.ParseExpression("input_tokens + output_tokens + cache_read_tokens + cache_write_tokens + reasoning_tokens + success", facts.Expr.Declared)
	if err != nil {
		t.Fatal(err)
	}
	amount, err := expression.Evaluate(facts.Expr)
	if err != nil || amount.String() != "37" {
		t.Fatalf("expression amount=%s err=%v facts=%+v", amount.String(), err, facts)
	}
}

func TestCanonicalBillingFactsLeaveUnknownExpressionVariableMissing(t *testing.T) {
	facts, err := canonicalBillingFacts(&canonical.Usage{InputTokens: 1})
	if err != nil {
		t.Fatal(err)
	}
	expression, err := billing.ParseExpression("cache_write_tokens + 1", facts.Expr.Declared)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := expression.Evaluate(facts.Expr); !errors.Is(err, billing.ErrMissingFact) {
		t.Fatalf("err=%v, want missing cache write fact", err)
	}
}

func TestCacheTokenPricesChargeDisjointUsage(t *testing.T) {
	usage := &canonical.Usage{InputTokens: 1000, CachedInputTokens: 200, OutputTokens: 100,
		Extra: map[string]json.RawMessage{"cache_creation_input_tokens": json.RawMessage("300")}}
	facts, err := CanonicalBillingFacts(usage)
	if err != nil {
		t.Fatal(err)
	}
	schedule := canonicalPriceFixture().SellSchedule
	schedule.Components = nil
	for _, rate := range []struct {
		code   string
		source billing.QuantitySource
		price  string
	}{
		{"input", billing.QuantityInputWithoutCacheTokens, "2"},
		{"output", billing.QuantityOutputTokens, "8"},
		{"cache_read", billing.QuantityCachedTokens, "0.2"},
		{"cache_write", billing.QuantityCacheWriteTokens, "3"},
	} {
		schedule.Components = append(schedule.Components, billing.RateComponent{Code: rate.code, Unit: "token", Source: rate.source,
			Event: billing.ChargeSucceeded, UnitPrice: rate.price, UnitScale: 6, QuantityStep: "0", MaxQuantity: "1000"})
	}
	charge, err := schedule.Evaluate(facts)
	// 500 ordinary input, 200 cache reads, 300 cache writes, 100 output.
	if err != nil || charge.Amount.String() != "0.00274" {
		t.Fatalf("charge=%+v error=%v", charge, err)
	}
	delete(usage.Extra, "cache_creation_input_tokens")
	facts, err = CanonicalBillingFacts(usage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := schedule.Evaluate(facts); !errors.Is(err, billing.ErrMissingFact) {
		t.Fatalf("missing cache usage was priced as zero: %v", err)
	}
	for _, value := range []string{"-1", "0.5", "801", `"NaN"`} {
		usage.Extra["cache_creation_input_tokens"] = json.RawMessage(value)
		if _, err := CanonicalBillingFacts(usage); !errors.Is(err, billing.ErrInvalidAmount) {
			t.Fatalf("invalid cache write usage %s accepted: %v", value, err)
		}
	}
}

func TestCachePricesCanIndependentlyInheritInputPrice(t *testing.T) {
	facts, err := CanonicalBillingFacts(&canonical.Usage{
		InputTokens: 1000, CachedInputTokens: 200, OutputTokens: 100,
		Extra: map[string]json.RawMessage{"cache_write_tokens": json.RawMessage("300")},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		input  billing.QuantitySource
		read   string
		write  string
		amount string
	}{
		{"both_inherit", billing.QuantityInputTokens, "", "", "0.0028"},
		{"write_inherits", billing.QuantityUncachedTokens, "0.2", "", "0.00244"},
		{"read_inherits", billing.QuantityInputWithoutCacheWriteTokens, "", "3", "0.0031"},
		{"read_is_free", billing.QuantityInputWithoutCacheTokens, "0", "3", "0.0027"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			route := canonicalPriceFixture()
			route.SellSchedule.Components = nil
			for _, rate := range []struct {
				code   string
				source billing.QuantitySource
				price  string
			}{
				{"input", tc.input, "2"}, {"output", billing.QuantityOutputTokens, "8"},
				{"cache_read", billing.QuantityCachedTokens, tc.read}, {"cache_write", billing.QuantityCacheWriteTokens, tc.write},
			} {
				if rate.price == "" {
					continue
				}
				route.SellSchedule.Components = append(route.SellSchedule.Components, billing.RateComponent{
					Code: rate.code, Source: rate.source, Unit: "token", Event: billing.ChargeSucceeded,
					UnitPrice: rate.price, UnitScale: 6, QuantityStep: "0", MaxQuantity: "1000",
				})
			}
			schedule, err := canonicalSellSchedule(route)
			if err != nil {
				t.Fatal(err)
			}
			charge, err := schedule.Evaluate(facts)
			if err != nil || charge.Amount.String() != tc.amount {
				t.Fatalf("charge=%+v error=%v, want %s", charge, err, tc.amount)
			}
		})
	}
}
