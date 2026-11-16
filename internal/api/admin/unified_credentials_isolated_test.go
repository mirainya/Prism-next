package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

const isolatedCredentialBody = `{"credential_code":"test","secret":"test-only-secret","weight":1,"purposes":["execution"]}`

func isolatedCredentialRouter(actor uint) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if actor != 0 {
			c.Set("user_id", actor)
		}
	})
	router.POST("/channels/:id/credentials", CreateUnifiedIsolatedCredential)
	return router
}

func TestIsolatedCredentialWriteValidatesAndRedacts(t *testing.T) {
	for _, body := range []string{
		`null`, `{}`, `{} {}`,
		strings.Replace(isolatedCredentialBody, `"weight":1`, `"weight":0`, 1),
		strings.Replace(isolatedCredentialBody, `"execution"`, `"unknown"`, 1),
		strings.Replace(isolatedCredentialBody, `"execution"`, `"execution","execution"`, 1),
		strings.Replace(isolatedCredentialBody, `test-only-secret`, `test-only-secret\r\nInjected: yes`, 1),
		strings.Replace(isolatedCredentialBody, `"weight":1`, `"weight":1,"pool_id":7`, 1),
	} {
		response := httptest.NewRecorder()
		isolatedCredentialRouter(1).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/channels/7/credentials", strings.NewReader(body)))
		if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "test-only-secret") {
			t.Fatalf("code=%d body=%s", response.Code, response.Body)
		}
	}
}

func TestIsolatedCredentialWriteRequiresAdmin(t *testing.T) {
	response := httptest.NewRecorder()
	isolatedCredentialRouter(0).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/channels/7/credentials", strings.NewReader(isolatedCredentialBody)))
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), "test-only-secret") {
		t.Fatalf("code=%d body=%s", response.Code, response.Body)
	}
}

func TestIsolatedCredentialWriteRejectsDisabledChannel(t *testing.T) {
	mock := unifiedChannelMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status FROM gateway_channels").WithArgs(uint64(7)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("disabled"))
	mock.ExpectRollback()
	response := httptest.NewRecorder()
	isolatedCredentialRouter(1).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/channels/7/credentials", strings.NewReader(isolatedCredentialBody)))
	if response.Code != http.StatusConflict || strings.Contains(response.Body.String(), "test-only-secret") {
		t.Fatalf("code=%d body=%s", response.Code, response.Body)
	}
}

func TestRegisterRoutesIncludesIsolatedCredentialCreation(t *testing.T) {
	router := gin.New()
	RegisterRoutes(router.Group("/api/admin"))
	for _, route := range router.Routes() {
		if route.Method == http.MethodPost && route.Path == "/api/admin/unified-gateway/channels/:id/credentials" {
			return
		}
	}
	t.Fatal("isolated credential creation route not registered")
}
