package adapter

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mirainya/Prism/internal/gateway/delivery"
	"github.com/mirainya/Prism/internal/gateway/execution"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

func audioTestDispatch(t *testing.T, variant int) repository.AsyncDispatch {
	t.Helper()
	config := `{"audio":{"url_paths":["output.link"],"format_paths":["output.codec"],"input_max_characters":20,"emotion_max_strength":2},"adapter":{"profile":"json_task_v1","auth_key":"X-Token","auth_prefix":"","submit":{"enabled":true,"method":"POST","path":"/jobs"},"poll":{"enabled":true,"method":"GET","path":"/jobs/{task_id}"},"request":{"fields":{"model":"engine","input":"payload.text","reference_audio":"payload.voice","emotion.mode":"mood","emotion.happy":"weights.joy"},"value_maps":{"emotion.mode":{"vector":"weights"}},"fixed_body":{"quality":"high"}},"response":{"root":"data","task_id_paths":["job"],"status_paths":["state"],"status_map":{"waiting":"submitted","working":"tracking","done":"completed","broken":"failed"}}}}`
	if variant == 2 {
		config = strings.NewReplacer(`"/jobs"`, `"/synthesis"`, `"/jobs/{task_id}"`, `"/synthesis/{task_id}"`, `"payload.text"`, `"text"`, `"payload.voice"`, `"voice_url"`, `"X-Token"`, `"Authorization"`, `"auth_prefix":""`, `"auth_prefix":"Bearer "`, `"job"`, `"ticket"`, `"state"`, `"phase"`, `"done":"completed"`, `"ready":"completed"`, `"output.link"`, `"artifact.url"`, `"output.codec"`, `"artifact.format"`).Replace(config)
	}
	fixed := repository.AsyncDispatch{BaseURL: "https://provider.example", Method: "POST", Path: "/jobs", VendorModel: "private-engine", AdapterConfig: []byte(config), DeliveryMode: "managed_copy"}
	if variant == 2 {
		fixed.Path = "/synthesis"
		fixed.VendorModel = "another-engine"
	}
	if err := ValidateCatalogProduct("generic_audio", 1, fixed.BaseURL, fixed.Method, fixed.Path, fixed.VendorModel, "task", fixed.AdapterConfig); err != nil {
		t.Fatal(err)
	}
	return fixed
}

func TestGenericAudioSwitchesUpstreamsThroughConfiguration(t *testing.T) {
	input := []byte(`{"model":"public-voice","input":"你好","reference_audio":"https://assets.example/voice.wav","emotion":{"mode":"vector","strengths":{"happy":0.5}}}`)
	codec := GenericAudio{}
	for _, variant := range []int{1, 2} {
		fixed := audioTestDispatch(t, variant)
		prepared, err := codec.Prepare(context.Background(), "submit", fixed, input, "")
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if json.Unmarshal(prepared.Body, &body) != nil || body["engine"] != fixed.VendorModel || body["model"] != nil || body["quality"] != "high" || body["mood"] != "weights" {
			t.Fatalf("mapped request: %s", prepared.Body)
		}
		if variant == 1 && (prepared.Path != "/jobs" || prepared.CredentialHeader != "X-Token" || prepared.CredentialPrefix != "" || body["payload"].(map[string]any)["text"] != "你好") {
			t.Fatalf("first request: %+v", prepared)
		}
		if variant == 2 && (prepared.Path != "/synthesis" || prepared.CredentialPrefix != "Bearer " || body["text"] != "你好") {
			t.Fatalf("second request: %+v", prepared)
		}
		query, err := codec.Prepare(context.Background(), "query", fixed, nil, "task-1")
		if err != nil || query.Path != fixed.Path+"/task-1" {
			t.Fatalf("query: %+v, %v", query, err)
		}
		response := `{"data":{"job":"task-1","state":"done","output":{"link":"https://assets.example/download","codec":"mp3"}}}`
		if variant == 2 {
			response = `{"data":{"ticket":"task-1","phase":"ready","artifact":{"url":"https://assets.example/download","format":"flac"}}}`
		}
		out, err := codec.DecodeWithDispatch("query", fixed, input, []byte(response))
		if err != nil || out.State != execution.AsyncSucceeded {
			t.Fatalf("completion: %+v, %v", out, err)
		}
		bound, err := delivery.BindResult(delivery.ResourceCapabilityTask, out.Result, out.Sources, []uint64{42})
		if err != nil || strings.Contains(string(bound), "assets.example") {
			t.Fatalf("result: %s, %v", bound, err)
		}
		var result delivery.AudioResult
		if json.Unmarshal(bound, &result) != nil || result.DeliveryID != 42 || result.Format == "wav" {
			t.Fatalf("format: %s", bound)
		}
		recovered, err := delivery.DecodeManagedSource("image", delivery.EncodeManagedSource(out.Sources[0]))
		if err != nil || recovered.Role != "audio" || recovered.AudioFormat != result.Format {
			t.Fatalf("recovery: %+v, %v", recovered, err)
		}
	}
}

func TestGenericAudioValidatesPublicInputAndModelLimits(t *testing.T) {
	for _, raw := range []string{`null`, `{"model":"voice","input":""}`, `{"model":"voice","input":"hi","vendor_option":"url"}`, `{"model":"voice","input":"hi","emotion":{"mode":"reference"}}`, `{"model":"voice","input":"hi","emotion":{"strengths":{"happy":-1}}}`, `{"model":"voice","input":"hi"} {}`} {
		if _, err := DecodeSpeechRequest(strings.NewReader(raw)); err == nil {
			t.Fatalf("accepted: %s", raw)
		}
	}
	fixed := audioTestDispatch(t, 1)
	for _, raw := range []string{`{"model":"voice","input":"` + strings.Repeat("字", 21) + `"}`, `{"model":"voice","input":"hi","emotion":{"strengths":{"happy":3}}}`, `{"model":"voice","input":"hi","emotion":{"random":false}}`} {
		if _, err := (GenericAudio{}).Prepare(context.Background(), "submit", fixed, []byte(raw), ""); err == nil {
			t.Fatalf("accepted unsupported input: %s", raw)
		}
	}
	// Plain text synthesis does not require reference audio unless configured.
	if _, err := (GenericAudio{}).Prepare(context.Background(), "submit", fixed, []byte(`{"model":"voice","input":"hi"}`), ""); err != nil {
		t.Fatal(err)
	}
	fixed.AdapterConfig = []byte(strings.Replace(string(fixed.AdapterConfig), `"input_max_characters":20`, `"input_max_characters":20,"reference_audio_required":true`, 1))
	if _, err := (GenericAudio{}).Prepare(context.Background(), "submit", fixed, []byte(`{"model":"voice","input":"hi"}`), ""); err == nil {
		t.Fatal("required reference was ignored")
	}
}

func TestGenericAudioRejectsInvalidResultsAndHidesFailureDetails(t *testing.T) {
	fixed := audioTestDispatch(t, 1)
	codec := GenericAudio{}
	for _, response := range []string{`{"data":{"state":"unknown"}}`, `{"data":{"state":"done"}}`, `{"data":{"state":"done","output":{"link":"file:///private","codec":"wav"}}}`, `{"data":{"state":"done","output":{"link":"https://assets.example/a","codec":"exe"}}}`} {
		if _, err := codec.DecodeWithDispatch("query", fixed, nil, []byte(response)); err == nil {
			t.Fatalf("accepted: %s", response)
		}
	}
	out, err := codec.DecodeWithDispatch("query", fixed, nil, []byte(`{"data":{"state":"broken","message":"private-provider at https://private.example"}}`))
	if err != nil || out.State != execution.AsyncFailed || strings.Contains(out.ProviderErrorCode+out.ProviderErrorMessage, "private") {
		t.Fatalf("failure: %+v, %v", out, err)
	}
	fixed.AdapterConfig = []byte(strings.Replace(string(fixed.AdapterConfig), `"response":{`, `"response":{"success_code_path":"code","success_code_values":["ok"],`, 1))
	out, err = codec.DecodeWithDispatch("submit", fixed, nil, []byte(`{"code":"denied","msg":"private-provider"}`))
	if err != nil || out.State != execution.AsyncFailed {
		t.Fatalf("rejection: %+v, %v", out, err)
	}
}
