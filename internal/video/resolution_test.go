package video

import (
	"encoding/json"
	"testing"
)

func TestResolveVideoResolutionInfersSingleOption(t *testing.T) {
	got, err := ResolveVideoResolution([]byte(`{"resolutions":["480p"]}`), "vendor", "", "standard", nil)
	if err != nil || got != "480p" {
		t.Fatalf("resolution=%q err=%v, want 480p", got, err)
	}
}

func TestResolveVideoResolutionRequiresChoiceForMultipleOptions(t *testing.T) {
	raw := []byte(`{"resolutions":["480p","720p"]}`)
	if _, err := ResolveVideoResolution(raw, "vendor", "", "standard", nil); err == nil {
		t.Fatal("missing resolution was accepted")
	}
	got, err := ResolveVideoResolution(raw, "vendor", "720p", "standard", nil)
	if err != nil || got != "720p" {
		t.Fatalf("resolution=%q err=%v, want 720p", got, err)
	}
}

func TestResolveVideoResolutionUsesConfiguredDefault(t *testing.T) {
	raw := []byte(`{"resolutions":["480p","720p"],"resolution_default":"720p"}`)
	got, err := ResolveVideoResolution(raw, "vendor", "", "standard", nil)
	if err != nil || got != "720p" {
		t.Fatalf("resolution=%q err=%v, want 720p", got, err)
	}
}

func TestResolveVideoResolutionAppliesTierAndParameterAdditions(t *testing.T) {
	raw := []byte(`{"resolutions":["480p"],"service_tier_options":[{"value":"priority","adds_resolutions":["720p"]}],"parameters":[{"name":"quality","options":[{"value":"high","adds_resolutions":["1080p"]}]}]}`)
	if _, err := ResolveVideoResolution(raw, "vendor", "1080p", "standard", map[string]any{"quality": "high"}); err != nil {
		t.Fatalf("selected parameter resolution rejected: %v", err)
	}
	if _, err := ResolveVideoResolution(raw, "vendor", "720p", "standard", nil); err == nil {
		t.Fatal("inactive tier resolution was accepted")
	}
	if _, err := ResolveVideoResolution(raw, "vendor", "", "priority", nil); err == nil {
		t.Fatal("unselected resolution with tier additions was accepted")
	}
}

func TestResolveVideoResolutionUsesServiceTierRequestParameterDefaults(t *testing.T) {
	raw := []byte(`{"resolutions":["480p"],"parameters":[{"name":"quality","options":[{"value":"high","adds_resolutions":["720p"]}]}],"adapter":{"service_tiers":{"priority":{"request_params":{"quality":"high"}}}}}`)
	got, err := ResolveVideoResolution(raw, "vendor", "720p", "priority", map[string]any{"quality": "low"})
	if err != nil || got != "720p" {
		t.Fatalf("resolution=%q err=%v, want 720p", got, err)
	}
}

func TestApplyVideoServiceTierParamsMatchesAdapterPrecedence(t *testing.T) {
	raw := []byte(`{"parameters":[{"name":"quality","options":[{"value":"high"}]}],"adapter":{"request":{"params_mode":"merge_missing","fixed_body":{"priority":5,"quality":"high","model":"fixed-model","stream":false}},"service_tiers":{"priority":{"request_params":{"priority":4,"fixed":true}}}}}`)
	got, err := ApplyVideoServiceTierParams(raw, "vendor", "priority", map[string]any{"priority": 2, "quality": "low", "caller": "value"})
	if err != nil {
		t.Fatal(err)
	}
	if got["priority"] != json.Number("5") || got["quality"] != "high" || got["fixed"] != true || got["caller"] != "value" {
		t.Fatalf("effective params = %#v", got)
	}
	if _, exists := got["model"]; exists {
		t.Fatal("unrelated fixed model field leaked into params")
	}
	if _, exists := got["stream"]; exists {
		t.Fatal("unrelated fixed stream field leaked into params")
	}
}

func TestApplyVideoServiceTierParamsUsesRuntimeDefaultModeAndCase(t *testing.T) {
	raw := []byte(`{"adapter":{"service_tiers":{"PRIORITY":{"request_params":{"priority":4}}}}}`)
	got, err := ApplyVideoServiceTierParams(raw, "vendor", "priority", map[string]any{"priority": 2, "caller": "value"})
	if err != nil {
		t.Fatal(err)
	}
	if got["priority"] != json.Number("4") || got["caller"] != "value" {
		t.Fatalf("effective params = %#v", got)
	}
}

func TestApplyVideoServiceTierParamsOverridesCallerParameter(t *testing.T) {
	raw := []byte(`{"parameters":[{"name":"quality","options":[{"value":"low"},{"value":"high"}]}],"adapter":{"service_tiers":{"priority":{"request_params":{"quality":"high"}}}}}`)
	got, err := ApplyVideoServiceTierParams(raw, "vendor", "priority", map[string]any{"quality": "low"})
	if err != nil {
		t.Fatal(err)
	}
	if got["quality"] != "high" {
		t.Fatalf("quality = %#v, want tier default high", got["quality"])
	}
}

func TestReadVideoResolutionOptionsTrimsMatchingDefaults(t *testing.T) {
	raw := []byte(`{"resolutions":["480p"],"resolution_default":" 480p ","adapter":{"validation":{"models":{"vendor":{"resolutions":["480p"],"resolution_default":"480p"}}}}}`)
	options, err := ReadVideoResolutionOptions(raw, "vendor")
	if err != nil || options.Default != "480p" {
		t.Fatalf("options=%+v err=%v, want normalized 480p default", options, err)
	}
}

func TestReadVideoResolutionOptionsUsesGenericModelValidation(t *testing.T) {
	raw := []byte(`{"adapter":{"validation":{"models":{"vendor":{"resolutions":["768p"],"resolution_default":"768p"}}}}}`)
	options, err := ReadVideoResolutionOptions(raw, "vendor")
	if err != nil || !options.Declared || len(options.Values) != 1 || options.Values[0] != "768p" || options.Default != "768p" {
		t.Fatalf("options=%+v err=%v", options, err)
	}
}

func TestReadVideoResolutionOptionsRejectsConflictingDeclarations(t *testing.T) {
	raw := []byte(`{"resolutions":["480p"],"adapter":{"validation":{"models":{"vendor":{"resolutions":["720p"]}}}}}`)
	if _, err := ReadVideoResolutionOptions(raw, "vendor"); err == nil {
		t.Fatal("conflicting resolution declarations were accepted")
	}
}
