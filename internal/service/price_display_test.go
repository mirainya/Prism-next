package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mirainya/Prism/internal/gateway/billing"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/shopspring/decimal"
)

func TestPriceDisplayUsesExactDecimalAmounts(t *testing.T) {
	display := PriceDisplay{multiplier: decimal.RequireFromString("1.8")}
	for input, want := range map[string]string{
		"": "", "0": "0", "0.20": "0.36", "0.32": "0.576",
		"0.000003": "0.0000054", "0.000000000000000001": "0.0000000000000000018",
	} {
		if got := display.Amount(input); got != want {
			t.Errorf("Amount(%q)=%q, want %q", input, got, want)
		}
	}
	identity := PriceDisplay{multiplier: decimal.NewFromInt(1)}
	if identity.Amount("0.2000") != "0.2000" {
		t.Fatal("multiplier 1 changed the price")
	}
}

func TestPriceDisplayOverridesAgreeAcrossPublicAPIsAndVideoEstimate(t *testing.T) {
	db := setupPublicCatalogQueryDB(t)
	if _, err := db.Exec(`UPDATE gw_price_display_settings SET default_multiplier='1.8',model_multipliers='{"gpt_4_1":"2"}',sku_multipliers='{"responses-standard":"3"}'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE gw_catalog_models SET description='4-30秒，0.2元/秒。' WHERE id=10`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	query := newQueryServiceWithDB(db)
	detail, err := query.GetModelDetail(ctx, "gpt-4.1")
	if err != nil {
		t.Fatal(err)
	}
	prices, err := (&PricingService{query: query}).GetPricing(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Description != "4-30秒，价格见规格表/秒。" || prices[0].Description != detail.Description {
		t.Fatalf("ambiguous description: %s / %s", detail.Description, prices[0].Description)
	}
	for _, variant := range detail.Pricing.Variants {
		want := "0.000006"
		if variant.Operation == "responses.create" {
			want = "0.06"
		}
		if variant.Components[0].UnitPrice != want {
			t.Fatalf("detail operation=%s price=%s want=%s", variant.Operation, variant.Components[0].UnitPrice, want)
		}
	}
	for _, sku := range prices[0].SKUs {
		display, err := PriceDisplayForSKU(ctx, db, sku.ID)
		if err != nil {
			t.Fatal(err)
		}
		base, want := "0.000003", "0.000006"
		if sku.ID == 31 {
			base, want = "0.02", "0.06"
		}
		if sku.Components[0].UnitPrice != want || display.Amount(base) != want {
			t.Fatalf("SKU %s price=%s estimate=%s want=%s", sku.Code, sku.Components[0].UnitPrice, display.Amount(base), want)
		}
	}
	settings := repository.PriceDisplaySettings{DefaultMultiplier: "1.8", ModelMultipliers: map[string]string{"gpt_4_1": "2"}}
	preview, err := PreviewPriceDisplay(ctx, db, settings, "gpt-4.1")
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range preview.Pricing.Variants {
		if variant.Operation == "responses.create" && variant.Components[0].UnitPrice != "0.04" {
			t.Fatal("preview used saved override")
		}
	}
	var saved string
	if err := db.QueryRow(`SELECT sku_multipliers FROM gw_price_display_settings WHERE id=1`).Scan(&saved); err != nil || saved != `{"responses-standard":"3"}` {
		t.Fatalf("preview saved changes: %q %v", saved, err)
	}
}

func TestPriceDisplayPriorityAndDescriptionInheritance(t *testing.T) {
	displays, err := PriceDisplaysFromSettings(repository.PriceDisplaySettings{DefaultMultiplier: "1.8", ModelMultipliers: map[string]string{"model": "2"}, SKUMultipliers: map[string]string{"sku": "3"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ model, sku, want string }{{"other", "other", "0.72"}, {"model", "other", "0.8"}, {"model", "sku", "1.2"}, {"other", "sku", "1.2"}} {
		if got := displays.For(item.model, item.sku).Amount("0.4"); got != item.want {
			t.Fatalf("%v got=%s", item, got)
		}
	}
	if got := displays.ModelDescription("model", []string{"sku"}, "0.4元，15秒"); got != "1.2元，15秒" {
		t.Fatal(got)
	}
}

func TestPublicPriceJSONExcludesInternalFormulaInputs(t *testing.T) {
	display := PriceDisplay{multiplier: decimal.RequireFromString("1.8")}
	rates, err := display.PublicRates([]billing.RateComponent{{PricingMode: billing.PricingModeExpression, UnitPrice: "0", PricingExpr: "seconds * (resolution == '1080p' ? 1.5 : 0.4)", ExpressionDeclared: map[string]bool{"seconds": true, "resolution": true}, MaxPrice: "30"}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(rates)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"expression_declared", "base_cost", "markup_ratio", "multiplier", "unit_cost", "unit_price", "1.8"} {
		if strings.Contains(string(encoded), field) {
			t.Fatalf("private field or multiplier exposed: %s", encoded)
		}
	}
	if !strings.Contains(string(encoded), "2.7") || !strings.Contains(string(encoded), "0.72") || rates[0].MaxPrice != "54" {
		t.Fatalf("unprojected formula: %s", encoded)
	}
}

func TestPriceDisplayTruncatesOnlyTotalAmounts(t *testing.T) {
	display := PriceDisplay{multiplier: decimal.RequireFromString("1.8")}
	for input, want := range map[string]string{
		"": "", "invalid": "invalid", "0": "0.00", "0.32": "0.57",
		"1.92": "3.45", "5": "9.00", "-1.92": "-3.45", "0.005": "0.00",
	} {
		if got := display.TotalAmount(input); got != want {
			t.Errorf("TotalAmount(%q)=%q, want %q", input, got, want)
		}
	}
	if display.Amount("0.000003") != "0.0000054" {
		t.Fatal("total formatting reduced unit-price precision")
	}
	identity := PriceDisplay{multiplier: decimal.NewFromInt(1)}
	if got := identity.TotalAmount("1.999"); got != "1.99" {
		t.Fatalf("identity total=%q, want 1.99", got)
	}
}

func TestPriceDisplayChangesOnlyPricesInDescriptions(t *testing.T) {
	display := PriceDisplay{multiplier: decimal.RequireFromString("1.8")}
	for input, want := range map[string]string{
		"720p，4-30秒，10图5视频5音频；0.20元/秒。":         "720p，4-30秒，10图5视频5音频；0.36元/秒。",
		"一次1.8元支持9图；1.8 元/次。":                   "一次3.24元支持9图；3.24 元/次。",
		"¥2.9/次；￥0.2/秒；$0.5；0.3 CREDIT":         "¥5.22/次；￥0.36/秒；$0.9；0.54 CREDIT",
		"0.20 credits/second; 0.3 USD; 0.4 CNY": "0.36 credits/second; 0.54 USD; 0.72 CNY",
		"seedance2.5-900；720p；16:9；4-30秒":       "seedance2.5-900；720p；16:9；4-30秒",
	} {
		if got := display.Description(input); got != want {
			t.Errorf("Description(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestPriceDisplayScalesWholeExpressionWithoutChangingSchedule(t *testing.T) {
	display := PriceDisplay{multiplier: decimal.RequireFromString("1.8")}
	declared := map[string]bool{"seconds": true, "resolution": true}
	formula := "seconds * (resolution=='720p' ? tier('seconds',seconds) : 0.3)"
	expression, err := billing.ParseExpression(formula, declared)
	if err != nil {
		t.Fatal(err)
	}
	source := []billing.RateComponent{{
		UnitPrice: "0", MaxPrice: "15", MaxQuantity: "30", UnitScale: 0, QuantityStep: "0",
		PricingMode: billing.PricingModeExpression, PricingExpr: formula, Expr: expression,
		ExpressionDeclared: declared, ExprTiers: map[string][]billing.ExprTier{"seconds": {{UpTo: "10", Value: "0.5"}, {Value: "0.4"}}},
	}}
	shown, err := display.Rates(source)
	if err != nil {
		t.Fatal(err)
	}
	if shown[0].MaxPrice != "27" || shown[0].MaxQuantity != "30" || shown[0].ExprTiers["rate_1"][0].UpTo != "10" || shown[0].ExprTiers["rate_1"][0].Value != "0.9" {
		t.Fatalf("expression price or non-price facts changed: %#v", shown[0])
	}
	if source[0].PricingExpr != formula || source[0].MaxPrice != "15" || source[0].Expr != expression {
		t.Fatal("display projection changed the source schedule")
	}
	shownExpression, err := billing.ParseExpression(shown[0].PricingExpr, declared)
	if err != nil {
		t.Fatal(err)
	}
	seconds, _ := billing.ParseAmount("6", 18, true)
	for resolution, want := range map[string]string{"720p": "5.4", "480p": "3.24"} {
		env := billing.ExprEnv{Declared: declared, Tiers: shown[0].ExprTiers, Vars: map[string]billing.ExprValue{
			"seconds": billing.ExprNumber(seconds), "resolution": billing.ExprString(resolution),
		}}
		amount, err := shownExpression.Evaluate(env)
		if err != nil || amount.String() != want {
			t.Fatalf("resolution=%s amount=%s err=%v, want %s", resolution, amount.String(), err, want)
		}
	}
}

func TestPriceDisplayDoesNotAccumulateAcrossQueriesOrUpdateStoredRates(t *testing.T) {
	db := setupPublicCatalogQueryDB(t)
	for _, statement := range []string{
		`UPDATE gw_catalog_models SET description='720p；4-30秒；10图；0.2元/秒。' WHERE id=10`,
		`UPDATE gw_operation_routes SET route_template='/v1/videos/generations' WHERE operation_contract_id=100`,
		`UPDATE gw_sku_downstream_paths SET path='/v1/videos/generations' WHERE sku_id=30`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	query := newQueryServiceWithDB(db)
	for attempt := 0; attempt < 2; attempt++ {
		models, err := query.ListAvailableCapabilities(context.Background(), "", "video")
		if err != nil || len(models) != 1 || models[0].Pricing.Variants[0].Components[0].UnitPrice != "0.0000054" || models[0].Description != "720p；4-30秒；10图；0.36元/秒。" {
			t.Fatalf("model projection=%#v err=%v", models, err)
		}
		prices, err := (&PricingService{query: query}).GetPricing(context.Background())
		if err != nil || len(prices) != 1 || prices[0].SKUs[0].Components[0].UnitPrice != "0.0000054" || prices[0].Description != models[0].Description {
			t.Fatalf("price projection=%#v err=%v", prices, err)
		}
	}
	schedules, err := repository.LoadSellSchedules(context.Background(), db, 1)
	if err != nil || schedules[30].Components[0].UnitPrice != "0.000003" {
		t.Fatalf("stored schedule changed: %#v err=%v", schedules, err)
	}
}
