package catalogsource

import (
	"context"
	"net/url"
	"testing"

	"github.com/mirainya/Prism/internal/gateway/adapter"
)

type testCatalogProvider struct {
	contracts []ProviderContract
}

func mustCatalogURL(t *testing.T, value string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func (p testCatalogProvider) Contracts() []ProviderContract { return p.contracts }

func (testCatalogProvider) Discover(context.Context, DiscoveryProviderRequest) (DiscoveryResult, error) {
	return DiscoveryResult{Response: []byte(`{"ok":true}`)}, nil
}

type testAvailabilityProvider struct {
	testCatalogProvider
	availability []AvailabilityContract
}

func (p testAvailabilityProvider) AvailabilityContracts() []AvailabilityContract {
	return p.availability
}

func (testAvailabilityProvider) RefreshAvailability(context.Context, AvailabilityProviderRequest) ([]AvailabilityObservation, error) {
	return nil, nil
}

func TestProviderRegistryMapsContractsAndAvailability(t *testing.T) {
	provider := testAvailabilityProvider{
		testCatalogProvider{contracts: []ProviderContract{{Code: "example_models_v1", Name: "Example models", MaxRequests: 2}}},
		[]AvailabilityContract{{Code: "example_health_v1", Name: "Example health", MaxRequests: 3}},
	}
	registry, err := NewProviderRegistry(provider)
	if err != nil {
		t.Fatal(err)
	}
	if got, maxRequests, ok := registry.Provider("EXAMPLE_MODELS_V1"); !ok || got == nil || maxRequests != 2 {
		t.Fatalf("provider lookup failed: %#v %d %v", got, maxRequests, ok)
	}
	bindings := registry.AvailabilityBindings()
	if len(bindings) != 1 || bindings[0].Contract != "example_health_v1" || bindings[0].MaxRequests != 3 || bindings[0].Provider == nil {
		t.Fatalf("availability bindings=%#v", bindings)
	}
	contracts := registry.Contracts()
	if len(contracts) != 1 || contracts[0].Code != "example_models_v1" || contracts[0].Name != "Example models" {
		t.Fatalf("contracts=%#v", contracts)
	}
}

func TestProviderRegistryRejectsDuplicateContractsAndUnknownAvailability(t *testing.T) {
	duplicate := testCatalogProvider{contracts: []ProviderContract{{Code: "same_v1", Name: "Same", MaxRequests: 1}}}
	if _, err := NewProviderRegistry(duplicate, duplicate); err != ErrProviderNotRegistered {
		t.Fatalf("duplicate error=%v", err)
	}
	invalidBudget := testCatalogProvider{contracts: []ProviderContract{{Code: "known_v1", Name: "Known", MaxRequests: 0}}}
	if _, err := NewProviderRegistry(invalidBudget); err != ErrProviderNotRegistered {
		t.Fatalf("invalid request budget error=%v", err)
	}
	invalidContract := testCatalogProvider{contracts: []ProviderContract{{Code: "invalid contract", Name: "Invalid", MaxRequests: 1}}}
	if _, err := NewProviderRegistry(invalidContract); err != ErrProviderNotRegistered {
		t.Fatalf("invalid contract error=%v", err)
	}
	emptyName := testCatalogProvider{contracts: []ProviderContract{{Code: "empty_name_v1", Name: " ", MaxRequests: 1}}}
	if _, err := NewProviderRegistry(emptyName); err != ErrProviderNotRegistered {
		t.Fatalf("empty name error=%v", err)
	}
	invalidAvailabilityBudget := testAvailabilityProvider{
		testCatalogProvider{contracts: []ProviderContract{{Code: "models_v1", Name: "Models", MaxRequests: 1}}},
		[]AvailabilityContract{{Code: "availability_v1", Name: "Availability", MaxRequests: 0}},
	}
	if _, err := NewProviderRegistry(invalidAvailabilityBudget); err != ErrProviderNotRegistered {
		t.Fatalf("invalid availability budget error=%v", err)
	}
	duplicateAvailability := testAvailabilityProvider{
		testCatalogProvider{contracts: []ProviderContract{{Code: "models_v1", Name: "Models", MaxRequests: 1}}},
		[]AvailabilityContract{{Code: "health_v1", Name: "Health", MaxRequests: 1}},
	}
	if _, err := NewProviderRegistry(duplicateAvailability, duplicateAvailability); err != ErrProviderNotRegistered {
		t.Fatalf("duplicate availability contract error=%v", err)
	}
}

func TestProviderRegistrySupportsDiscoveryOnlyProviderAndKeepsSnapshot(t *testing.T) {
	provider := testCatalogProvider{contracts: []ProviderContract{{Code: " EXAMPLE_MODELS_V1 ", Name: " Example models ", MaxRequests: 1}}}
	registry, err := NewProviderRegistry(provider)
	if err != nil {
		t.Fatal(err)
	}
	provider.contracts[0].Name = "changed"
	contracts := registry.Contracts()
	if len(contracts) != 1 || contracts[0].Code != "example_models_v1" || contracts[0].Name != "Example models" {
		t.Fatalf("contracts=%#v", contracts)
	}
	if len(registry.AvailabilityBindings()) != 0 {
		t.Fatal("discovery-only provider unexpectedly registered availability")
	}
}

func TestDefaultProviderContractsHaveCatalogAdapters(t *testing.T) {
	registry, err := DefaultProviderRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, contract := range registry.Contracts() {
		descriptor, ok := adapter.DescriptorFor(contract.Code, 1)
		if !ok || descriptor.Protocol != "catalog_discovery" {
			t.Fatalf("contract %q has no catalog adapter", contract.Code)
		}
	}
	for _, descriptor := range adapter.Descriptors() {
		if descriptor.Protocol != "catalog_discovery" {
			continue
		}
		if _, _, ok := registry.Provider(descriptor.Code); !ok {
			t.Fatalf("catalog adapter %q has no provider", descriptor.Code)
		}
	}
	if _, _, ok := registry.Provider(DeclarativeAvailabilityV1); ok {
		t.Fatal("availability cannot be a discovery contract")
	}
	availability, ok := registry.AvailabilityBinding(DeclarativeAvailabilityV1)
	if !ok || availability.Provider == nil || availability.MaxRequests != maxProviderRequests {
		t.Fatal("missing generic availability provider")
	}
	if len(registry.Contracts()) != 1 || len(registry.AvailabilityBindings()) != 1 {
		t.Fatal("only database-defined protocols should be registered")
	}
}
