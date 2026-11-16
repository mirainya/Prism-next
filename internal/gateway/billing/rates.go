package billing

import (
	"errors"
	"fmt"
	"regexp"
)

var (
	ErrInvalidRate = errors.New("billing: invalid rate schedule")
	ErrMissingFact = errors.New("billing: required billing fact is unknown")
)

type QuantitySource string
type ChargeEvent string

const (
	QuantityOne              QuantitySource = "one"
	QuantityInputTokens      QuantitySource = "usage.input_tokens"
	QuantityOutputTokens     QuantitySource = "usage.output_tokens"
	QuantityUncachedTokens   QuantitySource = "usage.uncached_input_tokens"
	QuantityCachedTokens     QuantitySource = "usage.cached_input_tokens"
	QuantityCacheWriteTokens QuantitySource = "usage.cache_write_tokens"
	// These meters keep cache writes separate without changing the existing
	// uncached_input_tokens meter, which excludes cache reads only.
	QuantityInputWithoutCacheWriteTokens QuantitySource = "usage.input_no_cache_write"
	QuantityInputWithoutCacheTokens      QuantitySource = "usage.input_no_cache"
	QuantityRequestedSeconds             QuantitySource = "request.seconds"
	QuantityGeneratedSeconds             QuantitySource = "result.seconds"
	QuantityBilledSeconds                QuantitySource = "usage.billing_seconds"
	QuantityRequestedImages              QuantitySource = "request.images"
	QuantityGeneratedImages              QuantitySource = "result.images"
	QuantityGeneratedVideos              QuantitySource = "result.videos"
	QuantityMegapixels                   QuantitySource = "result.megapixels"

	ChargeSucceeded      ChargeEvent = "call.succeeded"
	ChargeFailed         ChargeEvent = "call.failed"
	ChargeCancelled      ChargeEvent = "call.cancelled"
	ChargeAccepted       ChargeEvent = "provider.accepted"
	ChargeDelivered      ChargeEvent = "delivery.ready"
	ChargeDeliveryFailed ChargeEvent = "delivery.failed"
)

// A component prices 10^UnitScale units. QuantityStep zero preserves the exact
// measured quantity; a positive step rounds the quantity up before pricing.
//
// PricingMode "expression" replaces the unit_price × quantity multiplication
// with Expr. Everything else about the component is unchanged: charge_event
// still decides when the charge fires, quantity_source still decides what is
// measured, and max_quantity is still the contractual reservation bound.
type RateComponent struct {
	ID                 uint64                `json:"id"`
	Code               string                `json:"component_code"`
	Unit               string                `json:"unit_code"`
	Source             QuantitySource        `json:"quantity_source"`
	Event              ChargeEvent           `json:"charge_event"`
	UnitPrice          string                `json:"unit_price"`
	PricingMode        string                `json:"pricing_mode"`
	PricingExpr        string                `json:"pricing_expr,omitempty"`
	ExpressionDeclared map[string]bool       `json:"expression_declared,omitempty"`
	Expr               *Expression           `json:"-"`
	ExprTiers          map[string][]ExprTier `json:"expression_tiers,omitempty"`
	MaxPrice           string                `json:"max_price"`
	UnitScale          int32                 `json:"unit_scale"`
	QuantityStep       string                `json:"quantity_step"`
	MaxQuantity        string                `json:"max_quantity"`
}

const (
	PricingModeFlat       = "flat"
	PricingModeExpression = "expression"
)

// expression reports whether the component prices through an expression. An
// empty mode is flat so existing rows and existing callers keep working.
func (r RateComponent) expression() bool { return r.PricingMode == PricingModeExpression }

type Currency struct {
	Code           string `json:"code"`
	Version        uint32 `json:"version"`
	FractionDigits int32  `json:"fraction_digits"`
	RoundingMode   string `json:"rounding_mode"`
	MaxAmount      string `json:"max_amount"`
}

type RateSchedule struct {
	Currency   Currency        `json:"currency"`
	Components []RateComponent `json:"components"`
}

// Missing events are unknown, not false. This distinction prevents a missing
// provider response or delivery event from silently turning into a free call.
//
// Expr carries the adapter-supplied variables for expression components. It is
// unused by flat components, so an existing caller needs no change.
type Facts struct {
	Events     map[ChargeEvent]bool      `json:"events,omitempty"`
	Quantities map[QuantitySource]string `json:"quantities,omitempty"`
	Expr       ExprEnv                   `json:"expression,omitempty"`
}

// MergeExpressionFacts overlays observed execution variables on immutable
// request variables without changing either input. Adapter-returned values,
// such as generated seconds and success, take precedence over request values.
func MergeExpressionFacts(request, observed Facts) Facts {
	merged := observed
	if len(observed.Events)+len(request.Events) > 0 {
		merged.Events = make(map[ChargeEvent]bool, len(observed.Events)+len(request.Events))
		for event, occurred := range observed.Events {
			merged.Events[event] = occurred
		}
	}
	for event, occurred := range request.Events {
		if _, exists := merged.Events[event]; !exists {
			merged.Events[event] = occurred
		}
	}
	if len(observed.Quantities)+len(request.Quantities) > 0 {
		merged.Quantities = make(map[QuantitySource]string, len(observed.Quantities)+len(request.Quantities))
		for source, quantity := range observed.Quantities {
			merged.Quantities[source] = quantity
		}
	}
	for source, quantity := range request.Quantities {
		if _, exists := merged.Quantities[source]; !exists {
			merged.Quantities[source] = quantity
		}
	}
	vars := make(map[string]ExprValue, len(request.Expr.Vars)+len(observed.Expr.Vars))
	for name, value := range request.Expr.Vars {
		vars[name] = value
	}
	for name, value := range observed.Expr.Vars {
		vars[name] = value
	}
	if len(vars) > 0 {
		merged.Expr.Vars = vars
	}
	if len(merged.Expr.Declared) == 0 && len(request.Expr.Declared) > 0 {
		merged.Expr.Declared = request.Expr.Declared
	}
	if len(merged.Expr.Tiers) == 0 && len(request.Expr.Tiers) > 0 {
		merged.Expr.Tiers = request.Expr.Tiers
	}
	return merged
}

type ChargeLine struct {
	RateID          uint64                `json:"rate_id"`
	ComponentCode   string                `json:"component_code"`
	Quantity        string                `json:"quantity"`
	Amount          string                `json:"amount"`
	ExceededBound   bool                  `json:"exceeded_bound"`
	Unit            string                `json:"unit_code"`
	UnitPrice       string                `json:"unit_price"`
	PricingMode     string                `json:"pricing_mode"`
	PricingExpr     string                `json:"pricing_expr,omitempty"`
	ExpressionTiers map[string][]ExprTier `json:"expression_tiers,omitempty"`
}

type Charge struct {
	Amount Amount
	Lines  []ChargeLine
}

var componentCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

func (c Currency) Validate() error {
	if c.Code == "" || c.Version == 0 || c.FractionDigits < 0 || c.FractionDigits > 18 {
		return ErrInvalidRate
	}
	max, err := ParseAmount(c.MaxAmount, 18, true)
	if err != nil || max.Sign() <= 0 || max.value.Truncate(c.FractionDigits).Cmp(max.value) != 0 || max.value.GreaterThan(maxSQLDecimal) {
		return ErrInvalidRate
	}
	_, err = Zero().Round(c.FractionDigits, c.RoundingMode)
	return err
}

func (r RateComponent) Validate() error {
	if !componentCode.MatchString(r.Code) || r.UnitScale < 0 || r.UnitScale > 12 {
		return ErrInvalidRate
	}
	// The database pairs pricing_mode with pricing_expr; re-check it here so a
	// component assembled in Go cannot bypass that pairing.
	switch r.PricingMode {
	case "", PricingModeFlat:
		if r.Expr != nil {
			return ErrInvalidRate
		}
	case PricingModeExpression:
		// A provable upper bound is mandatory: Reserve() must produce a
		// pre-authorization amount before the call runs.
		if r.Expr == nil {
			return ErrInvalidRate
		}
		bound, err := ParseAmount(r.MaxPrice, 18, true)
		if err != nil || bound.value.GreaterThan(maxSQLDecimal) {
			return ErrInvalidRate
		}
	default:
		return ErrInvalidRate
	}
	if err := validateUnitSource(r.Unit, r.Source); err != nil {
		return err
	}
	switch r.Event {
	case ChargeSucceeded, ChargeFailed, ChargeCancelled, ChargeAccepted, ChargeDelivered, ChargeDeliveryFailed:
	default:
		return ErrInvalidRate
	}
	price, e1 := ParseAmount(r.UnitPrice, 18, true)
	step, e2 := ParseAmount(r.QuantityStep, 18, true)
	max, e3 := ParseAmount(r.MaxQuantity, 18, true)
	if e1 != nil || e2 != nil || e3 != nil || max.Sign() <= 0 {
		return ErrInvalidRate
	}
	for _, amount := range []Amount{price, step, max} {
		if amount.value.Coefficient().BitLen() > 127 || amount.value.Abs().GreaterThan(maxSQLDecimal) {
			return ErrAmountOverflow
		}
	}
	if r.Source == QuantityOne && (max.String() != "1" || step.Sign() != 0) {
		return ErrInvalidRate
	}
	if discreteQuantity(r.Source) && (max.value.Truncate(0).Cmp(max.value) != 0 || step.value.Truncate(0).Cmp(step.value) != 0) {
		return ErrInvalidRate
	}
	return nil
}

func validateUnitSource(unit string, source QuantitySource) error {
	want := ""
	switch source {
	case QuantityOne:
		want = "request"
	case QuantityInputTokens, QuantityOutputTokens, QuantityUncachedTokens, QuantityCachedTokens,
		QuantityCacheWriteTokens, QuantityInputWithoutCacheWriteTokens, QuantityInputWithoutCacheTokens:
		want = "token"
	case QuantityRequestedSeconds, QuantityGeneratedSeconds, QuantityBilledSeconds:
		want = "second"
	case QuantityRequestedImages, QuantityGeneratedImages:
		want = "image"
	case QuantityGeneratedVideos:
		want = "video"
	case QuantityMegapixels:
		want = "megapixel"
	}
	if want == "" || unit != want {
		return ErrInvalidRate
	}
	return nil
}

func discreteQuantity(source QuantitySource) bool {
	switch source {
	case QuantityOne, QuantityInputTokens, QuantityOutputTokens, QuantityUncachedTokens, QuantityCachedTokens,
		QuantityCacheWriteTokens, QuantityInputWithoutCacheWriteTokens, QuantityInputWithoutCacheTokens,
		QuantityRequestedImages, QuantityGeneratedImages, QuantityGeneratedVideos, QuantityBilledSeconds:
		return true
	default:
		return false
	}
}

func (s RateSchedule) Validate() error {
	if err := s.Currency.Validate(); err != nil {
		return err
	}
	return ValidateRateComponents(s.Components)
}

// ValidateRateComponents checks a commercial schedule independently of its
// settlement currency, for configuration boundaries that load currency later.
func ValidateRateComponents(components []RateComponent) error {
	if len(components) == 0 || len(components) > 32 {
		return ErrInvalidRate
	}
	codes := make(map[string]bool)
	meters := make(map[string]bool)
	for _, r := range components {
		if err := r.Validate(); err != nil {
			return fmt.Errorf("%w: component %s", err, r.Code)
		}
		meter := string(r.Event) + ":" + string(r.Source)
		if codes[r.Code] || meters[meter] {
			return ErrInvalidRate
		}
		codes[r.Code], meters[meter] = true, true
	}
	// Input meters represent disjoint subsets of the same total. Reject any
	// overlapping pair, including cache writes included in ordinary input.
	inputParts := make(map[ChargeEvent]uint8)
	for _, r := range components {
		parts := inputTokenParts(r.Source)
		if parts&inputParts[r.Event] != 0 {
			return ErrInvalidRate
		}
		inputParts[r.Event] |= parts
	}
	return nil
}

func inputTokenParts(source QuantitySource) uint8 {
	const ordinary, read, write = uint8(1), uint8(2), uint8(4)
	switch source {
	case QuantityInputTokens:
		return ordinary | read | write
	case QuantityUncachedTokens:
		return ordinary | write
	case QuantityCachedTokens:
		return read
	case QuantityCacheWriteTokens:
		return write
	case QuantityInputWithoutCacheWriteTokens:
		return ordinary | read
	case QuantityInputWithoutCacheTokens:
		return ordinary
	default:
		return 0
	}
}

// Reserve uses declared contractual bounds, not a heuristic token estimate.
func (s RateSchedule) Reserve() (Charge, error) { return s.calculate(Facts{}, true) }

func (s RateSchedule) Evaluate(f Facts) (Charge, error) { return s.calculate(f, false) }

func (s RateSchedule) calculate(f Facts, reserve bool) (Charge, error) {
	if err := s.Validate(); err != nil {
		return Charge{}, err
	}
	charge := Charge{Amount: Zero()}
	for _, r := range s.Components {
		quantity := r.MaxQuantity
		if !reserve {
			occurred, known := f.Events[r.Event]
			if !known {
				return Charge{}, fmt.Errorf("%w: %s", ErrMissingFact, r.Event)
			}
			if !occurred {
				continue
			}
			if r.Source == QuantityOne {
				quantity = "1"
			} else {
				var ok bool
				quantity, ok = f.Quantities[r.Source]
				if !ok {
					return Charge{}, fmt.Errorf("%w: %s", ErrMissingFact, r.Source)
				}
			}
		}
		q, err := ParseAmount(quantity, 18, true)
		if err != nil || q.value.Abs().GreaterThan(maxSQLDecimal) || (discreteQuantity(r.Source) && q.value.Truncate(0).Cmp(q.value) != 0) {
			return Charge{}, ErrInvalidAmount
		}
		bound, _ := ParseAmount(r.MaxQuantity, 18, true)
		step, _ := ParseAmount(r.QuantityStep, 18, true)
		price, _ := ParseAmount(r.UnitPrice, 18, true)
		measured := q
		if step.Sign() > 0 {
			whole, remainder := q.value.QuoRem(step.value, 0)
			if !remainder.IsZero() {
				whole = whole.Add(oneDecimal)
			}
			q = Amount{value: whole.Mul(step.value)}
		}
		line := Amount{value: q.value.Mul(price.value).Shift(-r.UnitScale)}
		if r.expression() {
			// Reservations use the published maximum; settlement evaluates the
			// expression against the immutable request facts.
			if reserve {
				line, _ = ParseAmount(r.MaxPrice, 18, true)
			} else {
				env := f.Expr
				if len(r.ExpressionDeclared) > 0 {
					env.Declared = r.ExpressionDeclared
				}
				if len(r.ExprTiers) > 0 {
					env.Tiers = r.ExprTiers
				}
				line, err = r.Expr.Evaluate(env)
				if err != nil {
					return Charge{}, err
				}
				priceBound, boundErr := ParseAmount(r.MaxPrice, 18, true)
				if boundErr != nil {
					return Charge{}, boundErr
				}
				// A live amount above the published bound means the proof and
				// expression disagree.
				if line.Cmp(priceBound) > 0 {
					return Charge{}, ErrAmountOverflow
				}
			}
		}
		charge.Amount = charge.Amount.Add(line)
		charge.Lines = append(charge.Lines, ChargeLine{RateID: r.ID, ComponentCode: r.Code,
			Quantity: q.String(), Amount: line.String(), ExceededBound: measured.Cmp(bound) > 0,
			Unit: r.Unit, UnitPrice: r.UnitPrice, PricingMode: r.PricingMode, PricingExpr: r.PricingExpr,
			ExpressionTiers: cloneRateExpressionTiers(r.ExprTiers)})
	}
	var err error
	charge.Amount, err = charge.Amount.Round(s.Currency.FractionDigits, s.Currency.RoundingMode)
	if err != nil {
		return Charge{}, err
	}
	max, _ := ParseAmount(s.Currency.MaxAmount, 18, true)
	if err := charge.Amount.EnsureRange(max); err != nil {
		return Charge{}, err
	}
	return charge, nil
}

func cloneRateExpressionTiers(source map[string][]ExprTier) map[string][]ExprTier {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string][]ExprTier, len(source))
	for name, tiers := range source {
		clone[name] = append([]ExprTier(nil), tiers...)
	}
	return clone
}
