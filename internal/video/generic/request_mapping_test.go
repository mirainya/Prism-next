package generic

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/mirainya/Prism/internal/video"
)

func TestRuntimeTaskIdentityAndValueMappings(t *testing.T) {
	raw := strings.Replace(string(runtimeH3Config()), `"fields":{"prompt":"prompt","resolution":"resolution","duration":"duration"}`, `"fields":{"task_id":"original_id","task_uuid":"id","resolution":"resolution"},"value_maps":{"resolution":{"768p":"768P"}}`, 1)
	codec, err := NewRuntimeCodec("https://provider.example", []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	request := &video.GenerateRequest{Model: "minimax-h3", Prompt: "test", Duration: 5, Resolution: "768p", TaskMode: "multimodal", TaskID: "call-one"}
	prepared, err := codec.PrepareSubmit(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	firstID := body["id"]
	if body["original_id"] != "call-one" || body["resolution"] != "768P" || firstID != uuid.NewSHA1(uuid.NameSpaceURL, []byte("call-one")).String() {
		t.Fatalf("body = %v", body)
	}
	request.TaskID = "call-two"
	prepared, err = codec.PrepareSubmit(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["id"] == firstID {
		t.Fatal("different calls shared a UUID")
	}
	request.TaskID = ""
	if _, err := codec.PrepareSubmit(context.Background(), request); err == nil {
		t.Fatal("missing task identity accepted")
	}
	request.TaskID = "call-one"
	codec.adapter.config.Request.ValueMaps["resolution"] = map[string]string{"720p": "720P"}
	if _, err := codec.PrepareSubmit(context.Background(), request); err == nil {
		t.Fatal("unmapped value accepted")
	}
	raw = strings.Replace(raw, `"value_maps":{"resolution":{"768p":"768P"}}`, `"value_maps":{"unknown":{"a":"b"}}`, 1)
	if _, err := NewRuntimeCodec("https://provider.example", []byte(raw)); err == nil {
		t.Fatal("unknown mapping source accepted")
	}
}
