package catalogsource

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/mirainya/Prism/internal/gateway/repository"
)

func TestConfiguredDiscoveryAcceptsEmptyListButRejectsMissingList(t *testing.T) {
	definition := []byte(`{"schema_version":1,"kind":"catalog_discovery","auth":{"mode":"bearer","header_name":"Authorization","header_format":"Bearer {secret}"},"steps":[{"name":"models","method":"GET","path":"/v1/models"}],"models":{"items_path":"data[]","model_path":"id"}}`)
	base, _ := url.Parse("https://provider.invalid")
	for _, payload := range []string{`{"data":[]}`, `{"data":[{"id":"model-a"}]}`, `{}`, `{"data":null}`, `{"data":{}}`} {
		calls := 0
		result, err := NewDeclarativeProvider().Discover(context.Background(), DiscoveryProviderRequest{
			Contract: DeclarativeCatalogV1, ExternalGroup: "default", Definition: definition, Secret: []byte("test-only-key"), BaseURL: base,
			Exchange: func(_ context.Context, method, target string, _ []byte, header http.Header) ([]byte, error) {
				calls++
				if method != "GET" || target != "https://provider.invalid/v1/models" || header.Get("Authorization") != "Bearer test-only-key" {
					t.Fatal("discovery ignored configured endpoint or Key")
				}
				return []byte(payload), nil
			},
		})
		valid := payload == `{"data":[]}` || payload == `{"data":[{"id":"model-a"}]}`
		if calls != 1 || valid != (err == nil) {
			t.Fatalf("payload=%s calls=%d error=%v", payload, calls, err)
		}
		if payload == `{"data":[]}` && len(result.Models) != 0 {
			t.Fatal("empty list was not preserved")
		}
		if len(result.Models) == 1 && (result.Models[0].ModelPrice != nil || len(result.Models[0].EndpointTypes) != 0 || result.Models[0].Description != "") {
			t.Fatal("discovery invented missing price or capabilities")
		}
	}
}

func TestDiscoveryDoesNotUseRevokedOrRotatedCredential(t *testing.T) {
	secret := []byte("test-only-key")
	definition := []byte("test-only-definition")
	// No runtime or HTTP client is configured: reaching either would panic.
	err := (&Worker{}).process(context.Background(), repository.CatalogDiscoveryRun{
		ID: 1, AuthorizationValid: false, CredentialSecret: secret, ProtocolDefinition: definition,
	})
	if !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("error=%v", err)
	}
	if discoveryFailureCode(err) != "discovery_configuration_changed" {
		t.Fatal("authorization change has no diagnostic reason")
	}
	for _, data := range [][]byte{secret, definition} {
		for _, value := range data {
			if value != 0 {
				t.Fatal("rejected discovery retained credential material")
			}
		}
	}
}
