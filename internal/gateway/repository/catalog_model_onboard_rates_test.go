package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/billing"
)

func tokenOnboardRate(code, price, max string) CatalogModelOnboardRate {
	source := billing.QuantityInputTokens
	if code == "output" {
		source = billing.QuantityOutputTokens
	}
	return CatalogModelOnboardRate{UnitCode: "token", UnitPrice: price, ComponentCode: code,
		QuantitySource: source, ChargeEvent: billing.ChargeSucceeded, UnitScale: 6,
		QuantityStep: "0", MaxQuantity: max, PricingMode: "flat", ReasonCode: "model_onboard"}
}

func tokenModelOnboardInput() CatalogModelOnboardInput {
	in := validCatalogModelOnboardInput()
	in.SellRate = tokenOnboardRate("input", "2", "1000000")
	cost := tokenOnboardRate("input", "1", "1000000")
	in.CostRate = &cost
	in.AdditionalSellRates = []CatalogModelOnboardRate{tokenOnboardRate("output", "8", "100000")}
	in.AdditionalCostRates = []CatalogModelOnboardRate{tokenOnboardRate("output", "4", "100000")}
	return in
}

func TestTokenModelOnboardRatesValidateAndUseExistingBilling(t *testing.T) {
	in := tokenModelOnboardInput()
	if err := in.Normalize(); err != nil {
		t.Fatal(err)
	}
	if err := in.Validate(); err != nil {
		t.Fatal(err)
	}
	schedule := billing.RateSchedule{Currency: billing.Currency{Code: "CNY", Version: 1, FractionDigits: 6, RoundingMode: "half_up", MaxAmount: "1000000"},
		Components: []billing.RateComponent{in.SellRate.meter(), in.AdditionalSellRates[0].meter()}}
	charge, err := schedule.Evaluate(billing.Facts{Events: map[billing.ChargeEvent]bool{billing.ChargeSucceeded: true},
		Quantities: map[billing.QuantitySource]string{billing.QuantityInputTokens: "12345", billing.QuantityOutputTokens: "1234"}})
	if err != nil || charge.Amount.String() != "0.034562" || len(charge.Lines) != 2 {
		t.Fatalf("charge=%+v err=%v", charge, err)
	}
	reserve, err := schedule.Reserve()
	if err != nil || reserve.Amount.String() != "2.8" {
		t.Fatalf("reserve=%+v err=%v", reserve, err)
	}
	if _, err := schedule.Evaluate(billing.Facts{Events: map[billing.ChargeEvent]bool{billing.ChargeSucceeded: true}}); !errors.Is(err, billing.ErrMissingFact) {
		t.Fatalf("missing usage err=%v", err)
	}
}

func TestModelOnboardAdditionalRatesRejectAmbiguousAndIncompleteSchedules(t *testing.T) {
	for name, mutate := range map[string]func(*CatalogModelOnboardInput){
		"duplicate code": func(in *CatalogModelOnboardInput) { in.AdditionalSellRates[0].ComponentCode = "input" },
		"duplicate meter": func(in *CatalogModelOnboardInput) {
			in.AdditionalSellRates[0].QuantitySource = billing.QuantityInputTokens
		},
		"cache double counting": func(in *CatalogModelOnboardInput) {
			in.AdditionalSellRates[0].QuantitySource = billing.QuantityCachedTokens
		},
		"missing output bound":  func(in *CatalogModelOnboardInput) { in.AdditionalSellRates[0].MaxQuantity = "" },
		"fractional bound":      func(in *CatalogModelOnboardInput) { in.AdditionalCostRates[0].MaxQuantity = "1.5" },
		"missing output cost":   func(in *CatalogModelOnboardInput) { in.AdditionalCostRates[0].UnitPrice = "" },
		"negative output cost":  func(in *CatalogModelOnboardInput) { in.AdditionalCostRates[0].UnitPrice = "-1" },
		"missing primary price": func(in *CatalogModelOnboardInput) { in.SellRate = CatalogModelOnboardRate{} },
		"wrong output unit":     func(in *CatalogModelOnboardInput) { in.AdditionalSellRates[0].UnitCode = "request" },
		"too many components":   func(in *CatalogModelOnboardInput) { in.AdditionalCostRates = make([]CatalogModelOnboardRate, 32) },
	} {
		t.Run(name, func(t *testing.T) {
			in := tokenModelOnboardInput()
			mutate(&in)
			if err := in.Normalize(); err != nil {
				t.Fatal(err)
			}
			if err := in.Validate(); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("err=%v", err)
			}
		})
	}
	in := tokenModelOnboardInput()
	in.AdditionalCostRates[0].UnitPrice = "0"
	in.AdditionalSellRates[0].UnitPrice = "0.000000000000000001"
	if err := in.Normalize(); err != nil {
		t.Fatal(err)
	}
	if err := in.Validate(); err != nil {
		t.Fatalf("explicit zero and precise prices: %v", err)
	}
}

func expectOnboardRateComponent(mock sqlmock.Sqlmock, kind string, rate CatalogModelOnboardRate, evidenceID uint64) {
	mock.ExpectQuery("SELECT status FROM billing_currency_definitions").WithArgs("CNY", uint32(1)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("active"))
	reference := fmt.Sprintf("prism-console:model-onboard:%s:public-model:stamp:%s", kind, rate.ComponentCode)
	mock.ExpectExec("INSERT INTO gw_rate_evidence\\(").WithArgs("operator_manual", "operator_declared", reference, sqlmock.AnyArg(), strings.Repeat("d", 64), rate.UnitCode, rate.UnitPrice, "CNY", uint32(1), sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(int64(evidenceID), 1))
	mock.ExpectExec("INSERT INTO gw_rate_evidence_review_events").WillReturnResult(sqlmock.NewResult(201, 1))
	mock.ExpectExec("INSERT INTO gw_rate_evidence_review_state").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT state,state_version FROM gw_rate_evidence_review_state").WithArgs(evidenceID).WillReturnRows(sqlmock.NewRows([]string{"state", "state_version"}).AddRow("submitted", 1))
	mock.ExpectExec("INSERT INTO gw_rate_evidence_review_events").WillReturnResult(sqlmock.NewResult(202, 1))
	mock.ExpectExec("UPDATE gw_rate_evidence_review_state").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO audit_events").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT e.unit_code,e.unit_price.*FROM gw_rate_evidence e").WithArgs(evidenceID).WillReturnRows(sqlmock.NewRows([]string{"unit_code", "unit_price", "currency_code", "currency_version", "state", "latest_review_event_id", "decision"}).AddRow(rate.UnitCode, rate.UnitPrice, "CNY", 1, "accepted", 202, "accepted"))
	parentTable, rateTable := "gw_skus", "gw_sell_rates"
	if kind == "cost" {
		parentTable, rateTable = "gw_cost_plans", "gw_cost_rates"
	}
	mock.ExpectQuery("SELECT COUNT.*FROM "+parentTable).WithArgs(uint64(41), uint64(7)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("INSERT INTO "+rateTable).WithArgs(uint64(7), uint64(41), uint64(202), rate.UnitCode, rate.UnitPrice, "CNY", uint32(1), rate.ComponentCode, rate.QuantitySource, rate.ChargeEvent, int32(6), "0", rate.MaxQuantity, "flat", nil, nil, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(int64(evidenceID+1000), 1))
}

func TestModelOnboardRateComponentsSaveTogetherAndRollbackOnFailure(t *testing.T) {
	for _, kind := range []string{"sell", "cost"} {
		for _, failSecond := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/failure=%t", kind, failSecond), func(t *testing.T) {
				store, mock := newCatalogModelOnboardTestStore(t)
				input, output := tokenOnboardRate("input", "2", "1000000"), tokenOnboardRate("output", "2", "100000")
				mock.ExpectBegin()
				expectOnboardRateComponent(mock, kind, input, 100)
				if failSecond {
					mock.ExpectRollback()
				} else {
					expectOnboardRateComponent(mock, kind, output, 101)
					mock.ExpectCommit()
				}
				var references []string
				forced := errors.New("second component signer failed")
				err := store.WithTx(context.Background(), func(tx *sql.Tx) error {
					ids, err := store.createModelOnboardRates(context.Background(), tx, kind, 7, 41, "public-model", "stamp", "CNY", 1, input, []CatalogModelOnboardRate{output}, func(e RateEvidenceInput) (string, error) {
						references = append(references, e.SourceReference)
						if failSecond && len(references) == 2 {
							return "", forced
						}
						return strings.Repeat("d", 64), nil
					}, 1)
					if !failSecond && (len(ids) != 2 || ids[0] != 1100 || ids[1] != 1101) {
						t.Fatalf("ids=%v", ids)
					}
					return err
				})
				if failSecond && !errors.Is(err, forced) || !failSecond && err != nil {
					t.Fatal(err)
				}
				if len(references) != 2 || references[0] == references[1] {
					t.Fatalf("references=%v", references)
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
