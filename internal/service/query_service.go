package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mirainya/Prism/internal/gateway/billing"
	"github.com/mirainya/Prism/internal/gateway/catalog"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
)

type QueryService struct {
	db                  *sql.DB
	priceDisplayPreview *repository.PriceDisplaySettings
}

const (
	RouteOperationImagesGenerate = "images.generate"
	RouteOperationImagesEdit     = "images.edit"
	RouteOperationVideosGenerate = "videos.generate"
)

type AvailableModelSKU struct {
	ID              uint64   `json:"id"`
	Code            string   `json:"code"`
	DeliveryMode    string   `json:"delivery_mode"`
	MaxResults      uint32   `json:"max_results"`
	IdempotencyMode string   `json:"idempotency_mode"`
	ServiceTiers    []string `json:"service_tiers"`
}

type AvailableModelOperation struct {
	ID             string              `json:"id"`
	Method         string              `json:"method"`
	Path           string              `json:"path"`
	SupportsStream bool                `json:"supports_stream"`
	SKUs           []AvailableModelSKU `json:"skus"`
}

type AvailableModelChannel struct {
	ChannelID       uint64 `json:"channel_id"`
	ChannelType     string `json:"channel_type"`
	ChannelName     string `json:"channel_name"`
	Model           string `json:"model"`
	Protocol        string `json:"protocol"`
	InteractionMode string `json:"interaction_mode"`
	RouteOperation  string `json:"route_operation"`
}

type ThinkingInfo struct {
	Default string              `json:"default"`
	Locked  bool                `json:"locked"`
	Options []ThinkingLevelInfo `json:"options"`
}

type ThinkingLevelInfo struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type AvailableModelCapability struct {
	ID                     string                    `json:"id"`
	Code                   string                    `json:"code"`
	ModelCode              string                    `json:"model_code"`
	Object                 string                    `json:"object"`
	Name                   string                    `json:"name"`
	Type                   string                    `json:"type"`
	Types                  []string                  `json:"types"`
	Description            string                    `json:"description"`
	Visibility             string                    `json:"visibility"`
	Features               []string                  `json:"features"`
	Operations             []AvailableModelOperation `json:"operations"`
	Channels               []AvailableModelChannel   `json:"channels"`
	Transports             []string                  `json:"transports"`
	SupportsStream         bool                      `json:"supports_stream,omitempty"`
	DefaultStream          bool                      `json:"default_stream,omitempty"`
	SupportsTools          bool                      `json:"supports_tools,omitempty"`
	SupportsResponseFormat bool                      `json:"supports_response_format,omitempty"`
	SupportsMultimodal     bool                      `json:"supports_multimodal,omitempty"`
	MaxTokens              int                       `json:"max_tokens,omitempty"`
	Group                  string                    `json:"group,omitempty"`
	Thinking               *ThinkingInfo             `json:"thinking,omitempty"`
	Availability           *PricingAvailability      `json:"availability,omitempty"`
	VideoOptions           map[string]any            `json:"video_options,omitempty"`
	LLMOptions             map[string]any            `json:"llm_options,omitempty"`
	Pricing                *AvailableModelPricing    `json:"pricing,omitempty"`
	Sort                   int                       `json:"-"`
	operationKeys          map[string]int
	channelKeys            map[string]struct{}
	typeSet                map[string]struct{}
	transportSet           map[string]struct{}
	videoVariants          []map[string]any
	pricingSKUs            map[string]struct{}
	displaySKUCodes        []string
	llmOptionsSeen         bool
}

type AvailableModelPricing struct {
	Currency        string                         `json:"currency"`
	CurrencyVersion uint32                         `json:"currency_version"`
	FractionDigits  int32                          `json:"fraction_digits"`
	RoundingMode    string                         `json:"rounding_mode"`
	Variants        []AvailableModelPricingVariant `json:"variants"`
}

type AvailableModelPricingVariant struct {
	Operation    string                         `json:"operation"`
	ServiceTiers []string                       `json:"service_tiers"`
	Constraints  map[string]any                 `json:"constraints,omitempty"`
	Components   []AvailableModelPriceComponent `json:"components"`
}

type AvailableModelPriceComponent struct {
	Component   string                        `json:"component"`
	Unit        string                        `json:"unit"`
	UnitPrice   string                        `json:"unit_price,omitempty"`
	UnitScale   int32                         `json:"unit_scale,omitempty"`
	MaxQuantity string                        `json:"max_quantity"`
	Quantity    string                        `json:"quantity_source"`
	Event       string                        `json:"charge_event"`
	Mode        string                        `json:"mode"`
	Formula     string                        `json:"formula,omitempty"`
	Tiers       map[string][]billing.ExprTier `json:"tiers,omitempty"`
	Maximum     string                        `json:"maximum,omitempty"`
}

func NewQueryService() *QueryService {
	return &QueryService{}
}

func newQueryServiceWithDB(db *sql.DB) *QueryService {
	return &QueryService{db: db}
}

func (s *QueryService) store() (*repository.Store, error) {
	db := s.db
	if db == nil {
		var err error
		db, err = model.DB().DB()
		if err != nil {
			return nil, err
		}
	}
	return repository.New(db)
}

func (s *QueryService) activeRoutes(ctx context.Context) ([]repository.PublicCatalogRoute, *repository.Store, error) {
	store, err := s.store()
	if err != nil {
		return nil, nil, err
	}
	rows, err := store.ListActivePublicCatalog(ctx)
	return rows, store, err
}

func (s *QueryService) ListAvailableChannels(ctx context.Context) ([]string, error) {
	rows, _, err := s.activeRoutes(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(rows))
	result := make([]string, 0)
	for _, row := range rows {
		if _, exists := seen[row.ChannelCode]; exists {
			continue
		}
		seen[row.ChannelCode] = struct{}{}
		result = append(result, row.ChannelCode)
	}
	sort.Strings(result)
	return result, nil
}

func (s *QueryService) ListAvailableCapabilities(ctx context.Context, channelCode, modelType string) ([]AvailableModelCapability, error) {
	return s.queryCapabilities(ctx, channelCode, modelType, capabilityQuery{details: true, prices: true, availability: true})
}

type capabilityQuery struct {
	modelCode    string
	details      bool
	prices       bool
	allPrices    bool
	availability bool
}

// ListModelSummaries does not load billing schedules or live observations.
func (s *QueryService) ListModelSummaries(ctx context.Context) ([]AvailableModelCapability, error) {
	return s.queryCapabilities(ctx, "", "", capabilityQuery{})
}

// GetModelDetail enriches only the requested public model with capabilities and prices.
func (s *QueryService) GetModelDetail(ctx context.Context, code string) (*AvailableModelCapability, error) {
	if code == "" {
		return nil, nil
	}
	items, err := s.queryCapabilities(ctx, "", "", capabilityQuery{modelCode: code, details: true, prices: true, allPrices: true})
	if err != nil || len(items) == 0 {
		return nil, err
	}
	return &items[0], nil
}

func (s *QueryService) queryCapabilities(ctx context.Context, channelCode, modelType string, query capabilityQuery) ([]AvailableModelCapability, error) {
	rows, store, err := s.activeRoutes(ctx)
	if err != nil {
		return nil, err
	}
	displays, err := s.priceDisplays(ctx, store.DB())
	if err != nil {
		return nil, err
	}
	channelCode = strings.TrimSpace(channelCode)
	modelType = strings.ToLower(strings.TrimSpace(modelType))
	filtered := rows[:0]
	for _, row := range rows {
		if channelCode != "" && row.ChannelCode != channelCode ||
			modelType != "" && publicOperationModelType(row.DownstreamOperation, row.DownstreamPath) != modelType ||
			query.modelCode != "" && row.APIName != query.modelCode {
			continue
		}
		filtered = append(filtered, row)
	}
	rows = filtered
	var availability map[uint64]repository.SKUAvailability
	var providerMetadata map[repository.ProviderModelMetadataKey]repository.ProviderModelMetadata
	var sellSchedules map[uint64]billing.RateSchedule
	if len(rows) != 0 {
		if query.availability {
			availability, err = store.LoadSKUAvailability(ctx, rows[0].ReleaseID)
			if err != nil {
				availability = nil
			}
		}
		providerMetadata, err = store.LoadProviderModelMetadata(ctx)
		if err != nil {
			providerMetadata = nil
		}
		if query.allPrices {
			sellSchedules = make(map[uint64]billing.RateSchedule)
			for _, row := range rows {
				if _, exists := sellSchedules[row.SKUID]; exists {
					continue
				}
				schedule, err := repository.LoadSellSchedule(ctx, store.DB(), row.ReleaseID, row.SKUID)
				if err != nil {
					return nil, fmt.Errorf("load model sell schedule: %w", err)
				}
				sellSchedules[row.SKUID] = schedule
			}
		} else if query.prices {
			for _, row := range rows {
				if row.DownstreamHTTPMethod != "POST" || row.DownstreamPath != "/v1/videos/generations" {
					continue
				}
				sellSchedules, err = repository.LoadSellSchedules(ctx, store.DB(), row.ReleaseID)
				if err != nil {
					return nil, fmt.Errorf("load active sell schedules: %w", err)
				}
				break
			}
		}
	}
	items := make(map[string]*AvailableModelCapability)
	for _, row := range rows {
		operationType := publicOperationModelType(row.DownstreamOperation, row.DownstreamPath)
		tiers, err := decodeCatalogStringArray(row.ServiceTiers)
		if err != nil || len(tiers) == 0 {
			return nil, fmt.Errorf("active catalog SKU %d has invalid service tiers", row.SKUID)
		}
		features := decodeCatalogFeatures(row.CapabilityTags)
		item := items[row.APIName]
		if item == nil {
			name := strings.TrimSpace(row.DisplayName)
			if name == "" {
				name = row.APIName
			}
			item = &AvailableModelCapability{
				ID: row.APIName, Code: row.APIName, ModelCode: row.ModelCode,
				Object: "model_capability", Name: name, Description: row.Description,
				Visibility: row.Visibility, Features: []string{}, Types: []string{},
				Operations: []AvailableModelOperation{}, Channels: []AvailableModelChannel{},
				Transports: []string{}, Sort: row.SortOrder,
				operationKeys: make(map[string]int), channelKeys: make(map[string]struct{}),
				typeSet: make(map[string]struct{}), transportSet: make(map[string]struct{}),
				pricingSKUs: make(map[string]struct{}),
			}
			items[row.APIName] = item
		}
		item.addType(operationType)
		item.applyFeatures(features)
		if operationType == string(model.ModelTypeChat) {
			if err := item.addLLMOptions(row.CapabilityConstraints); err != nil {
				return nil, fmt.Errorf("active chat model %s has invalid capability metadata: %w", row.APIName, err)
			}
		}
		item.addRoute(row, tiers)
		isVideo := row.DownstreamHTTPMethod == "POST" && row.DownstreamPath == "/v1/videos/generations"
		if query.details && isVideo {
			if err := item.addVideoOptionsForModel(row.CapabilityConstraints, row.VendorModel, tiers); err != nil {
				return nil, fmt.Errorf("active catalog product %s has invalid video constraints: %w", row.VendorModel, err)
			}
		}
		if query.prices && (query.allPrices || isVideo) {
			schedule, exists := sellSchedules[row.SKUID]
			if !exists {
				return nil, fmt.Errorf("active SKU %d has no sell schedule", row.SKUID)
			}
			if err := item.addPricing(row, tiers, schedule, displays.For(row.ModelCode, row.SKUCode)); err != nil {
				return nil, fmt.Errorf("active SKU %d has invalid sell schedule: %w", row.SKUID, err)
			}
		}
		item.displaySKUCodes = append(item.displaySKUCodes, row.SKUCode)
		item.applyAvailability(availability[row.SKUID])
		if item.Description == "" {
			metadata := providerMetadata[repository.NewProviderModelMetadataKey(row.ChannelID, row.VendorModel)]
			if metadata.Description != "" {
				item.Description = metadata.Description
			}
		}
	}

	result := make([]AvailableModelCapability, 0, len(items))
	for _, item := range items {
		item.Description = displays.ModelDescription(item.ModelCode, item.displaySKUCodes, item.Description)
		sort.SliceStable(item.Operations, func(i, j int) bool {
			left, right := item.Operations[i], item.Operations[j]
			if operationRank(left.ID) != operationRank(right.ID) {
				return operationRank(left.ID) < operationRank(right.ID)
			}
			if left.Path != right.Path {
				return left.Path < right.Path
			}
			return left.Method < right.Method
		})
		for index := range item.Operations {
			sort.SliceStable(item.Operations[index].SKUs, func(i, j int) bool {
				return item.Operations[index].SKUs[i].Code < item.Operations[index].SKUs[j].Code
			})
		}
		sort.Strings(item.Features)
		sort.Strings(item.Transports)
		item.operationKeys = nil
		item.channelKeys = nil
		item.typeSet = nil
		item.transportSet = nil
		item.videoVariants = nil
		if !query.details {
			item.LLMOptions = nil
		}
		result = append(result, *item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Sort != result[j].Sort {
			return result[i].Sort < result[j].Sort
		}
		return result[i].ID < result[j].ID
	})
	return result, nil
}

func (item *AvailableModelCapability) addPricing(row repository.PublicCatalogRoute, tiers []string, schedule billing.RateSchedule, display PriceDisplay) error {
	if item.Pricing == nil {
		item.Pricing = &AvailableModelPricing{
			Currency: schedule.Currency.Code, CurrencyVersion: schedule.Currency.Version,
			FractionDigits: schedule.Currency.FractionDigits, RoundingMode: schedule.Currency.RoundingMode,
			Variants: []AvailableModelPricingVariant{},
		}
	} else if item.Pricing.Currency != schedule.Currency.Code || item.Pricing.CurrencyVersion != schedule.Currency.Version ||
		item.Pricing.FractionDigits != schedule.Currency.FractionDigits || item.Pricing.RoundingMode != schedule.Currency.RoundingMode {
		return repository.ErrConflict
	}
	if item.pricingSKUs == nil {
		item.pricingSKUs = make(map[string]struct{})
	}
	var constraints map[string]any
	if publicOperationModelType(row.DownstreamOperation, row.DownstreamPath) == string(model.ModelTypeVideo) {
		var err error
		constraints, err = publicVideoConstraintsForModel(row.CapabilityConstraints, row.VendorModel)
		if err != nil {
			return err
		}
	}
	keyBytes, err := json.Marshal(struct {
		SKU         string
		Operation   string
		Tiers       []string
		Constraints map[string]any
	}{row.SKUCode, row.DownstreamOperation, tiers, constraints})
	if err != nil {
		return err
	}
	key := string(keyBytes)
	if _, exists := item.pricingSKUs[key]; exists {
		return nil
	}
	components := make([]AvailableModelPriceComponent, 0, len(schedule.Components))
	rates, err := display.Rates(schedule.Components)
	if err != nil {
		return err
	}
	for _, rate := range rates {
		component := AvailableModelPriceComponent{
			Component: rate.Code, Unit: rate.Unit, UnitScale: rate.UnitScale,
			Quantity: string(rate.Source), Event: string(rate.Event), Mode: rate.PricingMode,
			MaxQuantity: rate.MaxQuantity,
		}
		if rate.PricingMode == billing.PricingModeExpression {
			component.Formula = rate.PricingExpr
			component.Tiers = clonePricingTiers(rate.ExprTiers)
			component.Maximum = rate.MaxPrice
		} else {
			component.Mode = billing.PricingModeFlat
			component.UnitPrice = rate.UnitPrice
		}
		components = append(components, component)
	}
	item.Pricing.Variants = append(item.Pricing.Variants, AvailableModelPricingVariant{
		Operation:    row.DownstreamOperation,
		ServiceTiers: append([]string(nil), tiers...),
		Constraints:  constraints, Components: components,
	})
	item.pricingSKUs[key] = struct{}{}
	return nil
}

func clonePricingTiers(source map[string][]billing.ExprTier) map[string][]billing.ExprTier {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string][]billing.ExprTier, len(source))
	for name, tiers := range source {
		clone[name] = append([]billing.ExprTier(nil), tiers...)
	}
	return clone
}

// addVideoOptions projects the same active product constraints and SKU tiers used
// by /api/docs/videos. Distinct executable routes remain separate variants: a
// union of limits could advertise a combination accepted by no upstream route.
func (item *AvailableModelCapability) addVideoOptions(raw []byte, tiers []string) error {
	return item.addVideoOptionsForModel(raw, "", tiers)
}

func (item *AvailableModelCapability) addVideoOptionsForModel(raw []byte, vendorModel string, tiers []string) error {
	options, err := publicVideoConstraintsForModel(raw, vendorModel)
	if err != nil {
		return err
	}
	options["service_tiers"] = append([]string(nil), tiers...)
	for _, previous := range item.videoVariants {
		previousWithoutTiers := make(map[string]any, len(previous))
		for key, value := range previous {
			if key != "service_tiers" {
				previousWithoutTiers[key] = value
			}
		}
		nextWithoutTiers := make(map[string]any, len(options))
		for key, value := range options {
			if key != "service_tiers" {
				nextWithoutTiers[key] = value
			}
		}
		oldJSON, _ := json.Marshal(previousWithoutTiers)
		nextJSON, _ := json.Marshal(nextWithoutTiers)
		if string(oldJSON) == string(nextJSON) {
			oldTiers, _ := previous["service_tiers"].([]string)
			for _, tier := range tiers {
				if !containsCatalogString(oldTiers, tier) {
					oldTiers = append(oldTiers, tier)
				}
			}
			previous["service_tiers"] = oldTiers
			item.projectVideoOptions()
			return nil
		}
	}
	item.videoVariants = append(item.videoVariants, options)
	item.projectVideoOptions()
	return nil
}

func (item *AvailableModelCapability) projectVideoOptions() {
	if len(item.videoVariants) == 1 {
		item.VideoOptions = item.videoVariants[0]
	} else {
		item.VideoOptions = map[string]any{"variants": item.videoVariants}
	}
}
func (item *AvailableModelCapability) applyAvailability(value repository.SKUAvailability) {
	if value.SKUID == 0 {
		return
	}
	candidate := &PricingAvailability{
		SuccessRate: value.SuccessRate, Source: value.Source, Samples: value.Samples,
		WindowMinutes: value.WindowMinutes, ObservedAt: value.ObservedAt,
		ChannelStatus: value.ChannelStatus, EstimatedCompletionMS: value.EstimatedCompletionMS, AverageCompletionSeconds: value.AverageCompletionSeconds, Buckets: value.Buckets,
	}
	if !value.ChannelObservedAt.IsZero() {
		observed := value.ChannelObservedAt
		candidate.ChannelObservedAt = &observed
	}
	if item.Availability == nil {
		item.Availability = candidate
		return
	}
	current := item.Availability
	if candidate.SuccessRate != "" && (current.SuccessRate == "" || availabilityWorse(candidate, current)) {
		current.SuccessRate = candidate.SuccessRate
		current.Source = candidate.Source
		current.Samples = candidate.Samples
		current.WindowMinutes = candidate.WindowMinutes
		current.ObservedAt = candidate.ObservedAt
	}
	if candidate.ChannelStatus != "" && availabilityStatusRank(candidate.ChannelStatus) > availabilityStatusRank(current.ChannelStatus) {
		current.ChannelStatus = candidate.ChannelStatus
	}
	if current.AverageCompletionSeconds == "" {
		current.AverageCompletionSeconds = candidate.AverageCompletionSeconds
	}
	if len(candidate.Buckets) > 0 && (len(current.Buckets) == 0 || candidate.ObservedAt.After(current.ObservedAt)) {
		current.Buckets = candidate.Buckets
		current.AverageCompletionSeconds = candidate.AverageCompletionSeconds
	}
	if candidate.EstimatedCompletionMS != nil && (current.EstimatedCompletionMS == nil || *candidate.EstimatedCompletionMS > *current.EstimatedCompletionMS) {
		current.EstimatedCompletionMS = candidate.EstimatedCompletionMS
	}
	if candidate.ChannelObservedAt != nil && (current.ChannelObservedAt == nil || candidate.ChannelObservedAt.Before(*current.ChannelObservedAt)) {
		current.ChannelObservedAt = candidate.ChannelObservedAt
	}
	if current.SuccessRate == "" && (current.ObservedAt.IsZero() || candidate.ObservedAt.Before(current.ObservedAt)) {
		current.ObservedAt = candidate.ObservedAt
	}
}

func availabilityWorse(left, right *PricingAvailability) bool {
	if left.SuccessRate == "" {
		return false
	}
	if right.SuccessRate == "" {
		return true
	}
	leftRate, leftErr := billing.ParseAmount(left.SuccessRate, 18, true)
	rightRate, rightErr := billing.ParseAmount(right.SuccessRate, 18, true)
	if leftErr == nil && rightErr == nil && leftRate.Cmp(rightRate) != 0 {
		return leftRate.Cmp(rightRate) < 0
	}
	return left.ObservedAt.Before(right.ObservedAt)
}

func availabilityStatusRank(value string) int {
	status := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(status, "不可用"), strings.Contains(status, "异常"), strings.Contains(status, "unavailable"), strings.Contains(status, "failed"), strings.Contains(status, "error"), status == "down":
		return 3
	case strings.Contains(status, "波动"), strings.Contains(status, "不稳定"), strings.Contains(status, "degrad"), strings.Contains(status, "fluctuat"), strings.Contains(status, "unstable"):
		return 2
	case status == "":
		return 0
	default:
		return 1
	}
}

func (item *AvailableModelCapability) addType(value string) {
	if value == "" {
		return
	}
	if _, exists := item.typeSet[value]; exists {
		return
	}
	item.typeSet[value] = struct{}{}
	item.Types = append(item.Types, value)
	if item.Type == "" || modelTypeRank(value) < modelTypeRank(item.Type) {
		item.Type = value
	}
}

func (item *AvailableModelCapability) applyFeatures(features map[string]bool) {
	for name, enabled := range features {
		if !enabled || containsCatalogString(item.Features, name) {
			continue
		}
		item.Features = append(item.Features, name)
	}
	item.SupportsStream = item.SupportsStream || features["stream"]
	item.SupportsTools = item.SupportsTools || features["tools"]
	item.SupportsResponseFormat = item.SupportsResponseFormat || features["structured_output"] || features["response_format"]
	item.SupportsMultimodal = item.SupportsMultimodal || features["vision"] || features["multimodal"]
}

func (item *AvailableModelCapability) addLLMOptions(raw []byte) error {
	options, err := catalog.ReadLLMCapabilities(raw)
	if err != nil {
		return err
	}
	current := map[string]any{}
	if options != nil {
		encoded, _ := json.Marshal(options)
		if err := json.Unmarshal(encoded, &current); err != nil {
			return err
		}
		features := make(map[string]bool)
		for _, feature := range options.Features() {
			features[feature] = true
		}
		item.applyFeatures(features)
	}
	if !item.llmOptionsSeen {
		item.LLMOptions = current
		item.llmOptionsSeen = true
		return nil
	}
	// A shared public model may only advertise metadata all reachable routes
	// agree on. Missing fields remain unknown instead of borrowing another Key's.
	for key, value := range item.LLMOptions {
		if candidate, exists := current[key]; !exists || candidate != value {
			delete(item.LLMOptions, key)
		}
	}
	return nil
}

func (item *AvailableModelCapability) addRoute(row repository.PublicCatalogRoute, tiers []string) {
	operationKey := row.DownstreamOperation + "\x00" + row.DownstreamHTTPMethod + "\x00" + row.DownstreamPath
	operationIndex, exists := item.operationKeys[operationKey]
	if !exists {
		operationIndex = len(item.Operations)
		item.operationKeys[operationKey] = operationIndex
		item.Operations = append(item.Operations, AvailableModelOperation{
			ID: row.DownstreamOperation, Method: row.DownstreamHTTPMethod, Path: row.DownstreamPath,
			SupportsStream: item.SupportsStream, SKUs: []AvailableModelSKU{},
		})
	}
	operation := &item.Operations[operationIndex]
	operation.SupportsStream = operation.SupportsStream || item.SupportsStream
	if !operationHasSKU(*operation, row.SKUID) {
		operation.SKUs = append(operation.SKUs, AvailableModelSKU{
			ID: row.SKUID, Code: row.SKUCode, DeliveryMode: row.DeliveryMode,
			MaxResults: row.MaxResults, IdempotencyMode: row.IdempotencyMode,
			ServiceTiers: append([]string(nil), tiers...),
		})
	}

	interactionMode := "sync"
	if row.TaskScope == "task" {
		interactionMode = "poll"
	}
	channelKey := fmt.Sprintf("%d\x00%s\x00%s\x00%s", row.ChannelID, row.VendorModel, row.DownstreamOperation, row.DownstreamPath)
	if _, exists := item.channelKeys[channelKey]; !exists {
		item.channelKeys[channelKey] = struct{}{}
		item.Channels = append(item.Channels, AvailableModelChannel{
			ChannelID: row.ChannelID, ChannelType: row.ChannelCode,
			ChannelName: row.ChannelName, Model: row.VendorModel, Protocol: row.Protocol,
			InteractionMode: interactionMode, RouteOperation: row.DownstreamOperation,
		})
	}
	if _, exists := item.transportSet[row.Protocol]; !exists {
		item.transportSet[row.Protocol] = struct{}{}
		item.Transports = append(item.Transports, row.Protocol)
	}
}

func operationHasSKU(operation AvailableModelOperation, skuID uint64) bool {
	for _, sku := range operation.SKUs {
		if sku.ID == skuID {
			return true
		}
	}
	return false
}

func decodeCatalogStringArray(raw []byte) ([]string, error) {
	var values []string
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return nil, fmt.Errorf("invalid string array")
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || containsCatalogString(result, value) {
			continue
		}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func decodeCatalogFeatures(raw []byte) map[string]bool {
	result := make(map[string]bool)
	var object map[string]bool
	if len(raw) > 0 && json.Unmarshal(raw, &object) == nil {
		for name, enabled := range object {
			if enabled {
				result[strings.ToLower(strings.TrimSpace(name))] = true
			}
		}
		return result
	}
	var values []string
	if len(raw) > 0 && json.Unmarshal(raw, &values) == nil {
		for _, value := range values {
			value = strings.ToLower(strings.TrimSpace(value))
			if value != "" {
				result[value] = true
			}
		}
	}
	return result
}

func containsCatalogString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func publicOperationModelType(operationCode, route string) string {
	operationCode = strings.ToLower(operationCode)
	route = strings.ToLower(route)
	switch {
	case strings.Contains(operationCode, "image") || strings.HasPrefix(route, "/v1/images/"):
		return string(model.ModelTypeImage)
	case strings.Contains(operationCode, "video") || strings.HasPrefix(route, "/v1/videos/"):
		return string(model.ModelTypeVideo)
	case strings.Contains(operationCode, "audio") || strings.HasPrefix(route, "/v1/audio/"):
		return string(model.ModelTypeAudio)
	case strings.Contains(operationCode, "embedding") || strings.HasPrefix(route, "/v1/embeddings"):
		return string(model.ModelTypeEmbedding)
	case operationCode == "chat.completions" || operationCode == "responses.create" || operationCode == "messages.create" ||
		strings.HasPrefix(route, "/v1/chat/") || route == "/v1/responses" || route == "/v1/messages":
		return string(model.ModelTypeChat)
	default:
		return "other"
	}
}

func modelTypeRank(value string) int {
	switch value {
	case string(model.ModelTypeChat):
		return 10
	case string(model.ModelTypeImage):
		return 20
	case string(model.ModelTypeVideo):
		return 30
	case string(model.ModelTypeAudio):
		return 40
	case string(model.ModelTypeEmbedding):
		return 50
	default:
		return 100
	}
}

func operationRank(operationID string) int {
	switch operationID {
	case "chat.completions":
		return 10
	case "responses.create":
		return 20
	case "messages.create":
		return 30
	case RouteOperationImagesGenerate:
		return 100
	case RouteOperationImagesEdit:
		return 110
	case RouteOperationVideosGenerate:
		return 200
	default:
		return 1000
	}
}

func (s *QueryService) ListCapabilityChannels(ctx context.Context) ([]AvailableModelCapability, error) {
	return s.ListAvailableCapabilities(ctx, "", "")
}

type PricingService struct {
	query *QueryService
}

func NewPricingService() *PricingService {
	return &PricingService{query: NewQueryService()}
}

type PricingCapability struct {
	Code             string       `json:"code"`
	ModelCode        string       `json:"model_code"`
	Name             string       `json:"name"`
	Type             string       `json:"type"`
	Description      string       `json:"description"`
	Visibility       string       `json:"visibility"`
	SKUs             []PricingSKU `json:"skus"`
	Sort             int          `json:"-"`
	skuIndexes       map[uint64]int
	displayModelCode string
}

type PricingSKU struct {
	ID              uint64                `json:"id"`
	Code            string                `json:"code"`
	Operation       string                `json:"operation"`
	Routes          []PricingRoute        `json:"routes"`
	DeliveryMode    string                `json:"delivery_mode"`
	MaxResults      uint32                `json:"max_results"`
	IdempotencyMode string                `json:"idempotency_mode"`
	ServiceTiers    []string              `json:"service_tiers"`
	Currency        PricingCurrency       `json:"currency"`
	Components      []PublicRateComponent `json:"components"`
	Availability    *PricingAvailability  `json:"availability,omitempty"`
}

// PricingAvailability is the recently observed success rate for a SKU. It is
// absent rather than zero when nothing has been observed, because "no data" and
// "never succeeds" must not look the same to someone choosing a model.
//
// It sits beside the price but is not part of it: the price is fixed by the
// active release and may be proven against it, while this is a measurement that
// moves on its own and carries the timestamp needed to judge how stale it is.
type PricingAvailability struct {
	SuccessRate              string          `json:"success_rate,omitempty"`
	Source                   string          `json:"source,omitempty"`
	Samples                  uint32          `json:"samples,omitempty"`
	WindowMinutes            uint32          `json:"window_minutes,omitempty"`
	ObservedAt               time.Time       `json:"observed_at"`
	ChannelStatus            string          `json:"channel_status,omitempty"`
	AverageCompletionSeconds string          `json:"average_completion_seconds,omitempty"`
	Buckets                  json.RawMessage `json:"buckets,omitempty"`
	EstimatedCompletionMS    *uint64         `json:"estimated_completion_ms,omitempty"`
	ChannelObservedAt        *time.Time      `json:"channel_observed_at,omitempty"`
}

type PricingRoute struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}

type PricingCurrency struct {
	Code           string `json:"code"`
	Version        uint32 `json:"version"`
	FractionDigits int32  `json:"fraction_digits"`
}

func (s *PricingService) GetPricing(ctx context.Context) ([]PricingCapability, error) {
	rows, store, err := s.query.activeRoutes(ctx)
	if err != nil {
		return nil, err
	}
	displays, err := s.query.priceDisplays(ctx, store.DB())
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []PricingCapability{}, nil
	}
	releaseID := rows[0].ReleaseID
	schedules, err := repository.LoadSellSchedules(ctx, store.DB(), releaseID)
	if err != nil {
		return nil, err
	}
	// Availability is advisory. If it cannot be read the catalog is still served
	// without it, because a price list that fails to load is a worse outcome
	// than one missing a success-rate column.
	availability, err := store.LoadSKUAvailability(ctx, releaseID)
	if err != nil {
		availability = nil
	}
	providerMetadata, err := store.LoadProviderModelMetadata(ctx)
	if err != nil {
		providerMetadata = nil
	}
	items := make(map[string]*PricingCapability)
	for _, row := range rows {
		if row.ReleaseID != releaseID {
			return nil, fmt.Errorf("active catalog changed while pricing was read")
		}
		schedule, ok := schedules[row.SKUID]
		if !ok {
			return nil, fmt.Errorf("active catalog SKU %d has no valid sell schedule", row.SKUID)
		}
		tiers, err := decodeCatalogStringArray(row.ServiceTiers)
		if err != nil || len(tiers) == 0 {
			return nil, fmt.Errorf("active catalog SKU %d has invalid service tiers", row.SKUID)
		}
		item := items[row.APIName]
		if item == nil {
			name := strings.TrimSpace(row.DisplayName)
			if name == "" {
				name = row.APIName
			}
			item = &PricingCapability{
				Code: row.APIName, ModelCode: row.APIName, Name: name,
				Type:        publicOperationModelType(row.OperationCode, row.RouteTemplate),
				Description: row.Description, Visibility: row.Visibility, Sort: row.SortOrder,
				SKUs: []PricingSKU{}, skuIndexes: make(map[uint64]int), displayModelCode: row.ModelCode,
			}
			items[row.APIName] = item
		}
		if item.Description == "" {
			metadata := providerMetadata[repository.NewProviderModelMetadataKey(row.ChannelID, row.VendorModel)]
			if metadata.Description != "" {
				item.Description = metadata.Description
			}
		}
		index, exists := item.skuIndexes[row.SKUID]
		if !exists {
			components, err := displays.For(row.ModelCode, row.SKUCode).PublicRates(schedule.Components)
			if err != nil {
				return nil, err
			}
			index = len(item.SKUs)
			item.skuIndexes[row.SKUID] = index
			item.SKUs = append(item.SKUs, PricingSKU{
				ID: row.SKUID, Code: row.SKUCode, Operation: row.OperationCode,
				Routes: []PricingRoute{}, DeliveryMode: row.DeliveryMode,
				MaxResults: row.MaxResults, IdempotencyMode: row.IdempotencyMode,
				ServiceTiers: tiers,
				Currency:     PricingCurrency{Code: schedule.Currency.Code, Version: schedule.Currency.Version, FractionDigits: schedule.Currency.FractionDigits},
				Components:   components,
				Availability: pricingAvailability(availability, row.SKUID),
			})
		}
		sku := &item.SKUs[index]
		route := PricingRoute{Method: row.DownstreamHTTPMethod, Path: row.DownstreamPath}
		if !containsPricingRoute(sku.Routes, route) {
			sku.Routes = append(sku.Routes, route)
		}
	}

	result := make([]PricingCapability, 0, len(items))
	for _, item := range items {
		codes := make([]string, 0, len(item.SKUs))
		for _, sku := range item.SKUs {
			codes = append(codes, sku.Code)
		}
		item.Description = displays.ModelDescription(item.displayModelCode, codes, item.Description)
		sort.SliceStable(item.SKUs, func(i, j int) bool { return item.SKUs[i].Code < item.SKUs[j].Code })
		item.skuIndexes = nil
		result = append(result, *item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Sort != result[j].Sort {
			return result[i].Sort < result[j].Sort
		}
		return result[i].Code < result[j].Code
	})
	return result, nil
}

func containsPricingRoute(values []PricingRoute, target PricingRoute) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func pricingAvailability(items map[uint64]repository.SKUAvailability, skuID uint64) *PricingAvailability {
	item, ok := items[skuID]
	if !ok {
		return nil
	}
	return &PricingAvailability{
		SuccessRate: item.SuccessRate, Source: item.Source, Samples: item.Samples,
		WindowMinutes: item.WindowMinutes, ObservedAt: item.ObservedAt,
		ChannelStatus: item.ChannelStatus, EstimatedCompletionMS: item.EstimatedCompletionMS, AverageCompletionSeconds: item.AverageCompletionSeconds, Buckets: item.Buckets,
		ChannelObservedAt: availabilityObservedAt(item.ChannelObservedAt),
	}
}

func availabilityObservedAt(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}
