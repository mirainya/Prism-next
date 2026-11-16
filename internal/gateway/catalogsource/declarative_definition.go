package catalogsource

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

// DeclarativeProtocol is the persisted, provider-neutral description of a
// catalog or availability protocol. It describes data movement only; it does
// not contain credentials and it cannot execute arbitrary code.
type DeclarativeProtocol struct {
	SchemaVersion uint32                      `json:"schema_version"`
	Kind          string                      `json:"kind"`
	Auth          DeclarativeAuth             `json:"auth"`
	Steps         []DeclarativeStep           `json:"steps"`
	Models        *DeclarativeModelMapping    `json:"models,omitempty"`
	Availability  *DeclarativeAvailabilityMap `json:"availability,omitempty"`
}

type DeclarativeAuth struct {
	Mode         string `json:"mode"`
	LoginStep    string `json:"login_step,omitempty"`
	TokenPath    string `json:"token_path,omitempty"`
	HeaderName   string `json:"header_name,omitempty"`
	HeaderFormat string `json:"header_format,omitempty"`
}

type DeclarativeStep struct {
	Name    string             `json:"name"`
	Method  string             `json:"method"`
	Path    string             `json:"path"`
	Query   map[string]string  `json:"query,omitempty"`
	Headers map[string]string  `json:"headers,omitempty"`
	Body    json.RawMessage    `json:"body,omitempty"`
	Save    map[string]string  `json:"save,omitempty"`
	Checks  []DeclarativeCheck `json:"checks,omitempty"`
}

type DeclarativeCheck struct {
	Path     string          `json:"path"`
	Equals   json.RawMessage `json:"equals"`
	Optional bool            `json:"optional,omitempty"`
}

type DeclarativeModelMapping struct {
	SourceStep           string `json:"source_step,omitempty"`
	ItemsPath            string `json:"items_path"`
	ModelPath            string `json:"model_path"`
	DescriptionPath      string `json:"description_path,omitempty"`
	GroupPath            string `json:"group_path,omitempty"`
	TagsPath             string `json:"tags_path,omitempty"`
	PricePath            string `json:"price_path,omitempty"`
	EndpointTypesPath    string `json:"endpoint_types_path,omitempty"`
	SelectedGroup        string `json:"selected_group,omitempty"`
	VendorIDPath         string `json:"vendor_id_path,omitempty"`
	QuotaTypePath        string `json:"quota_type_path,omitempty"`
	ModelRatioPath       string `json:"model_ratio_path,omitempty"`
	CompletionRatioPath  string `json:"completion_ratio_path,omitempty"`
	OwnerByPath          string `json:"owner_by_path,omitempty"`
	PricingVersionPath   string `json:"pricing_version_path,omitempty"`
	RequireGroups        bool   `json:"require_groups,omitempty"`
	RequireEndpoints     bool   `json:"require_endpoints,omitempty"`
	CaseInsensitiveCodes bool   `json:"case_insensitive_codes,omitempty"`
	FilterSelectedGroup  bool   `json:"filter_selected_group,omitempty"`
}

type DeclarativeAvailabilityMap struct {
	SourceSteps                  []string                    `json:"source_steps,omitempty"`
	ItemsPath                    string                      `json:"items_path"`
	ModelPath                    string                      `json:"model_path"`
	CategoryPath                 string                      `json:"category_path,omitempty"`
	DefaultCategory              string                      `json:"default_category,omitempty"`
	HasDataPath                  string                      `json:"has_data_path,omitempty"`
	SuccessRatePath              string                      `json:"success_rate_path,omitempty"`
	BucketsPath                  string                      `json:"buckets_path,omitempty"`
	AverageCompletionSecondsPath string                      `json:"average_completion_seconds_path,omitempty"`
	ChannelStatusPath            string                      `json:"channel_status_path,omitempty"`
	EstimatedCompletionMSPath    string                      `json:"estimated_completion_ms_path,omitempty"`
	WindowMinutesPath            string                      `json:"window_minutes_path,omitempty"`
	DefaultWindowMinutes         uint32                      `json:"default_window_minutes,omitempty"`
	ObservedAtPath               string                      `json:"observed_at_path,omitempty"`
	Children                     *DeclarativeAvailabilityMap `json:"children,omitempty"`
	AggregateChildrenETA         bool                        `json:"aggregate_children_eta,omitempty"`
	StatusFallbackPath           string                      `json:"status_fallback_path,omitempty"`
	EstimatedCompletionMatch     *DeclarativeCheck           `json:"estimated_completion_match,omitempty"`
	Deduplicate                  string                      `json:"deduplicate,omitempty"`
}

var declarativePathPattern = regexp.MustCompile(`^[A-Za-z0-9_$.-]+(?:\[\])?(?:\.[A-Za-z0-9_$.-]+(?:\[\])?)*$`)

func ValidateDeclarativeProtocol(raw []byte) (DeclarativeProtocol, error) {
	var definition DeclarativeProtocol
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&definition); err != nil {
		return DeclarativeProtocol{}, fmt.Errorf("invalid declarative protocol JSON: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return DeclarativeProtocol{}, errors.New("protocol must contain exactly one JSON document")
	}
	if definition.SchemaVersion != 1 || definition.Kind != "catalog_discovery" && definition.Kind != "availability" {
		return DeclarativeProtocol{}, errors.New("unsupported declarative protocol version or kind")
	}
	if len(definition.Steps) == 0 || len(definition.Steps) > 16 {
		return DeclarativeProtocol{}, errors.New("declarative protocol must contain 1 to 16 steps")
	}
	if err := validateDeclarativeAuth(definition.Auth, definition.Steps); err != nil {
		return DeclarativeProtocol{}, err
	}
	seen := make(map[string]struct{}, len(definition.Steps))
	for _, step := range definition.Steps {
		if err := validateDeclarativeStep(step); err != nil {
			return DeclarativeProtocol{}, err
		}
		if _, exists := seen[step.Name]; exists {
			return DeclarativeProtocol{}, fmt.Errorf("duplicate declarative step %q", step.Name)
		}
		seen[step.Name] = struct{}{}
	}
	if definition.Kind == "catalog_discovery" {
		if definition.Availability != nil {
			return DeclarativeProtocol{}, errors.New("catalog protocol cannot define availability")
		}
		if definition.Models == nil {
			return DeclarativeProtocol{}, errors.New("catalog protocol requires models mapping")
		}
		if err := validateDeclarativeModelMapping(*definition.Models); err != nil {
			return DeclarativeProtocol{}, err
		}
		if definition.Models.SourceStep != "" {
			if _, ok := seen[definition.Models.SourceStep]; !ok {
				return DeclarativeProtocol{}, fmt.Errorf("models source step %q does not exist", definition.Models.SourceStep)
			}
		}
	} else {
		if definition.Models != nil {
			return DeclarativeProtocol{}, errors.New("availability protocol cannot define models")
		}
		if definition.Availability == nil {
			return DeclarativeProtocol{}, errors.New("availability protocol requires availability mapping")
		}
		if err := validateDeclarativeAvailability(*definition.Availability); err != nil {
			return DeclarativeProtocol{}, err
		}
		if definition.Availability.DefaultWindowMinutes > 10080 {
			return DeclarativeProtocol{}, errors.New("availability default window is too large")
		}
		if len(definition.Availability.SourceSteps) > len(definition.Steps) {
			return DeclarativeProtocol{}, errors.New("availability source_steps contains too many steps")
		}
		for _, step := range definition.Availability.SourceSteps {
			if _, ok := seen[step]; !ok {
				return DeclarativeProtocol{}, fmt.Errorf("availability source step %q does not exist", step)
			}
		}
	}
	return definition, nil
}

func validateDeclarativeAuth(auth DeclarativeAuth, steps []DeclarativeStep) error {
	if auth.HeaderName != "" && !validDeclarativeHeader(auth.HeaderName) || strings.ContainsAny(auth.HeaderFormat, "\r\n") {
		return errors.New("invalid authentication header")
	}
	switch auth.Mode {
	case "none":
		if auth.LoginStep != "" || auth.TokenPath != "" || auth.HeaderName != "" || auth.HeaderFormat != "" {
			return errors.New("none authentication cannot define login or token fields")
		}
	case "bearer":
		if auth.HeaderName == "" {
			return errors.New("bearer authentication requires header_name")
		}
	case "json_login":
		if auth.LoginStep == "" || auth.TokenPath == "" || auth.HeaderName == "" || auth.HeaderFormat == "" {
			return errors.New("json_login authentication requires login and token fields")
		}
		found := false
		for _, step := range steps {
			if step.Name == auth.LoginStep {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("authentication login step %q does not exist", auth.LoginStep)
		}
		if steps[0].Name != auth.LoginStep {
			return errors.New("login must be the first step")
		}
		if err := validateDeclarativePath(auth.TokenPath); err != nil {
			return fmt.Errorf("invalid authentication token path: %w", err)
		}
	default:
		return fmt.Errorf("unsupported declarative authentication mode %q", auth.Mode)
	}
	return nil
}

func validateDeclarativeStep(step DeclarativeStep) error {
	if !validDeclarativeName(step.Name) {
		return fmt.Errorf("invalid declarative step name %q", step.Name)
	}
	switch strings.ToUpper(step.Method) {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return fmt.Errorf("unsupported declarative method %q", step.Method)
	}
	if !strings.HasPrefix(step.Path, "/") || strings.HasPrefix(step.Path, "//") || strings.ContainsAny(step.Path, "\r\n?#") || len(step.Path) > 512 {
		return fmt.Errorf("invalid declarative path %q", step.Path)
	}
	for name, value := range step.Headers {
		if !validDeclarativeHeader(name) || strings.ContainsAny(value, "\r\n") {
			return errors.New("invalid step header")
		}
	}
	if len(step.Checks) > 16 {
		return errors.New("too many response checks")
	}
	for _, check := range step.Checks {
		if err := validateDeclarativeCheck(check); err != nil {
			return err
		}
	}
	if len(step.Query)+len(step.Headers) > 64 || len(step.Save) > 32 {
		return fmt.Errorf("declarative step %q has too many fields", step.Name)
	}
	for name, path := range step.Save {
		if !validDeclarativeName(name) {
			return fmt.Errorf("invalid saved value name %q", name)
		}
		if err := validateDeclarativePath(path); err != nil {
			return fmt.Errorf("invalid saved value path: %w", err)
		}
	}
	return nil
}

func validateDeclarativeModelMapping(mapping DeclarativeModelMapping) error {
	for name, path := range map[string]string{
		"items_path": mapping.ItemsPath, "model_path": mapping.ModelPath,
		"description_path": mapping.DescriptionPath, "group_path": mapping.GroupPath,
		"tags_path": mapping.TagsPath, "price_path": mapping.PricePath,
		"endpoint_types_path": mapping.EndpointTypesPath,
		"vendor_id_path":      mapping.VendorIDPath, "quota_type_path": mapping.QuotaTypePath, "model_ratio_path": mapping.ModelRatioPath,
		"completion_ratio_path": mapping.CompletionRatioPath, "owner_by_path": mapping.OwnerByPath, "pricing_version_path": mapping.PricingVersionPath,
	} {
		if path == "" && (name == "items_path" || name == "model_path") {
			return fmt.Errorf("models mapping requires %s", name)
		}
		if path != "" {
			if err := validateDeclarativePath(path); err != nil {
				return fmt.Errorf("invalid models %s: %w", name, err)
			}
		}
	}
	return nil
}

func validateDeclarativeAvailability(mapping DeclarativeAvailabilityMap) error {
	if mapping.Deduplicate != "" && mapping.Deduplicate != "latest" && mapping.Deduplicate != "worst" {
		return errors.New("invalid availability deduplication")
	}
	if mapping.Children != nil {
		if mapping.Children.Children != nil || len(mapping.Children.SourceSteps) != 0 {
			return errors.New("availability children cannot nest or select steps")
		}
		if err := validateDeclarativeAvailability(*mapping.Children); err != nil {
			return err
		}
	}
	if mapping.AggregateChildrenETA && mapping.Children == nil {
		return errors.New("ETA aggregation requires children")
	}
	if mapping.DefaultWindowMinutes > 10080 {
		return errors.New("availability window too large")
	}
	if mapping.EstimatedCompletionMatch != nil {
		if err := validateDeclarativeCheck(*mapping.EstimatedCompletionMatch); err != nil {
			return err
		}
	}
	for name, path := range map[string]string{
		"items_path": mapping.ItemsPath, "model_path": mapping.ModelPath,
		"category_path": mapping.CategoryPath, "has_data_path": mapping.HasDataPath, "success_rate_path": mapping.SuccessRatePath,
		"average_completion_seconds_path": mapping.AverageCompletionSecondsPath,
		"buckets_path":                    mapping.BucketsPath,
		"channel_status_path":             mapping.ChannelStatusPath,
		"estimated_completion_ms_path":    mapping.EstimatedCompletionMSPath,
		"window_minutes_path":             mapping.WindowMinutesPath, "observed_at_path": mapping.ObservedAtPath,
		"status_fallback_path": mapping.StatusFallbackPath,
	} {
		if path == "" && (name == "items_path" || name == "model_path") {
			return fmt.Errorf("availability mapping requires %s", name)
		}
		if path != "" {
			if err := validateDeclarativePath(path); err != nil {
				return fmt.Errorf("invalid availability %s: %w", name, err)
			}
		}
	}
	return nil
}

func validDeclarativeHeader(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
			return false
		}
	}
	return true
}

func validateDeclarativeCheck(check DeclarativeCheck) error {
	if err := validateDeclarativePath(check.Path); err != nil {
		return err
	}
	var value any
	if err := decodeDeclarativeJSON(check.Equals, &value); err != nil {
		return errors.New("invalid response check value")
	}
	switch value.(type) {
	case string, json.Number, bool, nil:
		return nil
	default:
		return errors.New("response check requires scalar value")
	}
}

func validateDeclarativePath(value string) error {
	if value == "" || len(value) > 256 || !declarativePathPattern.MatchString(value) {
		return fmt.Errorf("invalid JSON path %q", value)
	}
	return nil
}

func validDeclarativeName(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for index, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || index > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}
