package service

import (
	"regexp"

	"github.com/mirainya/Prism/internal/gateway/billing"
	"github.com/mirainya/Prism/pkg/config"
	"github.com/shopspring/decimal"
)

// PriceDisplay only projects read responses; never use it for billing schedules.
type PriceDisplay struct {
	multiplier decimal.Decimal
}

func NewPriceDisplay() (PriceDisplay, error) {
	server := config.ServerConfig{}
	if cfg := config.Get(); cfg != nil {
		server = cfg.Server
	}
	multiplier, err := server.DisplayPriceMultiplier()
	return PriceDisplay{multiplier: multiplier}, err
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

func (p PriceDisplay) Formula(value string) string {
	if value == "" || p.multiplier.Equal(decimal.NewFromInt(1)) {
		return value
	}
	return "(" + value + ") * " + p.multiplier.String()
}

func (p PriceDisplay) Rates(source []billing.RateComponent) []billing.RateComponent {
	rates := append([]billing.RateComponent(nil), source...)
	for index := range rates {
		rate := &rates[index]
		rate.UnitPrice = p.Amount(rate.UnitPrice)
		rate.MaxPrice = p.Amount(rate.MaxPrice)
		rate.PricingExpr = p.Formula(rate.PricingExpr)
	}
	return rates
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
