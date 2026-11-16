package service

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"

	"github.com/mirainya/Prism/internal/gateway/billing"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/shopspring/decimal"
)

// PriceDisplay only projects read responses; never use it for billing schedules.
type PriceDisplay struct {
	multiplier decimal.Decimal
}

type PriceDisplays struct {
	settings repository.PriceDisplaySettings
}

func LoadPriceDisplays(ctx context.Context, db *sql.DB) (PriceDisplays, error) {
	store, err := repository.New(db)
	if err != nil {
		return PriceDisplays{}, err
	}
	settings, err := store.ReadPriceDisplaySettings(ctx)
	return PriceDisplays{settings: settings}, err
}

func PriceDisplaysFromSettings(settings repository.PriceDisplaySettings) (PriceDisplays, error) {
	return PriceDisplays{settings: settings}, settings.Validate()
}

func (s *QueryService) priceDisplays(ctx context.Context, db *sql.DB) (PriceDisplays, error) {
	if s.priceDisplayPreview != nil {
		return PriceDisplaysFromSettings(*s.priceDisplayPreview)
	}
	return LoadPriceDisplays(ctx, db)
}

// PreviewPriceDisplay uses the same projection as the public model detail API.
func PreviewPriceDisplay(ctx context.Context, db *sql.DB, settings repository.PriceDisplaySettings, apiName string) (*AvailableModelCapability, error) {
	query := newQueryServiceWithDB(db)
	query.priceDisplayPreview = &settings
	return query.GetModelDetail(ctx, apiName)
}

// ValidatePriceDisplayProjection prevents saving settings that cannot be
// represented by the public pricing DSL for the active catalog.
func ValidatePriceDisplayProjection(ctx context.Context, db *sql.DB, settings repository.PriceDisplaySettings) error {
	query := newQueryServiceWithDB(db)
	query.priceDisplayPreview = &settings
	_, err := query.queryCapabilities(ctx, "", "", capabilityQuery{prices: true, allPrices: true})
	return err
}

func (p PriceDisplays) For(modelCode, skuCode string) PriceDisplay {
	value := p.settings.DefaultMultiplier
	if override, ok := p.settings.ModelMultipliers[modelCode]; ok {
		value = override
	}
	if override, ok := p.settings.SKUMultipliers[skuCode]; ok {
		value = override
	}
	return PriceDisplay{multiplier: decimal.RequireFromString(value)}
}

func (p PriceDisplays) ModelDescription(modelCode string, skuCodes []string, value string) string {
	display := p.For(modelCode, "")
	if len(skuCodes) > 0 {
		display = p.For(modelCode, skuCodes[0])
	}
	for _, code := range skuCodes {
		if !p.For(modelCode, code).multiplier.Equal(display.multiplier) {
			return descriptionPrice.ReplaceAllString(value, "价格见规格表")
		}
	}
	return display.Description(value)
}

func PriceDisplayForSKU(ctx context.Context, db *sql.DB, skuID uint64) (PriceDisplay, error) {
	displays, err := LoadPriceDisplays(ctx, db)
	if err != nil {
		return PriceDisplay{}, err
	}
	var modelCode, skuCode string
	err = db.QueryRowContext(ctx, `SELECT m.model_code,s.sku_code FROM gw_skus s JOIN gw_model_operations mo ON mo.id=s.model_operation_id AND mo.release_id=s.release_id JOIN gw_catalog_models cm ON cm.id=mo.catalog_model_id AND cm.release_id=mo.release_id JOIN gw_models m ON m.id=cm.model_id WHERE s.id=?`, skuID).Scan(&modelCode, &skuCode)
	if err != nil {
		return PriceDisplay{}, err
	}
	return displays.For(modelCode, skuCode), nil
}

func (p PriceDisplay) Amount(value string) string {
	if value == "" || p.multiplier.Equal(decimal.NewFromInt(1)) {
		return value
	}
	amount, err := decimal.NewFromString(value)
	if err != nil {
		return value
	}
	return amount.Mul(p.multiplier).String()
}

func (p PriceDisplay) TotalAmount(value string) string {
	amount, err := decimal.NewFromString(p.Amount(value))
	if err != nil {
		return value
	}
	return amount.Truncate(2).StringFixed(2)
}

func (p PriceDisplay) Rates(source []billing.RateComponent) ([]billing.RateComponent, error) {
	rates := append([]billing.RateComponent(nil), source...)
	for index := range rates {
		rate := &rates[index]
		rate.UnitPrice = p.Amount(rate.UnitPrice)
		rate.MaxPrice = p.Amount(rate.MaxPrice)
		if rate.PricingMode == billing.PricingModeExpression {
			expression := rate.Expr
			if expression == nil {
				var err error
				expression, err = billing.ParseExpression(rate.PricingExpr, rate.ExpressionDeclared)
				if err != nil {
					return nil, fmt.Errorf("project public price: %w", err)
				}
			}
			var err error
			rate.PricingExpr, rate.ExprTiers, err = expression.DisplayFormula(p.multiplier, rate.ExprTiers)
			if err != nil {
				return nil, fmt.Errorf("project public price: %w", err)
			}
			rate.Expr = nil
			rate.UnitPrice = ""
		}
	}
	return rates, nil
}

// PublicRateComponent explicitly limits the price-list response to public facts.
type PublicRateComponent struct {
	ID           uint64                        `json:"id"`
	Code         string                        `json:"component_code"`
	Unit         string                        `json:"unit_code"`
	Source       billing.QuantitySource        `json:"quantity_source"`
	Event        billing.ChargeEvent           `json:"charge_event"`
	UnitPrice    string                        `json:"unit_price,omitempty"`
	PricingMode  string                        `json:"pricing_mode"`
	PricingExpr  string                        `json:"pricing_expr,omitempty"`
	ExprTiers    map[string][]billing.ExprTier `json:"expression_tiers,omitempty"`
	MaxPrice     string                        `json:"max_price,omitempty"`
	UnitScale    int32                         `json:"unit_scale"`
	QuantityStep string                        `json:"quantity_step"`
	MaxQuantity  string                        `json:"max_quantity"`
}

func (p PriceDisplay) PublicRates(source []billing.RateComponent) ([]PublicRateComponent, error) {
	rates, err := p.Rates(source)
	if err != nil {
		return nil, err
	}
	result := make([]PublicRateComponent, 0, len(rates))
	for _, rate := range rates {
		result = append(result, PublicRateComponent{
			ID: rate.ID, Code: rate.Code, Unit: rate.Unit, Source: rate.Source, Event: rate.Event,
			UnitPrice: rate.UnitPrice, PricingMode: rate.PricingMode, PricingExpr: rate.PricingExpr,
			ExprTiers: rate.ExprTiers, MaxPrice: rate.MaxPrice, UnitScale: rate.UnitScale,
			QuantityStep: rate.QuantityStep, MaxQuantity: rate.MaxQuantity,
		})
	}
	return result, nil
}

var (
	descriptionPrice  = regexp.MustCompile(`(?:[¥￥$]\s*[0-9]+(?:\.[0-9]+)?|[0-9]+(?:\.[0-9]+)?\s*(?:元|(?i:CREDIT|credits|USD|CNY)))`)
	descriptionAmount = regexp.MustCompile(`[0-9]+(?:\.[0-9]+)?`)
)

func (p PriceDisplay) Description(value string) string {
	return descriptionPrice.ReplaceAllStringFunc(value, func(price string) string {
		return descriptionAmount.ReplaceAllStringFunc(price, p.Amount)
	})
}
