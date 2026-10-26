package billing

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func TestDisplayFormulaPreservesConditionsAndQuantities(t *testing.T) {
	formulas := []string{
		"seconds * (resolution == '1080p' ? 1.5 : resolution == '720p' ? 0.5 : 0.4)",
		"seconds * (resolution == '1080p' ? 0.35 : resolution == '720p' ? 0.30 : 0.25)",
		"resolution == '1080p' ? (seconds == 0 ? 6 : seconds) * 0.32 : resolution == '720p' ? (seconds == 0 ? 6 : seconds) * 0.168 : (seconds == 0 ? 6 : seconds) * 0.116",
		"(resolution == '720p') ? (seconds == 0 ? 6 : seconds) * 0.11 : (seconds == 0 ? 6 : seconds) * 0.08",
		"resolution == '1080p' || resolution == '4k' ? seconds * 15.96 : seconds * 7.11",
		"resolution == '4k' ? 29 : resolution == '1080p' ? 10.5 : resolution == '720p' ? 6.7 : 4.8",
		"resolution == '2k' ? 14 : 9.9",
		"seconds <= 10 ? seconds * 0.2 : seconds * 0.4 + 0.3",
		"(seconds * 0.4 + 0.2) / 1000",
		"0.4 * (seconds * 2)",
		"param('tokens') / 1000000 * 0.3 + seconds * 0.4",
	}
	declared := declare("seconds", "resolution", "tokens")
	factor := decimal.RequireFromString("1.8")
	for _, formula := range formulas {
		t.Run(formula, func(t *testing.T) {
			source, err := ParseExpression(formula, declared)
			if err != nil {
				t.Fatal(err)
			}
			shown, tables, err := source.DisplayFormula(factor, nil)
			if err != nil {
				t.Fatal(err)
			}
			projected, err := ParseExpression(shown, declared)
			if err != nil {
				t.Fatal(err)
			}
			for _, seconds := range []string{"0", "1", "6", "10", "11", "15", "30"} {
				for _, resolution := range []string{"480p", "720p", "1080p", "2k", "4k"} {
					env := ExprEnv{Declared: declared, Vars: numberVars(map[string]string{"seconds": seconds, "tokens": "2000000"}, t)}
					env.Vars["resolution"] = ExprString(resolution)
					base, err := source.Evaluate(env)
					if err != nil {
						t.Fatal(err)
					}
					env.Tiers = tables
					got, err := projected.Evaluate(env)
					if err != nil || !got.value.Equal(base.value.Mul(factor)) {
						t.Fatalf("seconds=%s resolution=%s formula=%s: got %v err=%v base=%v", seconds, resolution, shown, got, err, base)
					}
				}
			}
			if strings.Contains(shown, "* 1.8") || source.Source() != formula {
				t.Fatalf("unprojected/source formula: %s", shown)
			}
			if strings.Contains(formula, "seconds == 0 ? 6") && !strings.Contains(shown, "seconds == 0) ? 6") {
				t.Fatalf("default duration changed: %s", shown)
			}
			if formula == "0.4 * (seconds * 2)" && shown != "(0.72 * (seconds * 2))" {
				t.Fatalf("quantity coefficient changed: %s", shown)
			}
		})
	}
}

func TestDisplayFormulaCopiesTiersByRoleAndExcludesUnusedTables(t *testing.T) {
	declared := declare("seconds")
	source, err := ParseExpression("tier('rate',seconds) == 0.5 ? tier('rate',seconds) * 0.4 : seconds * tier('rate',seconds)", declared)
	if err != nil {
		t.Fatal(err)
	}
	tables := map[string][]ExprTier{
		"rate":   {{UpTo: "10", Value: "0.5"}, {Value: "0.4"}},
		"unused": {{Value: "999"}},
	}
	before, _ := json.Marshal(tables)
	shown, projectedTables, err := source.DisplayFormula(decimal.RequireFromString("1.8"), tables)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(tables)
	if string(before) != string(after) || len(projectedTables) != 3 {
		t.Fatalf("tiers changed/leaked: %v", projectedTables)
	}
	if projectedTables["rate_1"][0].Value != "0.5" || projectedTables["rate_2"][0].Value != "0.5" || projectedTables["rate_3"][0].Value != "0.9" {
		t.Fatalf("tier roles: %v", projectedTables)
	}
	for _, table := range projectedTables {
		if table[0].UpTo != "10" {
			t.Fatal("threshold changed")
		}
	}
	projected, err := ParseExpression(shown, declared)
	if err != nil {
		t.Fatal(err)
	}
	for seconds, want := range map[string]string{"6": "0.36", "15": "10.8"} {
		got, err := projected.Evaluate(ExprEnv{Declared: declared, Vars: numberVars(map[string]string{"seconds": seconds}, t), Tiers: projectedTables})
		if err != nil || got.String() != want {
			t.Fatalf("amount=%v err=%v want %s", got, err, want)
		}
	}
}

func TestDisplayFormulaDoesNotReturnUnprojectedPriceOnFailure(t *testing.T) {
	for _, formula := range []string{"seconds", "seconds + 0.4", "seconds * 0.000000000000000001"} {
		source, err := ParseExpression(formula, declare("seconds"))
		if err != nil {
			t.Fatal(err)
		}
		shown, tiers, err := source.DisplayFormula(decimal.RequireFromString("1.8"), nil)
		if err == nil || shown != "" || tiers != nil {
			t.Fatalf("invalid projection returned a price: %q %v %v", shown, tiers, err)
		}
	}
}
