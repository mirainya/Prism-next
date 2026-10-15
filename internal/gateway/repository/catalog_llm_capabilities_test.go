package repository

import (
	"slices"
	"testing"
)

func TestOnboardLLMCapabilitiesAddDeclaredFeaturesWithoutChangingVendorModel(t *testing.T) {
	in := validCatalogModelOnboardInput()
	in.SKU.CapabilityTags = []string{"llm", "tools"}
	in.Product.CapabilityConstraints = []byte(`{"llm":{"supports_tools":true,"supports_streaming":true,"supports_vision":false}}`)
	vendor := in.Product.VendorModel
	if err := in.Normalize(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(in.SKU.CapabilityTags, []string{"llm", "stream", "tools"}) || in.Product.VendorModel != vendor {
		t.Fatalf("unexpected metadata normalization: %#v", in)
	}
	in.SKU.CapabilityTags = []string{"video"}
	if err := in.Normalize(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(in.SKU.CapabilityTags, []string{"video"}) {
		t.Fatalf("LLM features added to video: %#v", in.SKU.CapabilityTags)
	}
}

func TestProductEditsRejectMalformedLLMMetadata(t *testing.T) {
	for _, raw := range []string{`{"llm":[]}`, `{"llm":{"supports_audio":"yes"}}`, `{"llm":{"context_window":9007199254740992}}`} {
		in := ProductChange{catalogChangeGuard: validChangeGuard(), ProductCode: "model-primary", VendorModel: "model-a", CapabilityConstraints: []byte(raw)}
		if err := in.Normalize(); err != nil {
			t.Fatal(err)
		}
		if err := in.Validate(); err == nil {
			t.Fatalf("product edit accepted %s", raw)
		}
		product := validCatalogModelOnboardInput().Product
		product.ExpectedVersion = 1
		product.CapabilityConstraints = []byte(raw)
		if err := product.Validate(); err == nil {
			t.Fatalf("product create accepted %s", raw)
		}
	}
}
