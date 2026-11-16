package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestActiveLLMMetadataUsesOnlyCommonExecutableRouteFields(t *testing.T) {
	for _, test := range []struct {
		name, second             string
		contextKnown, toolsKnown bool
	}{
		{"identical", `{"context_window":128000,"supports_streaming":true,"supports_tools":false}`, true, true},
		{"different", `{"context_window":64000,"supports_streaming":true,"supports_tools":true}`, false, false},
		{"missing", `{}`, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := setupPublicCatalogQueryDB(t)
			// Remove pre-existing feature tags so declared metadata is the only source.
			for _, statement := range []string{
				`UPDATE gw_catalog_models SET capability_tags='{}'`,
				`UPDATE gw_products SET capability_constraints='{"llm":{"context_window":128000,"supports_streaming":true,"supports_tools":false,"vendor":"private","secret":"hidden"}}' WHERE id=70`,
				`UPDATE gw_products SET capability_constraints='{"llm":` + test.second + `}' WHERE id=71`,
			} {
				if _, err := db.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			items, err := newQueryServiceWithDB(db).ListAvailableCapabilities(context.Background(), "", "chat")
			if err != nil || len(items) != 1 {
				t.Fatalf("models=%#v err=%v", items, err)
			}
			options := items[0].LLMOptions
			if _, known := options["context_window"]; known != test.contextKnown {
				t.Fatalf("context borrowed across routes: %#v", options)
			}
			if value, known := options["supports_tools"]; known != test.toolsKnown || known && value != false {
				t.Fatalf("explicit false lost or borrowed: %#v", options)
			}
			if _, known := options["supports_vision"]; known {
				t.Fatalf("invented capability: %#v", options)
			}
			if !items[0].SupportsStream {
				t.Fatal("stream feature not applied")
			}
			encoded, _ := json.Marshal(options)
			if strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "secret") {
				t.Fatalf("private metadata leaked: %s", encoded)
			}
			if test.name == "missing" && len(options) != 0 {
				t.Fatalf("missing route metadata must be unknown: %#v", options)
			}
		})
	}
}

func TestInactiveRoutesDoNotSuppressDeclaredLLMMetadata(t *testing.T) {
	db := setupPublicCatalogQueryDB(t)
	for _, statement := range []string{
		`UPDATE gw_products SET capability_constraints='{"llm":{"context_window":128000}}' WHERE id=70`,
		`UPDATE gw_offering_runtime_state SET state='disabled' WHERE offering_id=41`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	items, err := newQueryServiceWithDB(db).ListAvailableCapabilities(context.Background(), "", "chat")
	if err != nil || len(items) != 1 || items[0].LLMOptions["context_window"] != float64(128000) {
		t.Fatalf("models=%#v err=%v", items, err)
	}
}
