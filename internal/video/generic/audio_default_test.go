package generic

import (
	"context"
	"encoding/json"
	"github.com/mirainya/Prism/internal/video"
	"testing"
)

func TestAudioDurationDefault(t *testing.T) {
	for _, duration := range []float64{0, 6.5} {
		config := runtimeH3Config()
		var value map[string]any
		if err := json.Unmarshal(config, &value); err != nil {
			t.Fatal(err)
		}
		declaration := value["adapter"].(map[string]any)
		requestConfig := declaration["request"].(map[string]any)
		requestConfig["audio_duration_default"] = 18
		requestConfig["include_content"] = true
		requestConfig["content_fields"] = map[string]any{"type": "type", "url": "url", "duration": "duration_seconds"}
		config, _ = json.Marshal(value)
		codec, err := NewRuntimeCodec("https://example.com", config)
		if err != nil {
			t.Fatal(err)
		}
		request := &video.GenerateRequest{Model: "minimax-h3", Duration: 5, Resolution: "768p", TaskMode: "multimodal", Content: []video.ContentItem{{Type: "audio_url", URL: "https://example.com/a.mp3", DurationSeconds: duration}}}
		prepared, err := codec.PrepareSubmit(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Content []struct {
				Duration float64 `json:"duration_seconds"`
			} `json:"content"`
		}
		if err := json.Unmarshal(prepared.Body, &body); err != nil {
			t.Fatal(err)
		}
		expected := duration
		if expected == 0 {
			expected = 18
		}
		if len(body.Content) != 1 || body.Content[0].Duration != expected {
			t.Fatalf("body=%s", prepared.Body)
		}
		if request.Content[0].DurationSeconds != duration {
			t.Fatal("caller request mutated")
		}
	}
}
