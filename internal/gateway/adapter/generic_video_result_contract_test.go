package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/mirainya/Prism/internal/gateway/billing"
	"github.com/mirainya/Prism/internal/gateway/delivery"
	"github.com/mirainya/Prism/internal/gateway/execution"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

func resultContractDispatch(t *testing.T, protected bool) repository.AsyncDispatch {
	t.Helper()
	response := map[string]any{
		"task_id_paths": []string{"id"}, "status_paths": []string{"status"},
		"video_url_paths": []string{"result.url"}, "duration_paths": []string{"usage.output_seconds"},
		"status_map":            map[string]string{"queued": "submitted", "running": "tracking", "done": "completed", "error": "failed"},
		"submit_default_status": "submitted", "poll_default_status": "tracking", "unknown_status": "tracking",
	}
	if protected {
		response["result_auth"] = "upstream"
		response["billing_seconds_paths"] = []string{"usage.billable_seconds"}
		response["billing_seconds_max"] = 60
		response["video_url_template"] = "/videos/{task_id}/content"
		response["relative_result_url_base"] = "https://provider.example"
	}
	config, err := json.Marshal(map[string]any{"task_types": []string{"text"}, "resolutions": []string{"720p"}, "adapter": map[string]any{
		"profile": "json_task_v1", "auth_location": "header", "auth_key": "Authorization", "auth_prefix": "Bearer ",
		"submit":   map[string]any{"enabled": true, "method": "POST", "path": "/videos"},
		"poll":     map[string]any{"enabled": true, "method": "GET", "path": "/videos/{task_id}"},
		"request":  map[string]any{"fields": map[string]string{"model": "model", "prompt": "prompt", "duration": "seconds"}, "include_content": false},
		"response": response,
		"validation": map[string]any{"models": map[string]any{"video-basic": map[string]any{
			"duration_min": 1, "duration_max": 30, "resolutions": []string{"720p"}, "task_modes": []string{"text"},
		}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	fixed := repository.AsyncDispatch{
		BaseURL: "https://provider.example", Method: "POST", Path: "/videos", VendorModel: "video-basic",
		PublicID: "call-example", AdapterConfig: config, DeliveryMode: "managed_copy",
	}
	if err := ValidateCatalogProduct("generic", 1, fixed.BaseURL, fixed.Method, fixed.Path, fixed.VendorModel, "task", config); err != nil {
		t.Fatal(err)
	}
	return fixed
}

func TestGenericVideoProtectedResultRequiresManagedCopy(t *testing.T) {
	codec := GenericVideo{}
	request := []byte(`{"prompt":"animate","duration":4,"resolution":"720p","task_mode":"text"}`)
	for _, protected := range []bool{false, true} {
		fixed := resultContractDispatch(t, protected)
		if required, err := codec.RequiresManagedCopy(fixed); err != nil || required != protected {
			t.Fatalf("protected=%v required=%v error=%v", protected, required, err)
		}
		if _, err := codec.Prepare(context.Background(), "submit", fixed, request, ""); err != nil {
			t.Fatalf("managed copy rejected: %v", err)
		}
		fixed.DeliveryMode = "reference"
		if _, err := codec.Prepare(context.Background(), "submit", fixed, request, ""); (err != nil) != protected {
			t.Fatalf("protected=%v reference error=%v", protected, err)
		}
	}
}

func TestGenericVideoSeparatesBillingAndOutputDuration(t *testing.T) {
	fixed := resultContractDispatch(t, true)
	out, err := (GenericVideo{}).DecodeWithDispatch("query", fixed, []byte(`{"duration":4}`),
		[]byte(`{"id":"task/1","status":"done","usage":{"output_seconds":5,"billable_seconds":11}}`))
	if err != nil {
		t.Fatal(err)
	}
	if out.State != execution.AsyncSucceeded || len(out.Sources) != 1 || out.Sources[0].URL != "https://provider.example/videos/task%2F1/content" {
		t.Fatalf("unexpected completed result: %+v", out)
	}
	auth := out.Sources[0].UpstreamAuth
	if auth == nil || auth.Header != "Authorization" || auth.Prefix != "Bearer " {
		t.Fatalf("missing result authentication: %+v", auth)
	}
	if out.Facts.Quantities[billing.QuantityBilledSeconds] != "11" || out.Facts.Quantities[billing.QuantityGeneratedSeconds] != "5" || out.Facts.Quantities[billing.QuantityRequestedSeconds] != "4" {
		t.Fatalf("billing and duration facts were conflated: %+v", out.Facts)
	}
	var result delivery.VideoResult
	if err := json.Unmarshal(out.Result, &result); err != nil || result.Duration != "5" {
		t.Fatalf("output duration changed: %+v, %v", result, err)
	}
}

func TestGenericVideoRejectsInvalidConfiguredBillingMeter(t *testing.T) {
	fixed := resultContractDispatch(t, true)
	for _, value := range []string{"null", "0", "-1", "5.5", `"invalid"`, "true", "{}", "[]", "61", "1e2147483647", `"1e2147483647"`, "100000000000000000000"} {
		body := []byte(fmt.Sprintf(`{"id":"task-1","status":"done","usage":{"output_seconds":5,"billable_seconds":%s}}`, value))
		if _, err := (GenericVideo{}).DecodeWithDispatch("query", fixed, []byte(`{"duration":4}`), body); err == nil {
			t.Fatalf("invalid billing meter %s accepted", value)
		}
	}
	if _, err := (GenericVideo{}).DecodeWithDispatch("query", fixed, []byte(`{"duration":4}`), []byte(`{"id":"task-1","status":"done"}`)); err == nil {
		t.Fatal("missing billing meter accepted")
	}
	if _, err := (GenericVideo{}).DecodeWithDispatch("query", fixed, []byte(`{"duration":4}`), []byte(`{"id":"task-1","status":"done","usage":{"billable_seconds":"11"}}`)); err != nil {
		t.Fatalf("integer string meter rejected: %v", err)
	}
	for _, state := range []string{"queued", "running", "error"} {
		body := []byte(fmt.Sprintf(`{"id":"task-1","status":%q}`, state))
		out, err := (GenericVideo{}).DecodeWithDispatch("query", fixed, []byte(`{"duration":4}`), body)
		if err != nil || len(out.Sources) != 0 {
			t.Fatalf("unfinished task required billing or exposed a result: %+v, %v", out, err)
		}
	}
}

func TestGenericVideoResultOptionsRemainDisabledByDefault(t *testing.T) {
	fixed := resultContractDispatch(t, false)
	for _, usage := range []string{`"billable_seconds":11`, `"billable_seconds":"invalid"`, `"seconds":5`} {
		body := []byte(fmt.Sprintf(`{"id":"task-1","status":"done","usage":{"output_seconds":5,%s},"result":{"url":"https://media.example/video.mp4"}}`, usage))
		out, err := (GenericVideo{}).DecodeWithDispatch("query", fixed, []byte(`{"duration":4}`), body)
		if err != nil || len(out.Sources) != 1 {
			t.Fatalf("default result decoding changed: %+v, %v", out, err)
		}
		if out.Sources[0].UpstreamAuth != nil || out.Facts.Quantities[billing.QuantityBilledSeconds] != "" || out.Facts.Quantities[billing.QuantityGeneratedSeconds] != "5" {
			t.Fatalf("optional features affected default behavior: %+v", out)
		}
	}
}
