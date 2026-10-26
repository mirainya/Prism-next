package catalogsource

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDeclarativePriceMappingPreservesAllDecimalFacts(t *testing.T) {
	mapping := &DeclarativeModelMapping{ItemsPath: "data[]", ModelPath: "name", DescriptionPath: "description", TagsPath: "tags", GroupPath: "groups[]", EndpointTypesPath: "endpoints[]", VendorIDPath: "vendor", QuotaTypePath: "quota", PricePath: "price", ModelRatioPath: "ratio", CompletionRatioPath: "completion", OwnerByPath: "owner", PricingVersionPath: "version", RequireGroups: true, RequireEndpoints: true}
	items, err := mapDeclarativeModels([]byte(`{"data":[{"name":"video-a","description":"line\n two","tags":"tag\t two","groups":["z","a"],"endpoints":["video"],"vendor":9007199254740993,"quota":1,"price":0.123456789123456789,"ratio":1.2,"completion":3,"owner":"vendor","version":"v2"}]}`), mapping, "a")
	if err != nil {
		t.Fatal(err)
	}
	item := items[0]
	if *item.VendorID != 9007199254740993 || *item.ProviderQuotaType != 1 || *item.ModelPrice != "0.123456789123456789" || *item.ModelRatio != "1.2" || *item.CompletionRatio != "3" || item.Description != "line two" || item.Tags != "tag two" || item.OwnerBy != "vendor" || item.PricingVersion != `"v2"` || !reflect.DeepEqual(item.Groups, []string{"a", "z"}) {
		t.Fatalf("facts lost: %+v", item)
	}
	for _, payload := range []string{`{"data":null}`, `{"data":[{"name":"x","groups":[],"endpoints":["video"]}]}`, `{"data":[{"name":"x","groups":["a"],"endpoints":["video"],"price":-1}]}`, `{"data":[{"name":"x","groups":["a"],"endpoints":["video"],"quota":2}]}`} {
		if _, err := mapDeclarativeModels([]byte(payload), mapping, "a"); err == nil {
			t.Fatalf("accepted malformed facts: %s", payload)
		}
	}
}
func TestDeclarativeAvailabilityUsesResponseMetadataAndSamples(t *testing.T) {
	mapping := &DeclarativeAvailabilityMap{ItemsPath: "data.rows[]", ModelPath: "model", CategoryPath: "$.data.category", SuccessRatePath: "rate", HasDataPath: "buckets[].sample", AverageCompletionSecondsPath: "completion", WindowMinutesPath: "$.data.window", ObservedAtPath: "$.data.updated", Deduplicate: "latest"}
	raw := []byte(`{"data":{"category":"video","window":120,"updated":1789380174,"rows":[{"model":"a","rate":88.3,"completion":495.3,"buckets":[{"sample":true}]},{"model":"b","rate":100,"buckets":[]}]}}`)
	items, err := mapDeclarativeAvailability(raw, nil, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Category != "video" || items[0].SuccessRate != "88.3" || items[0].WindowMinutes != 120 || items[0].AverageCompletionSeconds != "495.3" || !items[0].ObservedAt.Equal(time.Unix(1789380174, 0)) {
		t.Fatalf("invalid metadata: %+v", items)
	}
	for _, bad := range []string{strings.Replace(string(raw), `"window":120`, `"window":0`, 1), strings.Replace(string(raw), `"updated":1789380174`, `"updated":0`, 1), strings.Replace(string(raw), `"rate":88.3`, `"rate":101`, 1)} {
		if _, err := mapDeclarativeAvailability([]byte(bad), nil, mapping); err == nil {
			t.Fatal("accepted invalid report")
		}
	}
}
func TestDeclarativeAvailabilityAggregatesChildrenAndDeduplicates(t *testing.T) {
	mapping := &DeclarativeAvailabilityMap{ItemsPath: "data[]", ModelPath: "name", CategoryPath: "category", ChannelStatusPath: "status", AggregateChildrenETA: true, Deduplicate: "worst", Children: &DeclarativeAvailabilityMap{ItemsPath: "routes[]", ModelPath: "name", CategoryPath: "$parent.category", ChannelStatusPath: "status", StatusFallbackPath: "$parent.status", EstimatedCompletionMSPath: "pace.ms", EstimatedCompletionMatch: &DeclarativeCheck{Path: "pace.kind", Equals: json.RawMessage(`"generation"`)}}}
	raw := []byte(`{"data":[{"name":"video-a","category":"video","status":"正常","routes":[{"name":"route-a","pace":{"kind":"generation","ms":285000}},{"name":"route-b","status":"服务波动","pace":{"kind":"generation","ms":6152000}},{"name":"route-c","pace":{"kind":"queue","ms":9000000}}]},{"name":"ROUTE-A","category":"video","status":"服务波动","routes":[]}]}`)
	items, err := mapDeclarativeAvailability(raw, nil, mapping)
	if err != nil {
		t.Fatal(err)
	}
	facts := map[string]AvailabilityObservation{}
	for _, item := range items {
		facts[strings.ToLower(item.ModelCode)] = item
	}
	if len(facts) != 4 || *facts["video-a"].EstimatedCompletionMS != 6152000 || facts["route-a"].ChannelStatus != "服务波动" || *facts["route-a"].EstimatedCompletionMS != 285000 || facts["route-c"].EstimatedCompletionMS != nil || facts["route-b"].Category != "video" {
		t.Fatalf("invalid facts: %+v", facts)
	}
	if items, err := mapDeclarativeAvailability([]byte(`{"data":[]}`), nil, mapping); err != nil || len(items) != 0 {
		t.Fatal("empty report must clear cache")
	}
}
func TestDeclarativeBodyEscapesCredentialValues(t *testing.T) {
	values := map[string]any{"username": "key", "password": "quote\" slash\\ newline\n", "id": json.Number("9007199254740993")}
	raw, err := expandDeclarativeBody(json.RawMessage(`{"username":"${username}","password":"${password}","id":"${id}"}`), values)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := decodeDeclarativeJSON(raw, &result); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, values) {
		t.Fatal("credential substitution changed data")
	}
	if _, err := expandDeclarativeBody(json.RawMessage(`{"password":"${missing}"}`), values); err == nil {
		t.Fatal("missing credential variable accepted")
	}
}
func TestDeclarativeLoginFailureStopsBeforeFetchingData(t *testing.T) {
	raw := []byte(`{"schema_version":1,"kind":"availability","auth":{"mode":"json_login","login_step":"login","token_path":"data.id","header_name":"X-User","header_format":"{value}"},"steps":[{"name":"login","method":"POST","path":"/login","checks":[{"path":"success","equals":true}]},{"name":"status","method":"GET","path":"/status"}],"availability":{"items_path":"data[]","model_path":"name","channel_status_path":"status"}}`)
	for _, response := range []string{`{"success":false,"data":{"id":42}}`, `{"success":true,"data":{"id":0}}`, `{"success":true,"data":{"id":42}} {"another":true}`} {
		calls := 0
		_, err := NewDeclarativeProvider().RefreshAvailability(context.Background(), AvailabilityProviderRequest{Contract: DeclarativeAvailabilityV1, Definition: raw, BaseURL: mustCatalogURL(t, "https://provider.invalid"), Exchange: func(context.Context, string, string, []byte, http.Header) ([]byte, error) {
			calls++
			return []byte(response), nil
		}})
		if err == nil || calls != 1 {
			t.Fatalf("failed login continued: calls=%d err=%v", calls, err)
		}
	}
}

func TestAvailabilityBucketsPreserveNoSamples(t *testing.T) {
	mapping := &DeclarativeAvailabilityMap{ItemsPath: "data.rows[]", ModelPath: "model", SuccessRatePath: "rate", HasDataPath: "buckets[].has_data", BucketsPath: "buckets", DefaultWindowMinutes: 120}
	raw := []byte(`{"data":{"rows":[{"model":"video-a","rate":0,"buckets":[{"label":"12:00","start":1000,"end":1300,"has_data":false,"has_failure":false,"success_rate":0}]}]}}`)
	items, err := mapDeclarativeAvailability(raw, nil, mapping)
	if err != nil || len(items) != 1 {
		t.Fatalf("buckets lost: %v %+v", err, items)
	}
	if items[0].SuccessRate != "" || !strings.Contains(string(items[0].Buckets), `"has_data":false`) {
		t.Fatal("no samples became a failure rate")
	}
	bad := strings.Replace(string(raw), `"end":1300`, `"end":900`, 1)
	if _, err := mapDeclarativeAvailability([]byte(bad), nil, mapping); err == nil {
		t.Fatal("accepted reversed time interval")
	}
}
