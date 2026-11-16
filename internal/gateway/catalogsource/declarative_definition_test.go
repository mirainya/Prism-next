package catalogsource

import "testing"

func TestValidateDeclarativeProtocol(t *testing.T) {
	definition, err := ValidateDeclarativeProtocol([]byte(`{
      "schema_version":1,
      "kind":"availability",
      "auth":{"mode":"json_login","login_step":"login","token_path":"data.id","header_name":"X-Session-User","header_format":"{value}"},
      "steps":[
        {"name":"login","method":"POST","path":"/login","save":{"user_id":"data.id"}},
        {"name":"status","method":"GET","path":"/api/status"}
      ],
      "availability":{"items_path":"data.items[]","model_path":"name","category_path":"category","channel_status_path":"latest_status","estimated_completion_ms_path":"pace.ms"}
    }`))
	if err != nil {
		t.Fatalf("validate declarative protocol: %v", err)
	}
	if definition.Kind != "availability" || len(definition.Steps) != 2 {
		t.Fatalf("unexpected definition: %#v", definition)
	}
}

func TestValidateDeclarativeProtocolRejectsExecutableFields(t *testing.T) {
	_, err := ValidateDeclarativeProtocol([]byte(`{
      "schema_version":1,
      "kind":"catalog_discovery",
      "auth":{"mode":"none"},
      "steps":[{"name":"list","method":"GET","path":"/v1/models"}],
      "models":{"items_path":"data.items[]","model_path":"name"},
      "script":"process.exit(1)"
    }`))
	if err == nil {
		t.Fatal("expected unknown executable field to be rejected")
	}
}

func TestValidateDeclarativeProtocolRejectsUnsafePath(t *testing.T) {
	_, err := ValidateDeclarativeProtocol([]byte(`{
      "schema_version":1,
      "kind":"catalog_discovery",
      "auth":{"mode":"none"},
      "steps":[{"name":"list","method":"GET","path":"/v1/models"}],
      "models":{"items_path":"data.items[]","model_path":"data.items[].name()"}
    }`))
	if err == nil {
		t.Fatal("expected unsafe path to be rejected")
	}
}
