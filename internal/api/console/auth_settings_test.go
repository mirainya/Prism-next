package console

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/pkg/config"
)

func TestAuthSettingsAndRegistrationUseSamePolicy(t *testing.T) {
	previous := config.Get()
	t.Cleanup(func() { config.C = previous })
	for _, enabled := range []bool{false, true} {
		config.C = &config.Config{Server: config.ServerConfig{RegistrationEnabled: enabled, JWTSecret: "must-not-appear"}}
		router := gin.New()
		RegisterAuthRoutes(router.Group("/auth"))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/auth/settings", nil))
		var body struct {
			Data struct {
				Enabled bool `json:"registration_enabled"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if response.Code != 200 || body.Data.Enabled != enabled || response.Header().Get("Cache-Control") != "no-store" || strings.Contains(response.Body.String(), "must-not-appear") {
			t.Fatalf("settings response=%s", response.Body)
		}
		if !enabled {
			request := httptest.NewRequest("POST", "/auth/register", strings.NewReader(`{"username":"blocked-user","password":"test-password"}`))
			request.Header.Set("Content-Type", "application/json")
			response = httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != 403 || !strings.Contains(response.Body.String(), "公开注册已关闭") {
				t.Fatalf("disabled registration response=%d %s", response.Code, response.Body)
			}
		}
	}
}
