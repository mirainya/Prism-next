package admin

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

const catalogModelOnboardBody = `{
  "expected_active_release_id":7,
  "expected_config_version":3,
  "sku":{
    "model_code":"new-model",
    "api_name":"new-model",
    "display_name":"New model",
    "description":"A model",
    "visibility":"visible",
    "capability_tags":["llm"],
    "operation_code":"chat.completions",
    "contract_version":1,
    "http_method":"POST",
    "route_template":"/v1/chat/completions",
    "normalization_version":1,
    "sku_code":"new-model-standard",
    "variant_code":"default",
    "delivery_mode":"reference",
    "max_results":1,
    "idempotency_mode":"optional",
    "service_tiers":["standard"]
  },
  "product":{
    "channel_id":11,
    "credential_pool_id":12,
    "product_code":"new-model-primary",
    "vendor_model":"provider-new-model",
    "capability_constraints":{},
    "constraints_schema_version":1,
    "adapter_code":"openai_chat",
    "adapter_version":1,
    "transport_code":"new-model-chat-v1",
    "base_url":"https://api.example.com",
    "request_method":"POST",
    "request_path":"/v1/chat/completions",
    "auth_scheme":"bearer",
    "transport_timeout_ms":30000,
    "task_timeout_ms":30000,
    "task_scope":"request",
    "cancel_mode":"none",
    "source_url_policy":"fixed",
    "upstream_scope_kind":"product_transport",
    "upstream_scope_key":"new-model-chat-v1",
    "allowed_hosts":[],
    "actions":[],
    "cost_plan_code":"new-model-cost"
  },
  "route":{"priority":100,"weight":100},
  "sell_rate":{"unit_code":"request","unit_price":"1.25","component_code":"request","quantity_source":"one","charge_event":"call.succeeded","unit_scale":0,"quantity_step":"0","max_quantity":"1","pricing_mode":"flat","pricing_expr":""},
  "cost_rate":{"unit_code":"request","unit_price":"1.00","component_code":"request","quantity_source":"one","charge_event":"call.succeeded","unit_scale":0,"quantity_step":"0","max_quantity":"1","pricing_mode":"flat","pricing_expr":""}
}`

func catalogModelOnboardRouter(actor uint) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if actor != 0 {
			c.Set("user_id", actor)
		}
	})
	router.POST("/catalog-changes/model-onboard", OnboardUnifiedCatalogModel)
	return router
}

func TestCatalogModelOnboardValidatesBeforeAdminWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := map[string]string{
		"missing active release": strings.Replace(catalogModelOnboardBody, `"expected_active_release_id":7`, `"expected_active_release_id":0`, 1),
		"missing version":        strings.Replace(catalogModelOnboardBody, `"expected_config_version":3`, `"expected_config_version":0`, 1),
		"nested sku version":     strings.Replace(catalogModelOnboardBody, `"model_code":"new-model"`, `"expected_version":3,"model_code":"new-model"`, 1),
		"caller route id":        strings.Replace(catalogModelOnboardBody, `"cost_plan_code":"new-model-cost"`, `"cost_plan_code":"new-model-cost","routes":[{"sku_id":99,"priority":1,"weight":1}]`, 1),
		"missing sell price":     strings.Replace(catalogModelOnboardBody, `"unit_price":"1.25"`, `"unit_price":""`, 1),
		"invalid cost rate":      strings.Replace(catalogModelOnboardBody, `"unit_price":"1.00"`, `"unit_price":""`, 1),
		"unknown adapter":        strings.Replace(catalogModelOnboardBody, `"adapter_code":"openai_chat"`, `"adapter_code":"missing"`, 1),
		"adapter path mismatch":  strings.Replace(catalogModelOnboardBody, `"route_template":"/v1/chat/completions"`, `"route_template":"/v1/images/generations"`, 1),
		"unknown field":          strings.Replace(catalogModelOnboardBody, `"channel_id":11`, `"unexpected":true,"channel_id":11`, 1),
		"trailing JSON":          catalogModelOnboardBody + ` {}`,
		"null":                   `null`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest("POST", "/catalog-changes/model-onboard", strings.NewReader(body))
			catalogModelOnboardRouter(1).ServeHTTP(response, request)
			if response.Code != 400 {
				t.Fatalf("code=%d body=%s", response.Code, response.Body)
			}
		})
	}
}

func TestCatalogModelOnboardRequiresAdminAfterValidation(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/catalog-changes/model-onboard", strings.NewReader(catalogModelOnboardBody))
	catalogModelOnboardRouter(0).ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatalf("code=%d body=%s", response.Code, response.Body)
	}
}

func TestCatalogModelOnboardAcceptsOmittedCostButRejectsMalformedCost(t *testing.T) {
	var body map[string]any
	if err := json.Unmarshal([]byte(catalogModelOnboardBody), &body); err != nil {
		t.Fatal(err)
	}
	delete(body, "cost_rate")
	for _, test := range []struct {
		cost   any
		status int
	}{
		{nil, 403},
		{map[string]any{}, 400},
	} {
		if test.cost != nil {
			body["cost_rate"] = test.cost
		}
		encoded, _ := json.Marshal(body)
		response := httptest.NewRecorder()
		catalogModelOnboardRouter(0).ServeHTTP(response, httptest.NewRequest("POST", "/catalog-changes/model-onboard", strings.NewReader(string(encoded))))
		if response.Code != test.status {
			t.Fatalf("code=%d body=%s", response.Code, response.Body)
		}
	}
}

func TestCatalogModelOnboardAcceptsTokenSchedulesAndRejectsDuplicateMeters(t *testing.T) {
	input := `{"unit_code":"token","unit_price":"2","component_code":"input","quantity_source":"usage.input_tokens","charge_event":"call.succeeded","unit_scale":6,"quantity_step":"0","max_quantity":"128000"}`
	output := `{"unit_code":"token","unit_price":"8","component_code":"output","quantity_source":"usage.output_tokens","charge_event":"call.succeeded","unit_scale":6,"quantity_step":"0","max_quantity":"32768"}`
	body := strings.Replace(catalogModelOnboardBody, `"sell_rate":{"unit_code":"request","unit_price":"1.25","component_code":"request","quantity_source":"one","charge_event":"call.succeeded","unit_scale":0,"quantity_step":"0","max_quantity":"1","pricing_mode":"flat","pricing_expr":""}`, `"sell_rate":`+input+`,"additional_sell_rates":[`+output+`]`, 1)
	body = strings.Replace(body, `"cost_rate":{"unit_code":"request","unit_price":"1.00","component_code":"request","quantity_source":"one","charge_event":"call.succeeded","unit_scale":0,"quantity_step":"0","max_quantity":"1","pricing_mode":"flat","pricing_expr":""}`, `"cost_rate":`+input+`,"additional_cost_rates":[`+output+`]`, 1)
	for _, test := range []struct {
		body   string
		status int
	}{
		{body, 403},
		{strings.ReplaceAll(body, `"component_code":"output"`, `"component_code":"input"`), 400},
		{strings.ReplaceAll(body, `"usage.output_tokens"`, `"usage.input_tokens"`), 400},
		{strings.ReplaceAll(body, `"32768"`, `""`), 400},
	} {
		response := httptest.NewRecorder()
		catalogModelOnboardRouter(0).ServeHTTP(response, httptest.NewRequest("POST", "/catalog-changes/model-onboard", strings.NewReader(test.body)))
		if response.Code != test.status {
			t.Fatalf("code=%d body=%s", response.Code, response.Body)
		}
	}
}

func TestCatalogModelOnboardUnknownVariantIsBadRequest(t *testing.T) {
	router := gin.New()
	router.GET("/", func(c *gin.Context) {
		unifiedChannelError(c, fmt.Errorf("variant rejected: %w", repository.ErrUnknownVariant))
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	if response.Code != 400 {
		t.Fatalf("code=%d body=%s", response.Code, response.Body)
	}
}
