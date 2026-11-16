package admin

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

func priceDisplayRouter(actor uint) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if actor != 0 {
			c.Set("user_id", actor)
		}
	})
	router.GET("/price-display", GetPriceDisplaySettings)
	router.PUT("/price-display", UpdatePriceDisplaySettings)
	router.POST("/price-display/preview", PreviewPriceDisplaySettings)
	return router
}

func TestPriceDisplayUpdateRejectsInvalidBodiesBeforeDatabase(t *testing.T) {
	for _, body := range []string{
		`null`, `{} {}`, `{"default_multiplier":"1.8"}`,
		`{"default_multiplier":"0","config_version":1}`,
		`{"default_multiplier":"1e3","config_version":1}`,
		`{"default_multiplier":"1.8","config_version":1,"sell_rate":"2"}`,
		`{"default_multiplier":"1.8","config_version":1,"model_multipliers":{"model":"-1"}}`,
		`{"default_multiplier":"1.8","config_version":1,"sku_multipliers":{"sku":2}}`,
		`{"default_multiplier":"1.8","config_version":1,"model_multipliers":{"` + strings.Repeat("a", 128*1024) + `":"2"}}`,
	} {
		response := httptest.NewRecorder()
		priceDisplayRouter(7).ServeHTTP(response, httptest.NewRequest("PUT", "/price-display", strings.NewReader(body)))
		if response.Code != 400 {
			t.Fatalf("code=%d body=%s", response.Code, response.Body)
		}
	}
}

func TestPriceDisplayUpdateRequiresActorBeforeProjection(t *testing.T) {
	response := httptest.NewRecorder()
	priceDisplayRouter(0).ServeHTTP(response, httptest.NewRequest("PUT", "/price-display", strings.NewReader(`{"default_multiplier":"1.8","config_version":1}`)))
	if response.Code != 403 {
		t.Fatalf("code=%d body=%s", response.Code, response.Body)
	}
}

func TestPriceDisplayPreviewRejectsInvalidSettingsBeforeDatabase(t *testing.T) {
	for _, body := range []string{
		`{"model":"image-model","settings":{"default_multiplier":"0"}}`,
		`{"model":"image-model","settings":{"default_multiplier":"1.8","sku_multipliers":{"sku":"-1"}}}`,
		`{"model":" ","settings":{"default_multiplier":"1.8"}}`,
	} {
		response := httptest.NewRecorder()
		priceDisplayRouter(7).ServeHTTP(response, httptest.NewRequest("POST", "/price-display/preview", strings.NewReader(body)))
		if response.Code != 400 {
			t.Fatalf("code=%d body=%s", response.Code, response.Body)
		}
	}
}

func TestPriceDisplayReadReturnsVersionAndEmptyOverrideObjects(t *testing.T) {
	mock := unifiedChannelMock(t)
	mock.ExpectQuery("SELECT default_multiplier,model_multipliers,sku_multipliers,config_version FROM gw_price_display_settings").
		WillReturnRows(sqlmock.NewRows([]string{"default_multiplier", "model_multipliers", "sku_multipliers", "config_version"}).AddRow("1.8", "null", "{}", 3))
	response := httptest.NewRecorder()
	priceDisplayRouter(7).ServeHTTP(response, httptest.NewRequest("GET", "/price-display", nil))
	var result struct {
		Data repository.PriceDisplaySettings `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || result.Data.DefaultMultiplier != "1.8" || result.Data.ConfigVersion != 3 || result.Data.ModelMultipliers == nil || result.Data.SKUMultipliers == nil {
		t.Fatalf("code=%d body=%s", response.Code, response.Body)
	}
}
