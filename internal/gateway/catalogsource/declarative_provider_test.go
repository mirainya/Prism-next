package catalogsource

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestDeclarativeProviderDiscoversModelsFromConfiguredSteps(t *testing.T) {
	provider := NewDeclarativeProvider()
	base, _ := url.Parse("https://provider.invalid")
	definition := []byte(`{
      "schema_version":1,
      "kind":"catalog_discovery",
      "auth":{"mode":"bearer","header_name":"Authorization","header_format":"Bearer {secret}"},
      "steps":[{"name":"models","method":"GET","path":"/v1/models"}],
      "models":{"items_path":"data[]","model_path":"id","description_path":"description","group_path":"group"}
    }`)
	var seenHeader string
	result, err := provider.Discover(context.Background(), DiscoveryProviderRequest{
		Contract: DeclarativeCatalogV1, Secret: []byte("secret"), Definition: definition, BaseURL: base,
		Exchange: func(_ context.Context, method, target string, _ []byte, header http.Header) ([]byte, error) {
			if method != http.MethodGet || target != "https://provider.invalid/v1/models" {
				t.Fatalf("unexpected request: %s %s", method, target)
			}
			seenHeader = header.Get("Authorization")
			return []byte(`{"data":[{"id":"model-a","description":"A","group":"video"}]}`), nil
		},
	})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if seenHeader != "Bearer secret" || len(result.Models) != 1 || result.Models[0].Code != "model-a" || result.Models[0].Description != "A" {
		t.Fatalf("unexpected result: header=%q result=%#v", seenHeader, result.Models)
	}
}

func TestDeclarativeProviderUsesLoginValueForLaterStep(t *testing.T) {
	provider := NewDeclarativeProvider()
	base, _ := url.Parse("https://provider.invalid")
	definition := []byte(`{
      "schema_version":1,
      "kind":"availability",
      "auth":{"mode":"json_login","login_step":"login","token_path":"data.id","header_name":"X-User","header_format":"{value}"},
      "steps":[
        {"name":"login","method":"POST","path":"/login","body":{"username":"key","password":"password"}},
        {"name":"status","method":"GET","path":"/status"}
      ],
      "availability":{"items_path":"data[]","model_path":"model","channel_status_path":"status"}
    }`)
	var seenHeader string
	result, err := provider.RefreshAvailability(context.Background(), AvailabilityProviderRequest{
		Contract: DeclarativeAvailabilityV1, Definition: definition, BaseURL: base,
		Exchange: func(_ context.Context, method, target string, body []byte, header http.Header) ([]byte, error) {
			if method == http.MethodPost {
				var credentials map[string]string
				if json.Unmarshal(body, &credentials) != nil || credentials["username"] != "key" || credentials["password"] != "password" {
					t.Fatalf("unexpected login body: %s", body)
				}
				return []byte(`{"data":{"id":42}}`), nil
			}
			seenHeader = header.Get("X-User")
			return []byte(`{"data":[{"model":"model-a","status":"available"}]}`), nil
		},
	})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if seenHeader != "42" || len(result) != 1 || result[0].ModelCode != "model-a" || result[0].ChannelStatus != "available" {
		t.Fatalf("unexpected result: header=%q result=%#v", seenHeader, result)
	}
}

func TestDeclarativeProviderExpandsSecretAndAggregatesAvailabilitySteps(t *testing.T) {
	provider := NewDeclarativeProvider()
	base, _ := url.Parse("https://provider.invalid")
	definition := []byte(`{
      "schema_version":1,
      "kind":"availability",
      "auth":{"mode":"json_login","login_step":"login","token_path":"data.id","header_name":"X-Session-User","header_format":"{value}"},
      "steps":[
        {"name":"login","method":"POST","path":"/login","body":{"username":"${username}","password":"${password}"}},
        {"name":"language","method":"GET","path":"/status","query":{"category":"language"}},
        {"name":"video","method":"GET","path":"/status","query":{"category":"video"}}
      ],
      "availability":{"source_steps":["language","video"],"items_path":"data[]","model_path":"model","channel_status_path":"status"}
    }`)
	result, err := provider.RefreshAvailability(context.Background(), AvailabilityProviderRequest{
		Contract: DeclarativeAvailabilityV1, Secret: []byte(`{"username":"key","password":"pass"}`), Definition: definition, BaseURL: base,
		Exchange: func(_ context.Context, method, target string, body []byte, header http.Header) ([]byte, error) {
			switch method {
			case http.MethodPost:
				var credentials map[string]string
				if json.Unmarshal(body, &credentials) != nil || credentials["username"] != "key" || credentials["password"] != "pass" {
					t.Fatalf("unexpected expanded body: %s", body)
				}
				return []byte(`{"data":{"id":42}}`), nil
			case http.MethodGet:
				if header.Get("X-Session-User") != "42" {
					t.Fatalf("missing login header: %q", header.Get("X-Session-User"))
				}
				if strings.Contains(target, "category=language") {
					return []byte(`{"data":[{"model":"model-language","status":"available"}]}`), nil
				}
				return []byte(`{"data":[{"model":"model-video","status":"degraded"}]}`), nil
			default:
				t.Fatalf("unexpected method: %s", method)
				return nil, nil
			}
		},
	})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(result) != 2 || result[0].ModelCode != "model-language" || result[1].ModelCode != "model-video" {
		t.Fatalf("unexpected aggregated result: %#v", result)
	}
}
