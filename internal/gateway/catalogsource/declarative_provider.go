package catalogsource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DeclarativeCatalogV1      = "declarative_catalog_v1"
	DeclarativeAvailabilityV1 = "declarative_availability_v1"
)

// DeclarativeProvider executes the bounded protocol document stored in the
// database. It is intentionally small: request construction, JSON extraction
// and field mapping are the only supported operations.
type DeclarativeProvider struct{}

func NewDeclarativeProvider() DeclarativeProvider { return DeclarativeProvider{} }

func (DeclarativeProvider) Contracts() []ProviderContract {
	return []ProviderContract{{Code: DeclarativeCatalogV1, Name: "数据库配置协议", MaxRequests: maxProviderRequests}}
}

func (DeclarativeProvider) AvailabilityContracts() []AvailabilityContract {
	return []AvailabilityContract{{Code: DeclarativeAvailabilityV1, Name: "数据库配置可用性协议", MaxRequests: maxProviderRequests}}
}

func (DeclarativeProvider) Discover(ctx context.Context, request DiscoveryProviderRequest) (DiscoveryResult, error) {
	if request.Contract != DeclarativeCatalogV1 || len(request.Definition) == 0 || request.BaseURL == nil || request.Exchange == nil {
		return DiscoveryResult{}, ErrProviderInvalidRequest
	}
	definition, err := ValidateDeclarativeProtocol(request.Definition)
	if err != nil || definition.Kind != "catalog_discovery" {
		return DiscoveryResult{}, ErrProviderInvalidRequest
	}
	lastResponse, responses, err := executeDeclarativeSteps(ctx, definition, request.Secret, request.BaseURL, request.Exchange)
	if err != nil {
		return DiscoveryResult{}, err
	}
	response := lastResponse
	if definition.Models.SourceStep != "" {
		response = responses[definition.Models.SourceStep]
	}
	models, err := mapDeclarativeModels(response, definition.Models, request.ExternalGroup)
	resultResponse := append([]byte(nil), response...)
	clear(lastResponse)
	clearDeclarativeResponses(responses)
	if err != nil {
		return DiscoveryResult{}, err
	}
	for index := range models {
		models[index].SelectedGroupEnabled = request.ExternalGroup == "" || contains(models[index].Groups, request.ExternalGroup)
		models[index].PriceCandidate = models[index].SelectedGroupEnabled && models[index].ModelPrice != nil
	}
	return DiscoveryResult{Models: models, Response: resultResponse}, nil
}

func (DeclarativeProvider) RefreshAvailability(ctx context.Context, request AvailabilityProviderRequest) ([]AvailabilityObservation, error) {
	if request.Contract != DeclarativeAvailabilityV1 || len(request.Definition) == 0 || request.BaseURL == nil || request.Exchange == nil {
		return nil, ErrProviderInvalidRequest
	}
	definition, err := ValidateDeclarativeProtocol(request.Definition)
	if err != nil || definition.Kind != "availability" {
		return nil, ErrProviderInvalidRequest
	}
	lastResponse, responses, err := executeDeclarativeSteps(ctx, definition, request.Secret, request.BaseURL, request.Exchange)
	if err != nil {
		return nil, err
	}
	observations, mapErr := mapDeclarativeAvailability(lastResponse, responses, definition.Availability)
	clear(lastResponse)
	clearDeclarativeResponses(responses)
	return observations, mapErr
}

func executeDeclarativeSteps(ctx context.Context, definition DeclarativeProtocol, secret []byte, base *url.URL, exchange ProviderExchange) ([]byte, map[string][]byte, error) {
	values := declarativeSecretValues(secret)
	var lastResponse []byte
	responses := make(map[string][]byte, len(definition.Steps))
	for _, step := range definition.Steps {
		method := strings.ToUpper(step.Method)
		target, err := declarativeTarget(base, step.Path, step.Query, values)
		if err != nil {
			clearDeclarativeResponses(responses)
			return nil, nil, err
		}
		body, err := expandDeclarativeBody(step.Body, values)
		if err != nil {
			clearDeclarativeResponses(responses)
			return nil, nil, err
		}
		header := make(http.Header, len(step.Headers)+1)
		for name, value := range step.Headers {
			expanded := expandDeclarativeString(value, values)
			if declarativeHasMissingVariable(value, values) || strings.ContainsAny(expanded, "\r\n") {
				clearDeclarativeResponses(responses)
				return nil, nil, ErrProviderInvalidRequest
			}
			header.Set(name, expanded)
		}
		if definition.Auth.Mode == "bearer" {
			name := definition.Auth.HeaderName
			if name == "" {
				name = "Authorization"
			}
			format := definition.Auth.HeaderFormat
			if format == "" {
				format = "Bearer {secret}"
			}
			header.Set(name, strings.ReplaceAll(format, "{secret}", string(secret)))
		} else if definition.Auth.Mode == "json_login" && step.Name != definition.Auth.LoginStep {
			token, ok := values["__auth_token"]
			if !ok {
				clearDeclarativeResponses(responses)
				return nil, nil, errors.New("declarative authentication token is unavailable")
			}
			format := strings.ReplaceAll(definition.Auth.HeaderFormat, "{value}", fmt.Sprint(token))
			header.Set(definition.Auth.HeaderName, format)
		}
		response, err := exchange(ctx, method, target, body, header)
		clear(body)
		if err != nil {
			clearDeclarativeResponses(responses)
			return nil, nil, err
		}
		if len(response) == 0 {
			clearDeclarativeResponses(responses)
			return nil, nil, ErrProviderResponse
		}
		var document any
		if err := decodeDeclarativeJSON(response, &document); err != nil {
			clear(response)
			clearDeclarativeResponses(responses)
			return nil, nil, ErrProviderResponse
		}
		for _, check := range step.Checks {
			if !declarativeCheckMatches(document, check) {
				clear(response)
				clearDeclarativeResponses(responses)
				return nil, nil, ErrProviderResponse
			}
		}
		for name, path := range step.Save {
			value, ok := declarativeLookup(document, path)
			if !ok {
				clear(response)
				clearDeclarativeResponses(responses)
				return nil, nil, fmt.Errorf("declarative saved value %q is missing", name)
			}
			values[name] = value
		}
		if definition.Auth.Mode == "json_login" && step.Name == definition.Auth.LoginStep {
			value, ok := declarativeLookup(document, definition.Auth.TokenPath)
			if !ok || !validDeclarativeToken(value) {
				clear(response)
				clearDeclarativeResponses(responses)
				return nil, nil, errors.New("declarative authentication token is missing")
			}
			values["__auth_token"] = value
		}
		clear(lastResponse)
		lastResponse = response
		responses[step.Name] = append([]byte(nil), response...)
	}
	return lastResponse, responses, nil
}

func declarativeSecretValues(secret []byte) map[string]any {
	values := make(map[string]any)
	var document map[string]any
	if json.Unmarshal(secret, &document) != nil {
		return values
	}
	for name, value := range document {
		values[name] = value
		values["secret."+name] = value
	}
	return values
}

func decodeDeclarativeJSON(value []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return ErrProviderResponse
	}
	return nil
}

func clearDeclarativeResponses(responses map[string][]byte) {
	for name, value := range responses {
		clear(value)
		delete(responses, name)
	}
}

func declarativeTarget(base *url.URL, path string, query map[string]string, values map[string]any) (string, error) {
	target := *base
	target.Path, target.RawPath, target.Fragment = path, "", ""
	params := url.Values{}
	for name, value := range query {
		expanded := expandDeclarativeString(value, values)
		if declarativeHasMissingVariable(value, values) {
			return "", errors.New("missing protocol variable")
		}
		params.Set(name, expanded)
	}
	target.RawQuery = params.Encode()
	return target.String(), nil
}

func expandDeclarativeBody(raw json.RawMessage, values map[string]any) ([]byte, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var document any
	if err := decodeDeclarativeJSON(raw, &document); err != nil {
		return nil, err
	}
	if declarativeHasMissingVariable(document, values) {
		return nil, errors.New("missing protocol variable")
	}
	document = expandDeclarativeValue(document, values)
	return json.Marshal(document)
}

func expandDeclarativeValue(value any, values map[string]any) any {
	switch typed := value.(type) {
	case string:
		matches := declarativeVariablePattern.FindStringSubmatch(typed)
		if len(matches) == 2 && matches[0] == typed {
			if replacement, ok := values[matches[1]]; ok {
				return replacement
			}
		}
		return expandDeclarativeString(typed, values)
	case []any:
		for index := range typed {
			typed[index] = expandDeclarativeValue(typed[index], values)
		}
		return typed
	case map[string]any:
		for key := range typed {
			typed[key] = expandDeclarativeValue(typed[key], values)
		}
		return typed
	default:
		return value
	}
}

var declarativeVariablePattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_.]*)\}`)

func expandDeclarativeString(value string, values map[string]any) string {
	return declarativeVariablePattern.ReplaceAllStringFunc(value, func(placeholder string) string {
		name := placeholder[2 : len(placeholder)-1]
		if replacement, ok := values[name]; ok {
			return fmt.Sprint(replacement)
		}
		return placeholder
	})
}

func declarativeLookup(document any, path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	return declarativeLookupSegments(document, strings.Split(path, "."))
}

func declarativeLookupSegments(document any, segments []string) (any, bool) {
	if len(segments) == 0 {
		return document, true
	}
	if values, ok := document.([]any); ok {
		result := make([]any, 0, len(values))
		for _, value := range values {
			item, found := declarativeLookupSegments(value, segments)
			if !found {
				continue
			}
			if nested, isArray := item.([]any); isArray {
				result = append(result, nested...)
			} else {
				result = append(result, item)
			}
		}
		return result, len(result) != 0
	}
	object, ok := document.(map[string]any)
	if !ok {
		return nil, false
	}
	segment := segments[0]
	array := strings.HasSuffix(segment, "[]")
	segment = strings.TrimSuffix(segment, "[]")
	value, ok := object[segment]
	if !ok {
		return nil, false
	}
	if array {
		if _, ok := value.([]any); !ok {
			return nil, false
		}
	}
	return declarativeLookupSegments(value, segments[1:])
}

func mapDeclarativeModels(response []byte, mapping *DeclarativeModelMapping, selectedGroup string) ([]DiscoveredModel, error) {
	if mapping == nil {
		return nil, ErrProviderResponse
	}
	var document any
	if decodeDeclarativeJSON(response, &document) != nil {
		return nil, ErrProviderResponse
	}
	value, ok := declarativeLookup(document, mapping.ItemsPath)
	rows, valid := value.([]any)
	if !ok || !valid || len(rows) > 10000 {
		return nil, ErrInvalidSnapshot
	}
	models := make([]DiscoveredModel, 0, len(rows))
	seen := make(map[string]bool)
	for _, row := range rows {
		model, ok := row.(map[string]any)
		if !ok {
			return nil, ErrInvalidSnapshot
		}
		code, ok := declarativeRelativeString(model, mapping.ModelPath)
		if !ok || !validModelCode(code) {
			return nil, ErrInvalidSnapshot
		}
		key := code
		if mapping.CaseInsensitiveCodes {
			key = strings.ToLower(key)
		}
		if mapping.FilterSelectedGroup && selectedGroup != "" {
			groups, ok := declarativeRelativeStrings(model, mapping.GroupPath)
			if !ok {
				group, _ := declarativeRelativeString(model, mapping.GroupPath)
				groups = []string{group}
			}
			if !contains(groups, selectedGroup) {
				continue
			}
		}
		if seen[key] {
			return nil, ErrInvalidSnapshot
		}
		seen[key] = true
		item := DiscoveredModel{Code: code}
		item.Description, _ = declarativeRelativeString(model, mapping.DescriptionPath)
		item.Description = normalizeProviderText(item.Description)
		item.Tags, _ = declarativeRelativeString(model, mapping.TagsPath)
		item.Tags = normalizeProviderText(item.Tags)
		item.OwnerBy, _ = declarativeRelativeString(model, mapping.OwnerByPath)
		item.OwnerBy = normalizeProviderText(item.OwnerBy)
		if !validOptionalText(item.Description, 1000) || !validOptionalText(item.Tags, 512) || !validOptionalText(item.OwnerBy, 128) {
			return nil, ErrInvalidSnapshot
		}
		if groups, ok := declarativeRelativeStrings(model, mapping.GroupPath); ok {
			item.Groups = groups
		} else if group, ok := declarativeRelativeString(model, mapping.GroupPath); ok && group != "" {
			item.Groups = []string{group}
		}
		if len(item.Groups) == 0 && !mapping.RequireGroups && selectedGroup != "" {
			item.Groups = []string{selectedGroup}
		}
		var err error
		item.Groups, err = normalizeStrings(item.Groups, 128, nil)
		if err != nil || mapping.RequireGroups && len(item.Groups) == 0 {
			return nil, ErrInvalidSnapshot
		}
		item.EndpointTypes, _ = declarativeRelativeStrings(model, mapping.EndpointTypesPath)
		if mapping.EndpointTypesPath != "" || mapping.RequireEndpoints {
			item.EndpointTypes, err = normalizeStrings(item.EndpointTypes, 64, endpointPattern)
			if err != nil || mapping.RequireEndpoints && len(item.EndpointTypes) == 0 {
				return nil, ErrInvalidSnapshot
			}
		}
		item.ModelPrice, err = declarativeDecimal(model, mapping.PricePath)
		if err != nil {
			return nil, err
		}
		item.ModelRatio, err = declarativeDecimal(model, mapping.ModelRatioPath)
		if err != nil {
			return nil, err
		}
		item.CompletionRatio, err = declarativeDecimal(model, mapping.CompletionRatioPath)
		if err != nil {
			return nil, err
		}
		if value, ok := declarativeLookup(model, mapping.VendorIDPath); mapping.VendorIDPath != "" && ok && value != nil {
			text, ok := declarativeScalarString(value)
			if !ok {
				return nil, ErrInvalidSnapshot
			}
			parsed, e := strconv.ParseUint(text, 10, 64)
			if e != nil {
				return nil, ErrInvalidSnapshot
			}
			item.VendorID = &parsed
		}
		if value, ok := declarativeLookup(model, mapping.QuotaTypePath); mapping.QuotaTypePath != "" && ok && value != nil {
			text, ok := declarativeScalarString(value)
			if !ok {
				return nil, ErrInvalidSnapshot
			}
			parsed, e := strconv.ParseUint(text, 10, 8)
			if e != nil || parsed > 1 {
				return nil, ErrInvalidSnapshot
			}
			quota := uint8(parsed)
			item.ProviderQuotaType = &quota
		}
		if value, ok := declarativeLookup(model, mapping.PricingVersionPath); mapping.PricingVersionPath != "" && ok && value != nil {
			raw, e := json.Marshal(value)
			if e != nil {
				return nil, ErrInvalidSnapshot
			}
			item.PricingVersion, err = canonicalScalar(raw)
			if err != nil {
				return nil, err
			}
		}
		models = append(models, item)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Code < models[j].Code })
	return models, nil
}

func declarativeDecimal(document map[string]any, path string) (*string, error) {
	if path == "" {
		return nil, nil
	}
	value, ok := declarativeLookup(document, path)
	if !ok || value == nil {
		return nil, nil
	}
	text, ok := declarativeScalarString(value)
	if !ok {
		return nil, ErrInvalidSnapshot
	}
	return optionalDecimal(json.Number(text))
}
func validDeclarativeToken(value any) bool {
	switch v := value.(type) {
	case json.Number:
		n, e := strconv.ParseUint(v.String(), 10, 64)
		return e == nil && n > 0
	case string:
		return validText(v, 2048)
	default:
		return false
	}
}
func declarativeCheckMatches(document any, check DeclarativeCheck) bool {
	value, ok := declarativeLookup(document, check.Path)
	if !ok {
		return check.Optional
	}
	var expected any
	if decodeDeclarativeJSON(check.Equals, &expected) != nil {
		return false
	}
	return reflect.DeepEqual(value, expected)
}
func declarativeHasMissingVariable(value any, values map[string]any) bool {
	switch v := value.(type) {
	case string:
		for _, match := range declarativeVariablePattern.FindAllStringSubmatch(v, -1) {
			if _, ok := values[match[1]]; !ok {
				return true
			}
		}
		return false
	case []any:
		for _, child := range v {
			if declarativeHasMissingVariable(child, values) {
				return true
			}
		}
	case map[string]any:
		for _, child := range v {
			if declarativeHasMissingVariable(child, values) {
				return true
			}
		}
	}
	return false
}

func declarativeRelativeString(document map[string]any, path string) (string, bool) {
	if path == "" {
		return "", false
	}
	value, ok := declarativeLookup(document, path)
	if !ok {
		return "", false
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed), true
	case json.Number:
		return typed.String(), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	default:
		return "", false
	}
}

func declarativeRelativeStrings(document map[string]any, path string) ([]string, bool) {
	if path == "" {
		return nil, false
	}
	value, ok := declarativeLookup(document, path)
	if !ok {
		return nil, false
	}
	values, ok := value.([]any)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(values))
	for _, item := range values {
		text, ok := declarativeScalarString(item)
		if !ok || text == "" {
			return nil, false
		}
		result = append(result, text)
	}
	return result, true
}

func declarativeScalarString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed), true
	case json.Number:
		return typed.String(), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	default:
		return "", false
	}
}

func declarativeHasData(document map[string]any, path string) bool {
	value, ok := declarativeLookup(document, path)
	if !ok {
		return false
	}
	var hasData func(any) bool
	hasData = func(value any) bool {
		switch typed := value.(type) {
		case bool:
			return typed
		case []any:
			for _, item := range typed {
				if hasData(item) {
					return true
				}
			}
		}
		return false
	}
	return hasData(value)
}

func declarativeObservedAt(value string, fallback time.Time) time.Time {
	if unixSeconds, err := strconv.ParseInt(value, 10, 64); err == nil && unixSeconds > 0 {
		return time.Unix(unixSeconds, 0).UTC()
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC()
	}
	return fallback
}

// Scoped paths read from the current row, the response root ($.), or its parent
// ($parent.). No expressions or provider-specific field names are interpreted.
func declarativeScope(row, parent, root any) map[string]any {
	scope := make(map[string]any)
	if values, ok := row.(map[string]any); ok {
		for key, value := range values {
			scope[key] = value
		}
	}
	scope["$"] = root
	scope["$parent"] = parent
	return scope
}

func mapDeclarativeAvailability(lastResponse []byte, responses map[string][]byte, mapping *DeclarativeAvailabilityMap) ([]AvailabilityObservation, error) {
	if mapping == nil {
		return nil, ErrProviderResponse
	}
	payloads := [][]byte{lastResponse}
	if len(mapping.SourceSteps) > 0 {
		payloads = nil
		for _, step := range mapping.SourceSteps {
			raw, ok := responses[step]
			if !ok {
				return nil, ErrProviderResponse
			}
			payloads = append(payloads, raw)
		}
	}
	byModel := make(map[string]AvailabilityObservation)
	observed := time.Now().UTC()
	add := func(item AvailabilityObservation) error {
		if item.SuccessRate == "" && item.ChannelStatus == "" && item.EstimatedCompletionMS == nil && len(item.Buckets) == 0 {
			return nil
		}
		if !validModelCode(item.ModelCode) || !validOptionalText(item.Category, 32) || !validOptionalText(item.ChannelStatus, 64) {
			return ErrInvalidSnapshot
		}
		key := strings.ToLower(item.ModelCode)
		current, exists := byModel[key]
		if !exists {
			byModel[key] = item
		} else {
			switch mapping.Deduplicate {
			case "latest":
				if item.ObservedAt.After(current.ObservedAt) {
					byModel[key] = item
				}
			case "worst":
				current.ChannelStatus = worseChannelStatus(current.ChannelStatus, item.ChannelStatus)
				if item.EstimatedCompletionMS != nil && (current.EstimatedCompletionMS == nil || *item.EstimatedCompletionMS > *current.EstimatedCompletionMS) {
					current.EstimatedCompletionMS = item.EstimatedCompletionMS
				}
				if current.Category == "" {
					current.Category = item.Category
				}
				byModel[key] = current
			default:
				return ErrInvalidSnapshot
			}
		}
		if len(byModel) > 10000 {
			return ErrInvalidSnapshot
		}
		return nil
	}
	for _, raw := range payloads {
		var root any
		if decodeDeclarativeJSON(raw, &root) != nil {
			return nil, ErrProviderResponse
		}
		value, ok := declarativeLookup(root, mapping.ItemsPath)
		rows, valid := value.([]any)
		if !ok || !valid || len(rows) > 10000 {
			return nil, ErrInvalidSnapshot
		}
		for _, row := range rows {
			scope := declarativeScope(row, nil, root)
			parent, err := mapDeclarativeObservation(scope, mapping, observed)
			if err != nil {
				return nil, err
			}
			if mapping.Children != nil {
				childValue, found := declarativeLookup(row, mapping.Children.ItemsPath)
				if found && childValue != nil {
					children, valid := childValue.([]any)
					if !valid || len(children) > 10000 {
						return nil, ErrInvalidSnapshot
					}
					for _, child := range children {
						if _, valid := child.(map[string]any); !valid {
							return nil, ErrInvalidSnapshot
						}
						item, err := mapDeclarativeObservation(declarativeScope(child, row, root), mapping.Children, observed)
						if err != nil {
							return nil, err
						}
						if mapping.AggregateChildrenETA && item.EstimatedCompletionMS != nil && (parent.EstimatedCompletionMS == nil || *item.EstimatedCompletionMS > *parent.EstimatedCompletionMS) {
							parent.EstimatedCompletionMS = item.EstimatedCompletionMS
						}
						if err := add(item); err != nil {
							return nil, err
						}
					}
				}
			}
			if err := add(parent); err != nil {
				return nil, err
			}
		}
	}
	result := make([]AvailabilityObservation, 0, len(byModel))
	for _, item := range byModel {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ModelCode < result[j].ModelCode })
	return result, nil
}

func mapDeclarativeObservation(scope map[string]any, mapping *DeclarativeAvailabilityMap, observed time.Time) (AvailabilityObservation, error) {
	code, _ := declarativeRelativeString(scope, mapping.ModelPath)
	category, _ := declarativeRelativeString(scope, mapping.CategoryPath)
	if category == "" {
		category = mapping.DefaultCategory
	}
	status, _ := declarativeRelativeString(scope, mapping.ChannelStatusPath)
	if status == "" {
		status, _ = declarativeRelativeString(scope, mapping.StatusFallbackPath)
	}
	item := AvailabilityObservation{ModelCode: code, Category: category, ChannelStatus: status, ObservedAt: observed}
	if text, ok := declarativeRelativeString(scope, mapping.ObservedAtPath); ok {
		item.ObservedAt = declarativeObservedAt(text, time.Time{})
		if item.ObservedAt.IsZero() {
			return item, ErrInvalidSnapshot
		}
	} else if mapping.ObservedAtPath != "" {
		return item, ErrInvalidSnapshot
	}
	if mapping.BucketsPath != "" {
		value, ok := declarativeLookup(scope, mapping.BucketsPath)
		if !ok {
			return item, ErrInvalidSnapshot
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return item, ErrInvalidSnapshot
		}
		var buckets []struct {
			Label       string  `json:"label"`
			Start       int64   `json:"start"`
			End         int64   `json:"end"`
			HasData     bool    `json:"has_data"`
			HasFailure  bool    `json:"has_failure"`
			SuccessRate float64 `json:"success_rate"`
		}
		if json.Unmarshal(raw, &buckets) != nil || len(buckets) > 288 {
			return item, ErrInvalidSnapshot
		}
		for i, b := range buckets {
			if b.Start <= 0 || b.End <= b.Start || b.SuccessRate < 0 || b.SuccessRate > 100 || (i > 0 && b.Start < buckets[i-1].End) {
				return item, ErrInvalidSnapshot
			}
		}
		item.Buckets, _ = json.Marshal(buckets)
	}
	if mapping.SuccessRatePath != "" {
		text, _ := declarativeRelativeString(scope, mapping.SuccessRatePath)
		rate, err := boundedDecimal(json.Number(text), "100")
		if err != nil {
			return item, err
		}
		if mapping.HasDataPath == "" || declarativeHasData(scope, mapping.HasDataPath) {
			item.SuccessRate = rate
			window := mapping.DefaultWindowMinutes
			if mapping.WindowMinutesPath != "" {
				text, ok := declarativeRelativeString(scope, mapping.WindowMinutesPath)
				if !ok {
					return item, ErrInvalidSnapshot
				}
				parsed, err := strconv.ParseUint(text, 10, 32)
				if err != nil {
					return item, ErrInvalidSnapshot
				}
				window = uint32(parsed)
			}
			if window == 0 || window > 10080 {
				return item, ErrInvalidSnapshot
			}
			item.WindowMinutes = window
			text, _ = declarativeRelativeString(scope, mapping.AverageCompletionSecondsPath)
			completion, err := boundedDecimal(json.Number(text), "86400")
			if err != nil {
				completion = "0"
			}
			item.AverageCompletionSeconds = completion
		}
	}
	if mapping.EstimatedCompletionMSPath != "" && (mapping.EstimatedCompletionMatch == nil || declarativeCheckMatches(scope, *mapping.EstimatedCompletionMatch)) {
		text, _ := declarativeRelativeString(scope, mapping.EstimatedCompletionMSPath)
		ms, err := strconv.ParseUint(text, 10, 64)
		if err == nil && ms > 0 && ms <= 86400000 {
			item.EstimatedCompletionMS = &ms
		}
	}
	return item, nil
}
