package open

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/service"
)

func TestPublicChatModelPublishesOnlyCatalogOperations(t *testing.T) {
	item := publicChatModel(service.AvailableModelCapability{
		ID:          "gpt-4.1",
		ModelCode:   "gpt_4_1",
		Name:        "GPT-4.1",
		Description: "General-purpose model",
		Type:        "chat",
		Types:       []string{"chat"},
		Visibility:  "public",
		Availability: &service.PricingAvailability{
			SuccessRate: "99.5", Source: "upstream", WindowMinutes: 120,
			ObservedAt: time.Unix(1789380174, 0).UTC(),
		},
		Operations: []service.AvailableModelOperation{
			{ID: "chat.completions", Path: "/v1/chat/completions"},
			{ID: "responses.create", Path: "/v1/responses"},
			{ID: "responses.create", Path: "/v1/responses"},
		},
	})

	endpoints, ok := item["supported_endpoints"].([]string)
	if !ok || !slices.Equal(endpoints, []string{"/v1/chat/completions", "/v1/responses"}) {
		t.Fatalf("supported_endpoints = %#v", item["supported_endpoints"])
	}
	operations, ok := item["supported_operations"].([]string)
	if !ok || !slices.Equal(operations, []string{"chat.completions", "responses.create"}) {
		t.Fatalf("supported_operations = %#v", item["supported_operations"])
	}
	if slices.Contains(endpoints, "/v1/messages") {
		t.Fatal("an undeclared endpoint was published")
	}
	if item["description"] != "General-purpose model" || item["type"] != "chat" {
		t.Fatalf("metadata = %#v", item)
	}
	if item["model_code"] != "gpt-4.1" {
		t.Fatalf("public model_code exposed internal identity: %#v", item["model_code"])
	}
	if _, exists := item["video_options"]; exists {
		t.Fatal("chat model must not have video options")
	}
	if _, exists := item["availability"]; exists {
		t.Fatal("dynamic availability must be queried separately")
	}
}

func TestPublicChatModelOmitsUnknownAvailability(t *testing.T) {
	item := publicChatModel(service.AvailableModelCapability{ID: "image-model", Type: "image"})
	if _, exists := item["availability"]; exists {
		t.Fatal("unknown availability must be omitted rather than reported as zero")
	}
}

func TestModelAvailabilityPublishesStatusWithoutRate(t *testing.T) {
	observed := time.Unix(1789380174, 0).UTC()
	estimated := uint64(285000)
	item := service.ModelAvailability{
		ID: "seedance-2.5",
		Availability: &service.PricingAvailability{
			ObservedAt: observed, ChannelStatus: "正常", EstimatedCompletionMS: &estimated,
			ChannelObservedAt: &observed,
		},
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		ID           string         `json:"id"`
		Availability map[string]any `json:"availability"`
	}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result.ID != "seedance-2.5" || result.Availability["channel_status"] != "正常" || result.Availability["estimated_completion_ms"] != float64(estimated) {
		t.Fatalf("status-only availability was not published: %s", encoded)
	}
	if _, exists := result.Availability["success_rate"]; exists {
		t.Fatal("unknown success rate must not become zero")
	}
}

func TestPublicModelSummaryOmitsDetailsAndInternalConfiguration(t *testing.T) {
	item := publicModelSummary(service.AvailableModelCapability{
		ID: "image-model", ModelCode: "private-model",
		VideoOptions: map[string]any{"max_images": 3}, LLMOptions: map[string]any{"context_window": 128000},
		Pricing: &service.AvailableModelPricing{Currency: "USD"}, Availability: &service.PricingAvailability{SuccessRate: "99"},
		Channels: []service.AvailableModelChannel{{ChannelName: "private-channel"}},
	})
	for _, key := range []string{"video_options", "llm_options", "pricing", "availability", "channels", "operations"} {
		if _, exists := item[key]; exists {
			t.Fatalf("summary exposed %s", key)
		}
	}
	if item["model_code"] != "image-model" {
		t.Fatal("summary exposed internal model identity")
	}
}

func TestModelAvailabilityRouteIsNotModelDetail(t *testing.T) {
	router := gin.New()
	group := router.Group("/v1", func(c *gin.Context) {
		c.String(http.StatusOK, c.FullPath())
		c.Abort()
	})
	RegisterRoutes(group)
	for _, test := range []struct{ path, route string }{
		{"/v1/models", "/v1/models"},
		{"/v1/models/availability", "/v1/models/availability"},
		{"/v1/models/image-model", "/v1/models/:code"},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
		if response.Code != http.StatusOK || response.Body.String() != test.route {
			t.Fatalf("%s matched %s (%d)", test.path, response.Body.String(), response.Code)
		}
	}
}

func TestPublicChatModelPublishesVideoOptions(t *testing.T) {
	options := map[string]any{"task_types": []string{"text", "multimodal"}, "max_images": 9, "max_audios": 3}
	pricing := &service.AvailableModelPricing{Currency: "CNY", Variants: []service.AvailableModelPricingVariant{{
		ServiceTiers: []string{"standard"}, Components: []service.AvailableModelPriceComponent{{Component: "video", Unit: "second", Mode: "expression", Formula: "seconds * 0.3", Maximum: "9"}},
	}}}
	item := publicChatModel(service.AvailableModelCapability{ID: "seedance", Type: "video", VideoOptions: options, Pricing: pricing})
	projected, ok := item["video_options"].(map[string]any)
	if !ok || projected["max_images"] != 9 || projected["max_audios"] != 3 {
		t.Fatalf("video options missing: %#v", item["video_options"])
	}
	if _, exposed := item["upstream_description"]; exposed {
		t.Fatalf("upstream description must stay internal: %#v", item)
	}
	if item["pricing"] == nil {
		t.Fatal("structured downstream pricing was not published")
	}
}

func TestPublicChatModelPublishesDeclaredLLMOptionsAndOmitsUnknown(t *testing.T) {
	for _, options := range []map[string]any{nil, {}} {
		item := publicChatModel(service.AvailableModelCapability{ID: "model-a", Type: "chat", LLMOptions: options})
		if _, exists := item["llm_options"]; exists {
			t.Fatal("unknown LLM options must be omitted")
		}
	}
	item := publicChatModel(service.AvailableModelCapability{ID: "model-a", Type: "chat", LLMOptions: map[string]any{"context_window": 128000, "supports_tools": false}})
	options, ok := item["llm_options"].(map[string]any)
	if !ok || options["context_window"] != 128000 || options["supports_tools"] != false {
		t.Fatalf("metadata lost: %#v", item)
	}
}
