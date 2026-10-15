package video

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

var ErrInvalidResolutionConfig = errors.New("video: invalid resolution configuration")

type VideoResolutionOptions struct {
	Values   []string
	Default  string
	Declared bool
}

type resolutionParameter struct {
	Name    string `json:"name"`
	Default any    `json:"default"`
	Options []struct {
		Value          any      `json:"value"`
		AddsResolution []string `json:"adds_resolutions"`
	} `json:"options"`
}

type resolutionServiceTier struct {
	AddsResolution []string       `json:"adds_resolutions"`
	RequestParams  map[string]any `json:"request_params"`
}

type resolutionCatalog struct {
	Resolutions        []string              `json:"resolutions"`
	ResolutionDefault  string                `json:"resolution_default"`
	Parameters         []resolutionParameter `json:"parameters"`
	ServiceTierOptions []struct {
		Value          string   `json:"value"`
		AddsResolution []string `json:"adds_resolutions"`
	} `json:"service_tier_options"`
	Adapter struct {
		Request struct {
			ParamsMode string         `json:"params_mode"`
			FixedBody  map[string]any `json:"fixed_body"`
		} `json:"request"`
		ServiceTiers map[string]resolutionServiceTier `json:"service_tiers"`
		Validation   struct {
			Models map[string]json.RawMessage `json:"models"`
		} `json:"validation"`
	} `json:"adapter"`
}

type modelResolutionCatalog struct {
	Resolutions       []string              `json:"resolutions"`
	ResolutionDefault string                `json:"resolution_default"`
	Parameters        []resolutionParameter `json:"parameters"`
}

func ReadVideoResolutionOptions(raw []byte, vendorModel string) (VideoResolutionOptions, error) {
	catalog, modelCatalog, err := decodeResolutionCatalog(raw, vendorModel)
	if err != nil {
		return VideoResolutionOptions{}, err
	}

	baseValues := append([]string(nil), catalog.Resolutions...)
	if len(baseValues) == 0 {
		baseValues = append(baseValues, modelCatalog.Resolutions...)
	} else if len(modelCatalog.Resolutions) > 0 && !sameResolutionSet(baseValues, modelCatalog.Resolutions) {
		return VideoResolutionOptions{}, fmt.Errorf("%w: product and adapter resolution lists differ", ErrInvalidResolutionConfig)
	}
	values := append([]string(nil), baseValues...)
	values = append(values, allParameterResolutionValues(catalog.Parameters)...)
	values = append(values, allParameterResolutionValues(modelCatalog.Parameters)...)
	for _, tier := range catalog.ServiceTierOptions {
		values = append(values, tier.AddsResolution...)
	}
	for _, tier := range catalog.Adapter.ServiceTiers {
		values = append(values, tier.AddsResolution...)
	}
	for _, parameter := range catalog.Parameters {
		for _, option := range parameter.Options {
			values = append(values, option.AddsResolution...)
		}
	}
	for _, parameter := range modelCatalog.Parameters {
		for _, option := range parameter.Options {
			values = append(values, option.AddsResolution...)
		}
	}
	values, err = normalizeResolutionValues(values)
	if err != nil {
		return VideoResolutionOptions{}, err
	}
	defaultResolution := strings.TrimSpace(catalog.ResolutionDefault)
	if defaultResolution == "" {
		defaultResolution = strings.TrimSpace(modelCatalog.ResolutionDefault)
	}
	if catalogDefault := strings.TrimSpace(catalog.ResolutionDefault); catalogDefault != "" &&
		strings.TrimSpace(modelCatalog.ResolutionDefault) != "" && catalogDefault != strings.TrimSpace(modelCatalog.ResolutionDefault) {
		return VideoResolutionOptions{}, fmt.Errorf("%w: product and adapter defaults differ", ErrInvalidResolutionConfig)
	}
	for _, parameters := range [][]resolutionParameter{catalog.Parameters, modelCatalog.Parameters} {
		for _, parameter := range parameters {
			if strings.TrimSpace(parameter.Name) == "resolution" {
				if parameter.Default != nil {
					if _, ok := parameter.Default.(string); !ok {
						return VideoResolutionOptions{}, fmt.Errorf("%w: resolution default must be a string", ErrInvalidResolutionConfig)
					}
				}
				if value, ok := parameter.Default.(string); ok && strings.TrimSpace(value) != "" {
					if defaultResolution != "" && defaultResolution != strings.TrimSpace(value) {
						return VideoResolutionOptions{}, fmt.Errorf("%w: conflicting resolution defaults", ErrInvalidResolutionConfig)
					}
					defaultResolution = strings.TrimSpace(value)
				}
				for _, option := range parameter.Options {
					if _, ok := option.Value.(string); !ok {
						return VideoResolutionOptions{}, fmt.Errorf("%w: resolution options must be strings", ErrInvalidResolutionConfig)
					}
				}
			}
		}
	}
	declared := len(catalog.Resolutions) > 0 || len(modelCatalog.Resolutions) > 0 || len(values) > 0
	if defaultResolution != "" && !slices.Contains(values, defaultResolution) {
		return VideoResolutionOptions{}, fmt.Errorf("%w: default %q is not supported", ErrInvalidResolutionConfig, defaultResolution)
	}
	return VideoResolutionOptions{Values: values, Default: defaultResolution, Declared: declared}, nil
}

func ResolveVideoResolution(raw []byte, vendorModel, requested, serviceTier string, params map[string]any) (string, error) {
	catalog, modelCatalog, err := decodeResolutionCatalog(raw, vendorModel)
	if err != nil {
		return "", err
	}
	options, err := ReadVideoResolutionOptions(raw, vendorModel)
	if err != nil {
		return "", err
	}
	selectedValues := append([]string(nil), catalog.Resolutions...)
	if len(selectedValues) == 0 {
		selectedValues = append(selectedValues, modelCatalog.Resolutions...)
	}
	effectiveParams, err := ApplyVideoServiceTierParams(raw, vendorModel, serviceTier, params)
	if err != nil {
		return "", err
	}
	selectedValues = append(selectedValues, allParameterResolutionValues(catalog.Parameters)...)
	selectedValues = append(selectedValues, allParameterResolutionValues(modelCatalog.Parameters)...)
	for _, tier := range catalog.ServiceTierOptions {
		if tier.Value == serviceTier {
			selectedValues = append(selectedValues, tier.AddsResolution...)
		}
	}
	if tier, exists := findResolutionServiceTier(catalog.Adapter.ServiceTiers, serviceTier); exists {
		selectedValues = append(selectedValues, tier.AddsResolution...)
	}
	selectedValues = append(selectedValues, selectedParameterResolutions(catalog.Parameters, effectiveParams)...)
	selectedValues = append(selectedValues, selectedParameterResolutions(modelCatalog.Parameters, effectiveParams)...)
	selectedValues, err = normalizeResolutionValues(selectedValues)
	if err != nil {
		return "", err
	}
	requested = strings.TrimSpace(requested)
	if requested != "" {
		if options.Declared && !slices.Contains(selectedValues, requested) {
			return "", fmt.Errorf("resolution %q is not supported", requested)
		}
		return requested, nil
	}
	if !options.Declared {
		return "", nil
	}
	if options.Default != "" && slices.Contains(selectedValues, options.Default) {
		return options.Default, nil
	}
	if len(selectedValues) == 1 {
		return selectedValues[0], nil
	}
	if len(selectedValues) == 0 {
		return "", fmt.Errorf("resolution options are not configured")
	}
	return "", fmt.Errorf("resolution is required; supported values: %s", strings.Join(selectedValues, ", "))
}

func ApplyVideoServiceTierParams(raw []byte, vendorModel, serviceTier string, params map[string]any) (map[string]any, error) {
	catalog, modelCatalog, err := decodeResolutionCatalog(raw, vendorModel)
	if err != nil {
		return nil, err
	}
	parameterNames := map[string]struct{}{"priority": {}}
	for _, parameters := range [][]resolutionParameter{catalog.Parameters, modelCatalog.Parameters} {
		for _, parameter := range parameters {
			name := strings.TrimSpace(parameter.Name)
			if name != "" && name != "resolution" {
				parameterNames[name] = struct{}{}
			}
		}
	}
	effective := make(map[string]any)
	paramsMode := strings.TrimSpace(catalog.Adapter.Request.ParamsMode)
	if paramsMode == "" || paramsMode == "merge_missing" {
		effective = cloneResolutionParams(params)
	}
	if tier, exists := findResolutionServiceTier(catalog.Adapter.ServiceTiers, serviceTier); exists {
		for name, value := range tier.RequestParams {
			effective[name] = value
		}
	}
	for name := range parameterNames {
		if value, exists := catalog.Adapter.Request.FixedBody[name]; exists {
			effective[name] = value
		}
	}
	return effective, nil
}

func findResolutionServiceTier(tiers map[string]resolutionServiceTier, name string) (resolutionServiceTier, bool) {
	name = strings.TrimSpace(name)
	for candidate, tier := range tiers {
		if strings.EqualFold(strings.TrimSpace(candidate), name) {
			return tier, true
		}
	}
	return resolutionServiceTier{}, false
}

func ValidateVideoResolutionConfig(raw []byte, vendorModel string) error {
	_, err := ReadVideoResolutionOptions(raw, vendorModel)
	return err
}

func RequireVideoResolutionConfig(raw []byte, vendorModel string) error {
	options, err := ReadVideoResolutionOptions(raw, vendorModel)
	if err != nil {
		return err
	}
	if !options.Declared || len(options.Values) == 0 {
		return fmt.Errorf("%w: model has no declared resolution options", ErrInvalidResolutionConfig)
	}
	return nil
}

func allParameterResolutionValues(parameters []resolutionParameter) []string {
	var values []string
	for _, parameter := range parameters {
		if strings.TrimSpace(parameter.Name) != "resolution" {
			continue
		}
		for _, option := range parameter.Options {
			if value, ok := option.Value.(string); ok {
				values = append(values, value)
			}
		}
	}
	return values
}

func selectedParameterResolutions(parameters []resolutionParameter, selected map[string]any) []string {
	var values []string
	for _, parameter := range parameters {
		name := strings.TrimSpace(parameter.Name)
		if name == "resolution" {
			continue
		}
		selectedValue, exists := selected[name]
		if !exists {
			continue
		}
		for _, option := range parameter.Options {
			if equalJSONValue(selectedValue, option.Value) {
				values = append(values, option.AddsResolution...)
			}
		}
	}
	return values
}

func equalJSONValue(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func cloneResolutionParams(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for name, value := range source {
		clone[name] = value
	}
	return clone
}

func decodeResolutionCatalog(raw []byte, vendorModel string) (resolutionCatalog, modelResolutionCatalog, error) {
	var catalog resolutionCatalog
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&catalog); err != nil {
		return resolutionCatalog{}, modelResolutionCatalog{}, fmt.Errorf("%w: %v", ErrInvalidResolutionConfig, err)
	}
	var modelCatalog modelResolutionCatalog
	if modelRaw := catalog.Adapter.Validation.Models[strings.TrimSpace(vendorModel)]; len(modelRaw) != 0 {
		modelDecoder := json.NewDecoder(bytes.NewReader(modelRaw))
		modelDecoder.UseNumber()
		if err := modelDecoder.Decode(&modelCatalog); err != nil {
			return resolutionCatalog{}, modelResolutionCatalog{}, fmt.Errorf("%w: invalid model resolution rule", ErrInvalidResolutionConfig)
		}
	}
	return catalog, modelCatalog, nil
}

func normalizeResolutionValues(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || strings.Contains(value, "://") {
			return nil, ErrInvalidResolutionConfig
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func sameResolutionSet(left, right []string) bool {
	left, leftErr := normalizeResolutionValues(left)
	right, rightErr := normalizeResolutionValues(right)
	if leftErr != nil || rightErr != nil || len(left) != len(right) {
		return false
	}
	for _, value := range left {
		if !slices.Contains(right, value) {
			return false
		}
	}
	return true
}
