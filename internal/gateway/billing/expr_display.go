package billing

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// DisplayFormula projects monetary results, preserving predicates, quantities
// and tier thresholds. It never changes the parsed billing expression or tiers.
func (e *Expression) DisplayFormula(multiplier decimal.Decimal, tiers map[string][]ExprTier) (string, map[string][]ExprTier, error) {
	if e == nil || !multiplier.IsPositive() {
		return "", nil, ErrInvalidExpression
	}
	p := expressionDisplay{tiers: tiers, shown: make(map[string][]ExprTier)}
	formula, err := p.scale(e.root, multiplier)
	if err != nil {
		return "", nil, err
	}
	declared := make(map[string]bool, len(e.idents))
	for _, name := range e.idents {
		declared[name] = true
	}
	if _, err := ParseExpression(formula, declared); err != nil {
		return "", nil, err
	}
	for _, table := range p.shown {
		if err := validateTiers(table); err != nil {
			return "", nil, err
		}
	}
	return formula, p.shown, nil
}

type expressionDisplay struct {
	tiers map[string][]ExprTier
	shown map[string][]ExprTier
}

// A price must contain a coefficient that can absorb the multiplier. A bare
// variable is not a declared monetary coefficient and cannot be exposed as one.
func scalablePrice(node *exprNode) bool {
	switch node.kind {
	case nodeNumber:
		return true
	case nodeTernary:
		return scalablePrice(node.children[1]) && scalablePrice(node.children[2])
	case nodeCall:
		return node.operator == "tier"
	case nodeBinary:
		switch node.operator {
		case "+", "-":
			return scalablePrice(node.children[0]) && scalablePrice(node.children[1])
		case "*":
			return scalablePrice(node.children[0]) || scalablePrice(node.children[1])
		case "/":
			return scalablePrice(node.children[0])
		}
	}
	return false
}

func (p *expressionDisplay) scale(node *exprNode, factor decimal.Decimal) (string, error) {
	if factor.Equal(decimal.NewFromInt(1)) {
		return p.render(node)
	}
	switch node.kind {
	case nodeNumber:
		return node.value.Mul(factor).String(), nil
	case nodeTernary:
		condition, err := p.render(node.children[0])
		if err != nil {
			return "", err
		}
		yes, err := p.scale(node.children[1], factor)
		if err != nil {
			return "", err
		}
		no, err := p.scale(node.children[2], factor)
		if err != nil {
			return "", err
		}
		return "(" + condition + " ? " + yes + " : " + no + ")", nil
	case nodeBinary:
		leftFactor, rightFactor := decimal.NewFromInt(1), decimal.NewFromInt(1)
		switch node.operator {
		case "+", "-":
			leftFactor, rightFactor = factor, factor
		case "*":
			// Prefer a direct coefficient to constants nested in a quantity.
			if node.children[0].kind == nodeNumber && node.children[1].kind != nodeNumber {
				leftFactor = factor
			} else if scalablePrice(node.children[1]) {
				rightFactor = factor
			} else {
				leftFactor = factor
			}
		case "/":
			leftFactor = factor
		default:
			return "", ErrInvalidExpression
		}
		left, err := p.scale(node.children[0], leftFactor)
		if err != nil {
			return "", err
		}
		right, err := p.scale(node.children[1], rightFactor)
		if err != nil {
			return "", err
		}
		return "(" + left + " " + node.operator + " " + right + ")", nil
	case nodeCall:
		if node.operator == "tier" {
			return p.tier(node, factor)
		}
	}
	return "", fmt.Errorf("%w: price has no monetary coefficient", ErrInvalidExpression)
}

func (p *expressionDisplay) tier(node *exprNode, factor decimal.Decimal) (string, error) {
	table, ok := p.tiers[node.children[0].text]
	if !ok {
		return "", ErrUndeclaredIdentifier
	}
	if err := validateTiers(table); err != nil {
		return "", err
	}
	argument, err := p.render(node.children[1])
	if err != nil {
		return "", err
	}
	// Each use gets a separate table: the same source table can appear in a
	// condition, a quantity expression, and a price branch with different roles.
	name := fmt.Sprintf("rate_%d", len(p.shown)+1)
	shown := append([]ExprTier(nil), table...)
	for i := range shown {
		value, err := decimal.NewFromString(shown[i].Value)
		if err != nil {
			return "", err
		}
		shown[i].Value = value.Mul(factor).String()
	}
	p.shown[name] = shown
	return "tier('" + name + "', " + argument + ")", nil
}

func (p *expressionDisplay) render(node *exprNode) (string, error) {
	switch node.kind {
	case nodeNumber:
		return node.value.String(), nil
	case nodeString:
		return "'" + node.text + "'", nil
	case nodeIdent:
		return node.text, nil
	case nodeCall:
		if node.operator == "tier" {
			return p.tier(node, decimal.NewFromInt(1))
		}
	}
	children := make([]string, len(node.children))
	for i, child := range node.children {
		value, err := p.render(child)
		if err != nil {
			return "", err
		}
		children[i] = value
	}
	switch node.kind {
	case nodeBinary:
		return "(" + children[0] + " " + node.operator + " " + children[1] + ")", nil
	case nodeTernary:
		return "(" + children[0] + " ? " + children[1] + " : " + children[2] + ")", nil
	case nodeCall:
		return node.operator + "(" + strings.Join(children, ", ") + ")", nil
	}
	return "", ErrInvalidExpression
}
