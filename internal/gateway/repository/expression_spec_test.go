package repository

import (
	"testing"

	"github.com/mirainya/Prism/internal/gateway/billing"
)

func TestGenericExpressionDomainsComeFromProductConfig(t *testing.T) {
	ref := adapterVariant{
		Code: "generic", Version: 1, Variant: "default", VendorModel: "video-model",
		ProductConfig: []byte(`{"adapter":{"validation":{"models":{"video-model":{"duration_min":4,"duration_max":29,"resolutions":["480p","720p"]}}}}}`),
	}
	spec, err := genericProductExpressionSpec(ref)
	if err != nil {
		t.Fatal(err)
	}
	expression, err := billing.ParseExpression("seconds * (resolution=='720p' ? 0.5 : 0.3)", spec.Declared)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := expression.ProveUpperBound(spec)
	if err != nil || proof.MaxPrice.String() != "14.5" {
		t.Fatalf("proof=%+v err=%v, want maximum 14.5 from 29 seconds at 720p", proof, err)
	}
}

func TestGenericExpressionDoesNotInventMissingResolutionDomain(t *testing.T) {
	ref := adapterVariant{
		Code: "generic", Version: 1, Variant: "default", VendorModel: "video-model",
		ProductConfig: []byte(`{"adapter":{"validation":{"models":{"video-model":{"duration_min":4,"duration_max":29}}}}}`),
	}
	spec, err := genericProductExpressionSpec(ref)
	if err != nil {
		t.Fatal(err)
	}
	expression, err := billing.ParseExpression("seconds * (resolution=='720p' ? 0.5 : 0.3)", spec.Declared)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := expression.ProveUpperBound(spec); err == nil {
		t.Fatal("an undeclared resolution domain produced a bound")
	}
}

func TestProductResolutionListNarrowsExpressionBound(t *testing.T) {
	ref := adapterVariant{
		Code: "generic", Version: 1, Variant: "default", VendorModel: "video-model",
		ProductConfig: []byte(`{"resolutions":["480p"],"adapter":{"validation":{"models":{"video-model":{"duration_min":4,"duration_max":29,"resolutions":["480p"]}}}}}`),
	}
	spec, err := expressionSpecForVariant(ref)
	if err != nil {
		t.Fatal(err)
	}
	expression, err := billing.ParseExpression("seconds * (resolution=='720p' ? 0.5 : 0.3)", spec.Declared)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := expression.ProveUpperBound(spec)
	if err != nil || proof.MaxPrice.String() != "8.7" {
		t.Fatalf("proof=%+v err=%v, want 8.7 for the sole supported resolution", proof, err)
	}
}

func TestGenericProviderBillingSecondsExpressionIsOptIn(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		response := ""
		if enabled {
			response = `"response":{"billing_seconds_paths":["usage.duration"],"billing_seconds_max":31},`
		}
		ref := adapterVariant{Code: "generic", Version: 1, VendorModel: "wan", ProductConfig: []byte(`{"adapter":{` + response + `"validation":{"models":{"wan":{"duration_min":2,"duration_max":30,"resolutions":["480p","720p","1080p"]}}}}}`)}
		spec, err := genericProductExpressionSpec(ref)
		if err != nil {
			t.Fatal(err)
		}
		expression, err := billing.ParseExpression("billing_seconds * (resolution == '1080p' ? 0.54 : 0.468)", spec.Declared)
		if !enabled {
			if err == nil {
				t.Fatal("provider meter exposed without explicit contract")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		proof, err := expression.ProveUpperBound(spec)
		if err != nil || proof.MaxPrice.String() != "16.74" {
			t.Fatalf("31 second bound: %+v %v", proof, err)
		}
	}
}
