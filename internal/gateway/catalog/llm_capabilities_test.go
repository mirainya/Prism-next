package catalog

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestReadLLMCapabilitiesPreservesUnknownAndExplicitFalse(t *testing.T) {
	for _, raw := range []string{"", `{}`, `{"llm":null}`, `{"llm":{}}`, `{"llm":{"vendor":"private"}}`} {
		options, err := ReadLLMCapabilities([]byte(raw))
		if err != nil || options != nil {
			t.Fatalf("unknown %s: options=%#v err=%v", raw, options, err)
		}
	}
	options, err := ReadLLMCapabilities([]byte(`{"llm":{"context_window":128000,"supports_tools":false,"supports_streaming":true,"vendor":"private","secret":"hidden"}}`))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(options)
	if err != nil {
		t.Fatal(err)
	}
	if options.SupportsTools == nil || *options.SupportsTools || options.SupportsVision != nil || !slices.Equal(options.Features(), []string{"stream"}) {
		t.Fatalf("unknown/false conflated: %#v", options)
	}
	for _, forbidden := range []string{"vendor", "private", "secret", "hidden", "supports_vision"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("private/unknown field published: %s", encoded)
		}
	}
}

func TestReadLLMCapabilitiesValidatesPublicFieldTypesAndExactIntegers(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{"llm":[]}`, `{"llm":"yes"}`, `{"llm":{"supports_tools":1}}`, `{"llm":{"context_window":0}}`, `{"llm":{"context_window":-1}}`, `{"llm":{"context_window":1.5}}`, `{"llm":{"context_window":"128000"}}`, `{"llm":{"context_window":9007199254740992}}`} {
		if _, err := ReadLLMCapabilities([]byte(raw)); !errors.Is(err, ErrInvalidLLMCapabilities) {
			t.Fatalf("accepted %s: %v", raw, err)
		}
	}
	options, err := ReadLLMCapabilities([]byte(`{"llm":{"context_window":9007199254740991}}`))
	if err != nil || options.ContextWindow == nil || *options.ContextWindow != 9007199254740991 {
		t.Fatalf("exact boundary rejected: %#v %v", options, err)
	}
}
