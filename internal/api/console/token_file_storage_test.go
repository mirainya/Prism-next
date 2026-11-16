package console

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/pkg/config"
)

func TestDecodeTokenFileStorageBodyIsStrict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	valid := `{"base_url":"https://storage.example","api_key":"xfs_0123456789abcdef0123456789abcdef"}`
	for name, body := range map[string]string{
		"empty":         "",
		"malformed":     `{`,
		"null":          `null`,
		"unknown field": strings.TrimSuffix(valid, "}") + `,"provider":"other"}`,
		"trailing json": valid + ` {}`,
		"too large":     `{"base_url":"` + strings.Repeat("a", 4100) + `","api_key":"xfs_0123456789abcdef0123456789abcdef"}`,
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			ctx.Request = httptest.NewRequest(http.MethodPut, "/api/tokens/1/file-storage", strings.NewReader(body))
			if _, ok := decodeTokenFileStorageBody(ctx); ok {
				t.Fatal("invalid body accepted")
			}
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d", response.Code)
			}
		})
	}

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/tokens/1/file-storage", strings.NewReader(valid))
	req, ok := decodeTokenFileStorageBody(ctx)
	if !ok || req.BaseURL != "https://storage.example" || req.APIKey != "xfs_0123456789abcdef0123456789abcdef" {
		t.Fatal("valid binding request was not decoded")
	}
}

func TestListTokenFileStorageEndpointsUsesApprovedURLs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.C
	config.C = &config.Config{FileStorage: config.FileStorageConfig{
		BaseURL:         "https://legacy.example",
		AllowedBaseURLs: []string{"https://STORAGE.example/", "https://storage.example", "https://other.example"},
	}}
	t.Cleanup(func() { config.C = previous })

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/tokens/file-storage-endpoints", nil)
	ListTokenFileStorageEndpoints(ctx)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"https://storage.example"`) ||
		!strings.Contains(response.Body.String(), `"https://other.example"`) || strings.Contains(response.Body.String(), `"https://legacy.example"`) {
		t.Fatalf("unexpected endpoint response: %s", response.Body.String())
	}
}
