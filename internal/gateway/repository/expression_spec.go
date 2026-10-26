package repository

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mirainya/Prism/internal/gateway/billing"
	"github.com/mirainya/Prism/internal/video"
)

func expressionSpecForVariant(ref adapterVariant) (billing.ExpressionSpec, error) {
	source := currentExpressionSpecSource()
	var spec billing.ExpressionSpec
	var err error
	if ref.Code == "generic" {
		spec, err = genericProductExpressionSpec(ref)
	} else {
		if source == nil {
			return billing.ExpressionSpec{}, ErrExpressionSpecUnavailable
		}
		spec, err = source.ExpressionSpec(ref.Code, ref.Version, ref.Variant)
	}
	if err != nil {
		return billing.ExpressionSpec{}, err
	}
	resolutionOptions, err := video.ReadVideoResolutionOptions(ref.ProductConfig, ref.VendorModel)
	if err != nil {
		return billing.ExpressionSpec{}, fmt.Errorf("%w: invalid resolution domain: %v", ErrExpressionSpecUnavailable, err)
	}
	if resolutionOptions.Declared && len(resolutionOptions.Values) != 0 {
		values := make([]billing.ExprValue, 0, len(resolutionOptions.Values))
		for _, value := range resolutionOptions.Values {
			values = append(values, billing.ExprString(value))
		}
		domain, domainErr := billing.EnumDomain(values...)
		if domainErr != nil {
			return billing.ExpressionSpec{}, domainErr
		}
		if spec.Domains == nil {
			spec.Domains = make(map[string]billing.VarDomain)
		}
		spec.Domains["resolution"] = domain
	}
	return spec, nil
}

func genericProductExpressionSpec(ref adapterVariant) (billing.ExpressionSpec, error) {
	if ref.VendorModel == "" || len(ref.ProductConfig) == 0 {
		return billing.ExpressionSpec{}, fmt.Errorf("%w: generic product facts are missing", ErrExpressionSpecUnavailable)
	}
	var product struct {
		Adapter struct {
			Response struct {
				BillingSecondsPaths []string `json:"billing_seconds_paths"`
				BillingSecondsMax   int      `json:"billing_seconds_max"`
			} `json:"response"`
			Validation struct {
				Models map[string]json.RawMessage `json:"models"`
			} `json:"validation"`
		} `json:"adapter"`
	}
	decoder := json.NewDecoder(bytes.NewReader(ref.ProductConfig))
	decoder.UseNumber()
	if err := decoder.Decode(&product); err != nil {
		return billing.ExpressionSpec{}, fmt.Errorf("%w: invalid generic product facts", ErrExpressionSpecUnavailable)
	}
	rawRule, ok := product.Adapter.Validation.Models[ref.VendorModel]
	if !ok {
		return billing.ExpressionSpec{}, fmt.Errorf("%w: generic model %q has no validation rule", ErrExpressionSpecUnavailable, ref.VendorModel)
	}
	var rule struct {
		DurationOptions               []int    `json:"duration_options"`
		DurationMin                   int      `json:"duration_min"`
		DurationMax                   int      `json:"duration_max"`
		DurationMaxWithVideoReference int      `json:"duration_max_with_video_reference"`
		TaskModes                     []string `json:"task_modes"`
		AllowGeneratedAudio           *bool    `json:"allow_generated_audio"`
		MaxVideos                     *int     `json:"max_videos"`
		Parameters                    []struct {
			Name    string       `json:"name"`
			Type    string       `json:"type"`
			Min     *json.Number `json:"min"`
			Max     *json.Number `json:"max"`
			Options []struct {
				Value any `json:"value"`
			} `json:"options"`
		} `json:"parameters"`
	}
	ruleDecoder := json.NewDecoder(bytes.NewReader(rawRule))
	ruleDecoder.UseNumber()
	if err := ruleDecoder.Decode(&rule); err != nil {
		return billing.ExpressionSpec{}, fmt.Errorf("%w: invalid generic validation rule", ErrExpressionSpecUnavailable)
	}
	domains := map[string]billing.VarDomain{
		"success":       billing.BoolDomain(),
		"duration_ms":   mustIntDomain("0", "3600000"),
		"attempt_count": mustIntDomain("1", "3"),
	}

	durationMin, durationMax := rule.DurationMin, rule.DurationMax
	if len(rule.DurationOptions) > 0 {
		for _, value := range rule.DurationOptions {
			if durationMin == 0 || value < durationMin {
				durationMin = value
			}
			if value > durationMax {
				durationMax = value
			}
		}
	}
	if rule.DurationMaxWithVideoReference > durationMax {
		durationMax = rule.DurationMaxWithVideoReference
	}
	if durationMin > 0 && durationMax >= durationMin {
		domains["seconds"] = mustIntDomain(strconv.Itoa(durationMin), strconv.Itoa(durationMax))
	}

	if rule.AllowGeneratedAudio != nil && !*rule.AllowGeneratedAudio {
		domains["has_audio"] = billing.ConstDomain(billing.ExprNumber(billing.Zero()))
	} else {
		domains["has_audio"] = billing.BoolDomain()
	}
	if rule.MaxVideos != nil && *rule.MaxVideos == 0 {
		domains["has_video_ref"] = billing.ConstDomain(billing.ExprNumber(billing.Zero()))
	} else {
		domains["has_video_ref"] = billing.BoolDomain()
	}
	if len(rule.TaskModes) > 0 {
		values := make([]billing.ExprValue, 0, len(rule.TaskModes))
		for _, value := range uniqueNonEmpty(rule.TaskModes) {
			values = append(values, billing.ExprString(value))
		}
		domain, err := billing.EnumDomain(values...)
		if err != nil {
			return billing.ExpressionSpec{}, err
		}
		domains["task_mode"] = domain
	}
	for _, parameter := range rule.Parameters {
		if parameter.Name != "priority" {
			continue
		}
		if parameter.Type == "integer" && parameter.Min != nil && parameter.Max != nil {
			min, minErr := strconv.Atoi(parameter.Min.String())
			max, maxErr := strconv.Atoi(parameter.Max.String())
			if minErr == nil && maxErr == nil && min <= max {
				domains["priority"] = mustIntDomain(strconv.Itoa(min), strconv.Itoa(max))
			}
		} else if parameter.Type == "select" {
			values := make([]billing.ExprValue, 0, len(parameter.Options))
			for _, option := range parameter.Options {
				value, ok := genericNumberValue(option.Value)
				if !ok {
					values = nil
					break
				}
				values = append(values, billing.ExprNumber(value))
			}
			if len(values) > 0 {
				domain, err := billing.EnumDomain(values...)
				if err != nil {
					return billing.ExpressionSpec{}, err
				}
				domains["priority"] = domain
			}
		}
		break
	}
	// Keep the DSL vocabulary stable even when this product omits a usable
	// domain. Parsing can then succeed while publication-time bound proof still
	// refuses formulas that depend on facts the product does not define.
	declared := map[string]bool{
		"success": true, "has_audio": true, "has_video_ref": true,
		"duration_ms": true, "attempt_count": true,
		"seconds": true, "resolution": true, "task_mode": true, "priority": true,
	}
	// Only products that explicitly define the provider meter expose it to pricing.
	if response := product.Adapter.Response; len(response.BillingSecondsPaths) > 0 && response.BillingSecondsMax > 0 {
		declared["billing_seconds"] = true
		domains["billing_seconds"] = mustIntDomain("1", strconv.Itoa(response.BillingSecondsMax))
	}
	resolutionOptions, err := video.ReadVideoResolutionOptions(ref.ProductConfig, ref.VendorModel)
	if err != nil {
		return billing.ExpressionSpec{}, fmt.Errorf("%w: invalid resolution domain: %v", ErrExpressionSpecUnavailable, err)
	}
	if resolutionOptions.Declared && len(resolutionOptions.Values) > 0 {
		values := make([]billing.ExprValue, 0, len(resolutionOptions.Values))
		for _, value := range resolutionOptions.Values {
			values = append(values, billing.ExprString(value))
		}
		domain, err := billing.EnumDomain(values...)
		if err != nil {
			return billing.ExpressionSpec{}, err
		}
		domains["resolution"] = domain
	}
	digestInput := fmt.Sprintf("generic@%d/%s\x00%s\x00%s", ref.Version, ref.Variant, ref.VendorModel, ref.ProductConfig)
	digest := sha256.Sum256([]byte(digestInput))
	return billing.ExpressionSpec{Digest: hex.EncodeToString(digest[:]), Declared: declared, Domains: domains}, nil
}

func mustIntDomain(min, max string) billing.VarDomain {
	domain, err := billing.IntDomain(min, max)
	if err != nil {
		panic(err)
	}
	return domain
}

func genericNumberValue(value any) (billing.Amount, bool) {
	var text string
	switch value := value.(type) {
	case json.Number:
		text = value.String()
	case string:
		text = strings.TrimSpace(value)
	default:
		return billing.Amount{}, false
	}
	amount, err := billing.ParseAmount(text, 18, true)
	return amount, err == nil && amount.Scale() == 0
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
