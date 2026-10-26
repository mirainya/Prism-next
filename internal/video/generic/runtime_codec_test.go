package generic

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mirainya/Prism/internal/video"
)

func runtimeH3Config() []byte {
	return []byte(`{
  "adapter": {
    "profile": "json_task_v1",
    "auth_location": "header",
    "auth_key": "Authorization",
    "auth_prefix": "",
    "submit": {"enabled":true,"method":"POST","path":"/api/v1/workflows/h3"},
    "poll": {"enabled":true,"method":"GET","path":"/api/v1/workflows/result/{task_id}"},
    "request": {
      "fields":{"prompt":"prompt","resolution":"resolution","duration":"duration"},
      "include_content":false,
      "content_projections":[
        {"source":"url","target":"ref_image_0","output":"scalar","types":["image_url"],"index":0},
        {"source":"url","target":"ref_audio_0","output":"scalar","types":["audio_url"],"index":0}
      ],
      "params_mode":"merge_missing",
      "request_id_header":"X-Request-ID"
    },
    "response": {
      "task_id_paths":["data.task_id"],
      "status_paths":["data.status"],
      "video_url_paths":["data.results.0.url"],
      "thumbnail_url_paths":["data.thumbnail_url"],
      "duration_paths":["data.duration"],
      "error_paths":["data.message"],
      "status_map":{"queued":"submitted","running":"tracking","success":"completed","failed":"failed"},
      "submit_default_status":"submitted",
      "poll_default_status":"tracking",
      "unknown_status":"tracking"
    },
    "validation":{"models":{"minimax-h3":{"duration_min":1,"duration_max":15,"resolutions":["768p"],"task_modes":["multimodal"],"allow_generated_audio":false,"max_images":9,"max_audios":3,"max_media":12,"parameters":[{"name":"seed","type":"integer","min":1,"max":999999999999999,"options":[]}]}}}
  }
}`)
}

func TestRuntimeCodecPreparesH3WithoutCredentialMaterial(t *testing.T) {
	codec, err := NewRuntimeCodec("https://autodl.example", runtimeH3Config())
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := codec.PrepareSubmit(context.Background(), &video.GenerateRequest{
		Model: "minimax-h3", Prompt: "animate", Resolution: "768p", Duration: 5,
		TaskMode: "multimodal", TaskID: "call-1", Params: map[string]any{"seed": 731242627237534.0},
		Content: []video.ContentItem{
			{Type: "image_url", Role: "first_frame", URL: "https://assets.example/frame.png"},
			{Type: "audio_url", Role: "reference_audio", URL: "https://assets.example/audio.mp3"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Method != "POST" || prepared.Path != "/api/v1/workflows/h3" || prepared.CredentialHeader != "Authorization" || prepared.CredentialPrefix != "" {
		t.Fatalf("prepared request = %+v", prepared)
	}
	var body map[string]any
	if err := json.Unmarshal(prepared.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["prompt"] != "animate" || body["ref_image_0"] != "https://assets.example/frame.png" || body["ref_audio_0"] != "https://assets.example/audio.mp3" || body["Authorization"] != nil {
		t.Fatalf("provider body = %#v", body)
	}
	query, err := codec.PrepareQuery("task/with?reserved")
	if err != nil {
		t.Fatal(err)
	}
	if query.Method != "GET" || query.Path != "/api/v1/workflows/result/task%2Fwith%3Freserved" {
		t.Fatalf("query = %+v", query)
	}
}

func TestRuntimeCodecDecodesH3AndPreservesExactDuration(t *testing.T) {
	codec, err := NewRuntimeCodec("https://autodl.example", runtimeH3Config())
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := codec.DecodeSubmit([]byte(`{"data":{"task_id":"task-1","status":"queued"}}`))
	if err != nil || submitted.ProviderTaskID != "task-1" || submitted.Status != video.VideoTaskStatusSubmitted {
		t.Fatalf("submit = %+v, err = %v", submitted, err)
	}
	completed, err := codec.DecodePoll([]byte(`{"data":{"task_id":"task-1","status":"success","duration":6.123456789123456789,"results":[{"url":"https://media.example/result.mp4"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != video.VideoTaskStatusCompleted || completed.Result == nil || completed.Result.VideoURL != "https://media.example/result.mp4" || completed.Duration != "6.123456789123456789" {
		t.Fatalf("completed = %+v", completed)
	}
}

func TestRuntimeCodecBuildsAuthenticatedResultFromTaskIDTemplate(t *testing.T) {
	raw := []byte(`{
  "adapter": {
    "profile": "json_task_v1",
    "auth_location": "header",
    "auth_key": "Authorization",
    "auth_prefix": "Bearer ",
    "submit": {"enabled":true,"method":"POST","path":"/v1/videos"},
    "poll": {"enabled":true,"method":"GET","path":"/v1/videos/{task_id}"},
    "request": {"fields":{"model":"model","prompt":"prompt","duration":"seconds"},"include_content":false},
    "response": {
      "task_id_paths":["id"],
      "status_paths":["status"],
      "video_url_template":"/v1/videos/{task_id}/content",
      "relative_result_url_base":"https://provider.example",
      "result_auth":"upstream",
      "status_map":{"queued":"submitted","in_progress":"tracking","completed":"completed","failed":"failed"},
      "submit_default_status":"submitted",
      "poll_default_status":"tracking",
      "unknown_status":"tracking"
    },
    "validation":{"models":{"test-video":{"duration_min":5,"duration_max":30,"resolutions":["480p"],"task_modes":["text"],"allow_generated_audio":false}}}
  }
}`)
	codec, err := NewRuntimeCodec("https://provider.example", raw)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := codec.DecodePoll([]byte(`{"id":"task/with?reserved","status":"completed"}`))
	if err != nil {
		t.Fatal(err)
	}
	want := "https://provider.example/v1/videos/task%2Fwith%3Freserved/content"
	if decoded.ProviderTaskID != "task/with?reserved" || decoded.Status != video.VideoTaskStatusCompleted || decoded.Result == nil || decoded.Result.VideoURL != want {
		t.Fatalf("decoded = %+v result = %+v, want video URL %q", decoded, decoded.Result, want)
	}
}

func TestRuntimeCodecRejectsUnsafeVideoURLTemplates(t *testing.T) {
	for _, template := range []string{
		"https://evil.example/v1/videos/{task_id}/content",
		"//evil.example/v1/videos/{task_id}/content",
		"/v1/videos/{task_id}/{other}/content",
		"/v1/videos/{task_id}/{task_id}/content",
	} {
		raw, err := json.Marshal(map[string]any{"adapter": map[string]any{
			"profile": "json_task_v1", "auth_location": "header", "auth_key": "Authorization",
			"submit":   map[string]any{"enabled": true, "method": "POST", "path": "/v1/videos"},
			"poll":     map[string]any{"enabled": true, "method": "GET", "path": "/v1/videos/{task_id}"},
			"response": map[string]any{"task_id_paths": []string{"id"}, "status_paths": []string{"status"}, "video_url_template": template, "result_auth": "upstream"},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := NewRuntimeCodec("https://provider.example", raw); err == nil {
			t.Fatalf("unsafe video URL template %q was accepted", template)
		}
	}
}

func TestRuntimeCodecRejectsCredentialInBodyOrQuery(t *testing.T) {
	for _, location := range []string{"body", "query"} {
		var envelope map[string]any
		if err := json.Unmarshal(runtimeH3Config(), &envelope); err != nil {
			t.Fatal(err)
		}
		envelope["adapter"].(map[string]any)["auth_location"] = location
		raw, _ := json.Marshal(envelope)
		if _, err := NewRuntimeCodec("https://autodl.example", raw); err == nil {
			t.Fatalf("auth_location %q was accepted", location)
		}
	}
}

func TestValidateRuntimeCatalogPublishedModesAndContent(t *testing.T) {
	base := map[string]any{}
	if err := json.Unmarshal(runtimeH3Config(), &base); err != nil {
		t.Fatal(err)
	}
	adapter := base["adapter"].(map[string]any)
	request := adapter["request"].(map[string]any)
	models := adapter["validation"].(map[string]any)["models"].(map[string]any)
	modelRule := models["minimax-h3"].(map[string]any)
	base["task_types"] = []string{"multimodal"}
	request["content_projections"] = []any{
		map[string]any{"source": "url", "target": "images", "output": "array", "types": []string{"image_url"}},
		map[string]any{"source": "url", "target": "audios", "output": "array", "types": []string{"audio_url"}},
	}
	encode := func() []byte {
		t.Helper()
		out, err := json.Marshal(base)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	validate := func() error {
		return ValidatePublishedRuntimeCatalog("https://autodl.example", "POST", "/api/v1/workflows/h3", "minimax-h3", encode())
	}
	if err := validate(); err != nil {
		t.Fatalf("valid model rejected: %v", err)
	}
	base["task_types"] = []string{"text"}
	if err := validate(); err == nil {
		t.Fatal("published mode absent from validation was accepted")
	}
	base["task_types"] = []string{"multimodal"}
	modelRule["task_modes"] = []string{"text", "multimodal"}
	if err := validate(); err == nil {
		t.Fatal("validation-only mode was accepted")
	}
	modelRule["task_modes"] = []string{"multimodal"}
	request["content_projections"] = []any{map[string]any{
		"source": "url", "target": "ref_image_0", "types": []string{"image_url"}, "index": 0,
	}}
	if err := validate(); err == nil {
		t.Fatal("unmapped audio was accepted")
	}
	request["include_content"] = true
	request["content_fields"] = map[string]string{"url": "url"}
	if err := validate(); err != nil {
		t.Fatalf("content passthrough rejected: %v", err)
	}
	request["include_content"] = false
	request["content_projections"] = []any{
		map[string]any{"source": "url", "target": "ref_image_0", "types": []string{"image_url"}},
		map[string]any{"source": "url", "target": "ref_audio_0", "types": []string{"audio_url"}, "models": []string{"other-model"}},
	}
	if err := validate(); err == nil {
		t.Fatal("another model's audio mapping was accepted")
	}
}
