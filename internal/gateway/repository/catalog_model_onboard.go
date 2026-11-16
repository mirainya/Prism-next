package repository

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/mirainya/Prism/internal/gateway/billing"
	"github.com/mirainya/Prism/internal/gateway/catalog"
)

// CatalogModelOnboardInput describes the complete graph needed to expose one
// new downstream model and route it to one upstream offering. Nested draft
// versions and route ids are intentionally omitted: this operation owns those
// relationships and advances the active catalog exactly once.
type CatalogModelOnboardInput struct {
	ExpectedActiveReleaseID uint64                    `json:"expected_active_release_id"`
	ExpectedConfigVersion   uint64                    `json:"expected_config_version"`
	SKU                     CatalogSKUInput           `json:"sku"`
	Product                 CatalogProductInput       `json:"product"`
	Route                   CatalogModelOnboardRoute  `json:"route"`
	SellRate                CatalogModelOnboardRate   `json:"sell_rate"`
	CostRate                *CatalogModelOnboardRate  `json:"cost_rate,omitempty"`
	AdditionalSellRates     []CatalogModelOnboardRate `json:"additional_sell_rates,omitempty"`
	AdditionalCostRates     []CatalogModelOnboardRate `json:"additional_cost_rates,omitempty"`
}

type CatalogModelOnboardRoute struct {
	Priority uint32 `json:"priority"`
	Weight   uint64 `json:"weight"`
}

// CatalogModelOnboardRate is the operator-facing price shape. Onboarding owns
// the evidence lifecycle, so an operator enters the price once instead of
// creating and approving a separate evidence row before the model exists.
type CatalogModelOnboardRate struct {
	UnitCode       string                 `json:"unit_code"`
	UnitPrice      string                 `json:"unit_price"`
	ComponentCode  string                 `json:"component_code"`
	QuantitySource billing.QuantitySource `json:"quantity_source"`
	ChargeEvent    billing.ChargeEvent    `json:"charge_event"`
	UnitScale      int32                  `json:"unit_scale"`
	QuantityStep   string                 `json:"quantity_step"`
	MaxQuantity    string                 `json:"max_quantity"`
	PricingMode    string                 `json:"pricing_mode"`
	PricingExpr    string                 `json:"pricing_expr"`
	ReasonCode     string                 `json:"reason_code"`
}

func (in *CatalogModelOnboardRate) Normalize() {
	in.UnitCode = strings.ToLower(strings.TrimSpace(in.UnitCode))
	in.UnitPrice = strings.TrimSpace(in.UnitPrice)
	in.ComponentCode = strings.ToLower(strings.TrimSpace(in.ComponentCode))
	in.QuantitySource = billing.QuantitySource(strings.ToLower(strings.TrimSpace(string(in.QuantitySource))))
	in.ChargeEvent = billing.ChargeEvent(strings.ToLower(strings.TrimSpace(string(in.ChargeEvent))))
	in.QuantityStep = strings.TrimSpace(in.QuantityStep)
	in.MaxQuantity = strings.TrimSpace(in.MaxQuantity)
	in.PricingMode = strings.ToLower(strings.TrimSpace(in.PricingMode))
	if in.PricingMode == "" {
		in.PricingMode = billing.PricingModeFlat
	}
	in.PricingExpr = strings.TrimSpace(in.PricingExpr)
	in.ReasonCode = strings.ToLower(strings.TrimSpace(in.ReasonCode))
	if in.ReasonCode == "" {
		in.ReasonCode = "model_onboard"
	}
}

func (in CatalogModelOnboardRate) Validate() error {
	pricing := ratePricingChange{
		UnitPrice: in.UnitPrice, PricingMode: in.PricingMode,
		PricingExpr: in.PricingExpr, ReasonCode: in.ReasonCode,
	}
	if err := pricing.validate(); err != nil {
		return err
	}
	// Expression parsing needs the newly-created SKU/Product domains and runs
	// inside createCatalogRateRow. Validate the meter itself here as a flat
	// component so malformed unit/source/event pairs fail before the lock.
	if err := in.meter().Validate(); err != nil {
		return ErrInvalidInput
	}
	return nil
}

func (in CatalogModelOnboardRate) meter() billing.RateComponent {
	return billing.RateComponent{
		Code: in.ComponentCode, Unit: in.UnitCode, Source: in.QuantitySource,
		Event: in.ChargeEvent, UnitPrice: in.UnitPrice, PricingMode: billing.PricingModeFlat,
		UnitScale: in.UnitScale, QuantityStep: in.QuantityStep, MaxQuantity: in.MaxQuantity,
	}
}

func (in CatalogModelOnboardRate) catalogRate(evidenceID uint64) CatalogRateInput {
	return CatalogRateInput{
		EvidenceID: evidenceID, ComponentCode: in.ComponentCode,
		QuantitySource: in.QuantitySource, ChargeEvent: in.ChargeEvent,
		UnitScale: in.UnitScale, QuantityStep: in.QuantityStep, MaxQuantity: in.MaxQuantity,
		PricingMode: in.PricingMode, PricingExpr: in.PricingExpr,
	}
}

type CatalogModelOnboardResult struct {
	CatalogChangeResult
	SKUID       uint64   `json:"sku_id"`
	ProductID   uint64   `json:"product_id"`
	OfferingID  uint64   `json:"offering_id"`
	CostPlanID  uint64   `json:"cost_plan_id"`
	SellRateID  uint64   `json:"sell_rate_id"`
	CostRateID  uint64   `json:"cost_rate_id"`
	SellRateIDs []uint64 `json:"sell_rate_ids"`
	CostRateIDs []uint64 `json:"cost_rate_ids"`
}

func (in *CatalogModelOnboardInput) Normalize() error {
	in.SKU.Normalize()
	if err := in.Product.Normalize(); err != nil {
		return err
	}
	if options, err := catalog.ReadLLMCapabilities(in.Product.CapabilityConstraints); err != nil {
		return ErrInvalidInput
	} else if options != nil && slices.Contains(in.SKU.CapabilityTags, "llm") {
		for _, feature := range options.Features() {
			if !slices.Contains(in.SKU.CapabilityTags, feature) {
				in.SKU.CapabilityTags = append(in.SKU.CapabilityTags, feature)
			}
		}
		in.SKU.Normalize()
	}
	in.SellRate.Normalize()
	if in.CostRate != nil {
		in.CostRate.Normalize()
	}
	for index := range in.AdditionalSellRates {
		in.AdditionalSellRates[index].Normalize()
	}
	for index := range in.AdditionalCostRates {
		in.AdditionalCostRates[index].Normalize()
	}
	return nil
}

func (in CatalogModelOnboardInput) Validate() error {
	if in.ExpectedActiveReleaseID == 0 || in.ExpectedConfigVersion == 0 ||
		in.SKU.ExpectedVersion != 0 || in.Product.ExpectedVersion != 0 ||
		len(in.Product.Routes) != 0 || in.Route.Weight == 0 || in.Route.Weight > 1000000 {
		return ErrInvalidInput
	}
	sku := in.SKU
	sku.ExpectedVersion = in.ExpectedConfigVersion
	if err := sku.Validate(); err != nil {
		return err
	}
	product := in.Product
	product.ExpectedVersion = in.ExpectedConfigVersion
	product.Routes = []CatalogRouteInput{{SKUID: 1, Priority: in.Route.Priority, Weight: in.Route.Weight}}
	if err := product.Validate(); err != nil {
		return err
	}
	if err := validateModelOnboardRates(in.SellRate, in.AdditionalSellRates); err != nil {
		return err
	}
	if in.CostRate == nil {
		if len(in.AdditionalCostRates) != 0 {
			return ErrInvalidInput
		}
	} else if err := validateModelOnboardRates(*in.CostRate, in.AdditionalCostRates); err != nil {
		return err
	}
	return nil
}

func validateModelOnboardRates(primary CatalogModelOnboardRate, additional []CatalogModelOnboardRate) error {
	if len(additional) > 31 {
		return ErrInvalidInput
	}
	rates := append([]CatalogModelOnboardRate{primary}, additional...)
	components := make([]billing.RateComponent, 0, len(rates))
	for _, rate := range rates {
		if err := rate.Validate(); err != nil {
			return err
		}
		components = append(components, rate.meter())
	}
	if billing.ValidateRateComponents(components) != nil {
		return ErrInvalidInput
	}
	return nil
}

// OnboardCatalogModel creates the complete model, commercial, and upstream
// graph in the caller's transaction. No intermediate version is visible: an
// error leaves rollback to Store.WithTx and the active version advances only
// after every content row exists.
func (s *Store) OnboardCatalogModel(ctx context.Context, tx *sql.Tx, in CatalogModelOnboardInput, sign RateEvidenceSigner, actorID uint64) (CatalogModelOnboardResult, error) {
	if tx == nil || sign == nil || actorID == 0 {
		return CatalogModelOnboardResult{}, ErrInvalidInput
	}
	if err := in.Normalize(); err != nil {
		return CatalogModelOnboardResult{}, err
	}
	if err := in.Validate(); err != nil {
		return CatalogModelOnboardResult{}, err
	}
	active, err := lockActiveCatalogRelease(ctx, tx, in.ExpectedActiveReleaseID, in.ExpectedConfigVersion)
	if err != nil {
		return CatalogModelOnboardResult{}, fmt.Errorf("lock active catalog release: %w", err)
	}

	sku := in.SKU
	sku.ExpectedVersion = in.ExpectedConfigVersion
	// New-model onboarding must never reuse a global identity from another
	// Key, a hidden model, or an older release. Association is a separate action.
	modelID, modelNameID, err := createNewCatalogModelIdentity(ctx, tx, sku.ModelCode, sku.APIName)
	if err != nil {
		return CatalogModelOnboardResult{}, err
	}
	skuID, err := createCatalogSKURowsForModel(ctx, tx, active.ID, active.SemanticDigest, sku, modelID, modelNameID)
	if err != nil {
		return CatalogModelOnboardResult{}, fmt.Errorf("create catalog SKU: %w", err)
	}

	product := in.Product
	product.ExpectedVersion = in.ExpectedConfigVersion
	product.Routes = []CatalogRouteInput{{SKUID: skuID, Priority: in.Route.Priority, Weight: in.Route.Weight}}
	productID, offeringID, err := createCatalogProductRows(ctx, tx, active.ID, product)
	if err != nil {
		return CatalogModelOnboardResult{}, fmt.Errorf("create catalog product: %w", err)
	}
	if err := validateSKUVariant(ctx, tx, active.ID, skuID, sku.VariantCode); err != nil {
		return CatalogModelOnboardResult{}, fmt.Errorf("validate SKU variant: %w", err)
	}
	downstreamPaths, err := modelOnboardDownstreamPaths(product.AdapterCode, product.AdapterVersion, sku.RouteTemplate)
	if err != nil {
		return CatalogModelOnboardResult{}, fmt.Errorf("load downstream paths: %w", err)
	}
	if err := validateRequestedDownstreamPaths(ctx, tx, active.ID, skuID, downstreamPaths); err != nil {
		return CatalogModelOnboardResult{}, fmt.Errorf("validate downstream path: %w", err)
	}
	for _, path := range downstreamPaths {
		if _, err := tx.ExecContext(ctx, `INSERT IGNORE INTO gw_sku_downstream_paths(release_id,sku_id,path,created_at) VALUES (?,?,?,?)`, active.ID, skuID, path, nowUTC()); err != nil {
			return CatalogModelOnboardResult{}, fmt.Errorf("declare downstream path: %w", err)
		}
	}

	var costPlanID uint64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM gw_cost_plans WHERE release_id=? AND offering_id=? AND plan_code=?`, active.ID, offeringID, product.CostPlanCode).Scan(&costPlanID); err == sql.ErrNoRows {
		return CatalogModelOnboardResult{}, fmt.Errorf("load cost plan: %w", ErrNotFound)
	} else if err != nil {
		return CatalogModelOnboardResult{}, fmt.Errorf("load cost plan: %w", err)
	}
	currencyCode, currencyVersion, err := activeSettlementCurrency(ctx, tx)
	if err != nil {
		return CatalogModelOnboardResult{}, fmt.Errorf("load settlement currency: %w", err)
	}
	evidenceStamp := nowUTC().Format(time.RFC3339Nano)
	sellRateIDs, err := s.createModelOnboardRates(ctx, tx, "sell", active.ID, skuID, sku.SKUCode, evidenceStamp, currencyCode, currencyVersion, in.SellRate, in.AdditionalSellRates, sign, actorID)
	if err != nil {
		return CatalogModelOnboardResult{}, fmt.Errorf("create sell rate: %w", err)
	}
	costRateIDs := []uint64{}
	var costRateID uint64
	if in.CostRate != nil {
		costRateIDs, err = s.createModelOnboardRates(ctx, tx, "cost", active.ID, costPlanID, product.ProductCode, evidenceStamp, currencyCode, currencyVersion, *in.CostRate, in.AdditionalCostRates, sign, actorID)
		if err != nil {
			return CatalogModelOnboardResult{}, fmt.Errorf("create cost rate: %w", err)
		}
		costRateID = costRateIDs[0]
	}
	sellRateID := sellRateIDs[0]
	if err := CheckCatalogStructure(ctx, tx, active.ID); err != nil {
		return CatalogModelOnboardResult{}, fmt.Errorf("check catalog structure: %w", err)
	}
	if err := CheckCatalogPricing(ctx, tx, active.ID); err != nil {
		return CatalogModelOnboardResult{}, fmt.Errorf("check catalog pricing: %w", err)
	}

	version, err := advanceActiveCatalog(ctx, tx, active, "catalog_change.model_onboard", actorID, ginSafeMetadata{
		"sku_id": skuID, "sku_code": sku.SKUCode,
		"product_id": productID, "product_code": product.ProductCode,
		"offering_id": offeringID, "cost_plan_id": costPlanID,
		"sell_rate_id": sellRateID, "cost_rate_id": costRateID,
		"sell_rate_ids": sellRateIDs, "cost_rate_ids": costRateIDs,
	})
	if err != nil {
		return CatalogModelOnboardResult{}, fmt.Errorf("advance active catalog: %w", err)
	}
	return CatalogModelOnboardResult{
		CatalogChangeResult: CatalogChangeResult{ReleaseID: active.ID, ConfigVersion: version, Activated: true, SourceReleaseID: active.ID},
		SKUID:               skuID, ProductID: productID, OfferingID: offeringID, CostPlanID: costPlanID,
		SellRateID: sellRateID, CostRateID: costRateID,
		SellRateIDs: sellRateIDs, CostRateIDs: costRateIDs,
	}, nil
}

// Every component gets its own evidence reference, even when input and output
// happen to have identical prices. All rows still belong to the same transaction.
func (s *Store) createModelOnboardRates(ctx context.Context, tx *sql.Tx, kind string, releaseID, parentID uint64, code, stamp, currencyCode string, currencyVersion uint32, primary CatalogModelOnboardRate, additional []CatalogModelOnboardRate, sign RateEvidenceSigner, actorID uint64) ([]uint64, error) {
	rates := append([]CatalogModelOnboardRate{primary}, additional...)
	ids := make([]uint64, 0, len(rates))
	for _, rate := range rates {
		evidenceID, err := s.declareOperatorPrice(ctx, tx, operatorPriceDeclaration{
			Reference: fmt.Sprintf("prism-console:model-onboard:%s:%s:%s:%s", kind, code, stamp, rate.ComponentCode),
			UnitCode:  rate.UnitCode, UnitPrice: rate.UnitPrice,
			CurrencyCode: currencyCode, CurrencyVersion: currencyVersion, ReasonCode: rate.ReasonCode,
		}, sign, actorID)
		if err != nil {
			return nil, fmt.Errorf("declare %s price: %w", rate.ComponentCode, err)
		}
		id, err := createCatalogRateRow(ctx, tx, kind, releaseID, parentID, rate.catalogRate(evidenceID))
		if err != nil {
			return nil, fmt.Errorf("create %s rate: %w", rate.ComponentCode, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func modelOnboardDownstreamPaths(adapterCode string, adapterVersion uint32, operationPath string) ([]string, error) {
	if downstreamPathSource == nil {
		return []string{operationPath}, nil
	}
	paths, err := downstreamPathSource.DownstreamPaths(adapterCode, adapterVersion)
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		if path == operationPath {
			// The manifest is an allowlist, not a grant. A SKU only exposes the
			// operation selected by its route template; additional operations must
			// be configured explicitly for models that actually support them.
			return []string{operationPath}, nil
		}
	}
	return nil, ErrInvalidInput
}

func activeSettlementCurrency(ctx context.Context, tx *sql.Tx) (string, uint32, error) {
	var code string
	var version uint32
	err := tx.QueryRowContext(ctx, `SELECT s.currency_code,s.currency_version
FROM billing_system_state s
JOIN billing_currency_definitions d ON d.currency_code=s.currency_code AND d.definition_version=s.currency_version AND d.status='active'
WHERE s.id=1 FOR SHARE`).Scan(&code, &version)
	if err == sql.ErrNoRows {
		return "", 0, ErrConflict
	}
	if err != nil {
		return "", 0, err
	}
	return code, version, nil
}
