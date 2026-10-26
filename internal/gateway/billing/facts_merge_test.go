package billing

import "testing"

func TestMergeExpressionFactsKeepsRequestAndObservedValues(t *testing.T) {
	one, _ := ParseAmount("1", 18, true)
	five, _ := ParseAmount("5", 18, true)
	six, _ := ParseAmount("6", 18, true)
	request := Facts{
		Events:     map[ChargeEvent]bool{ChargeAccepted: true},
		Quantities: map[QuantitySource]string{QuantityRequestedSeconds: "5"},
		Expr: ExprEnv{Declared: map[string]bool{"seconds": true}, Vars: map[string]ExprValue{
			"resolution": ExprString("720p"), "seconds": ExprNumber(five),
		}},
	}
	observed := Facts{
		Events:     map[ChargeEvent]bool{ChargeSucceeded: true},
		Quantities: map[QuantitySource]string{QuantityGeneratedSeconds: "6"},
		Expr: ExprEnv{Declared: map[string]bool{"resolution": true, "seconds": true, "success": true}, Vars: map[string]ExprValue{
			"seconds": ExprNumber(six), "success": ExprNumber(one),
		}},
	}

	merged := MergeExpressionFacts(request, observed)
	expression, err := ParseExpression("resolution=='720p' ? seconds + success : 0", map[string]bool{
		"resolution": true, "seconds": true, "success": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	amount, err := expression.Evaluate(merged.Expr)
	if err != nil || amount.String() != "7" {
		t.Fatalf("merged expression amount=%s err=%v, want 7", amount.String(), err)
	}
	if got := merged.Quantities[QuantityRequestedSeconds]; got != "5" {
		t.Fatalf("requested seconds=%q, want 5", got)
	}
	if !merged.Events[ChargeAccepted] || !merged.Events[ChargeSucceeded] {
		t.Fatalf("merged events=%v", merged.Events)
	}
	requestAmount, err := mustTestExpression(t, "seconds", map[string]bool{"seconds": true}).Evaluate(request.Expr)
	if err != nil || requestAmount.String() != "5" {
		t.Fatalf("request facts changed: amount=%s err=%v", requestAmount.String(), err)
	}
	observedAmount, err := mustTestExpression(t, "seconds", map[string]bool{"seconds": true}).Evaluate(observed.Expr)
	if err != nil || observedAmount.String() != "6" {
		t.Fatalf("observed facts changed: amount=%s err=%v", observedAmount.String(), err)
	}
}

func mustTestExpression(t *testing.T, source string, declared map[string]bool) *Expression {
	t.Helper()
	expression, err := ParseExpression(source, declared)
	if err != nil {
		t.Fatal(err)
	}
	return expression
}
