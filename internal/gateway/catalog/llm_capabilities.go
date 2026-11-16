package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
)

var ErrInvalidLLMCapabilities = errors.New("invalid LLM capability metadata")

// Pointers preserve the difference between unknown and explicitly unsupported.
// This is model metadata; it does not impose request limits or change codecs.
type LLMCapabilities struct {
	ContextWindow     *uint64 `json:"context_window,omitempty"`
	SupportsStreaming *bool   `json:"supports_streaming,omitempty"`
	SupportsTools     *bool   `json:"supports_tools,omitempty"`
	SupportsVision    *bool   `json:"supports_vision,omitempty"`
	SupportsAudio     *bool   `json:"supports_audio,omitempty"`
	SupportsReasoning *bool   `json:"supports_reasoning,omitempty"`
}

// ReadLLMCapabilities projects only declared public fields. Unknown config keys,
// such as transport or vendor data, never become part of the public metadata.
func ReadLLMCapabilities(raw []byte) (*LLMCapabilities, error) {
	var root map[string]json.RawMessage
	if len(raw) == 0 {
		return nil, nil
	}
	if json.Unmarshal(raw, &root) != nil || root == nil {
		return nil, ErrInvalidLLMCapabilities
	}
	value, exists := root["llm"]
	if !exists || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return nil, nil
	}
	var options LLMCapabilities
	if json.Unmarshal(value, &options) != nil {
		return nil, ErrInvalidLLMCapabilities
	}
	// JSON clients can represent integers exactly only through 2^53-1.
	if options.ContextWindow != nil && (*options.ContextWindow == 0 || *options.ContextWindow > 9007199254740991) {
		return nil, ErrInvalidLLMCapabilities
	}
	if options == (LLMCapabilities{}) {
		return nil, nil
	}
	return &options, nil
}

func (options LLMCapabilities) Features() []string {
	features := make([]string, 0, 5)
	for _, item := range []struct {
		name  string
		value *bool
	}{
		{"stream", options.SupportsStreaming}, {"tools", options.SupportsTools},
		{"vision", options.SupportsVision}, {"audio", options.SupportsAudio},
		{"reasoning", options.SupportsReasoning},
	} {
		if item.value != nil && *item.value {
			features = append(features, item.name)
		}
	}
	return features
}
