package admin

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/middleware"
)

func TestBillingAccountEndpointsRequireAdministrator(t *testing.T) {
	for _, role := range []string{"", "user"} {
		for _, method := range []string{"GET", "POST"} {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(middleware.ContextKeyUserID, uint(5))
				c.Set(middleware.ContextKeyUserRole, role)
			})
			router.GET("/users/:id/billing-account", GetUserBillingAccount)
			router.POST("/users/:id/billing-account", UnfreezeUserBillingAccount)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(method, "/users/1/billing-account", strings.NewReader(`{"expected_freeze_event_id":20,"note":"已核查"}`)))
			if response.Code != 403 {
				t.Fatalf("role=%q method=%s status=%d", role, method, response.Code)
			}
		}
	}
}

func TestBillingAccountRecoveryValidatesInputBeforeOpeningTransaction(t *testing.T) {
	for _, body := range []string{
		`{}`, `{"expected_freeze_event_id":20,"note":" "}`,
		`{"expected_freeze_event_id":0,"note":"已核查"}`,
		`{"expected_freeze_event_id":20,"note":"已核查","force":true}`,
		`{"expected_freeze_event_id":20,"note":"已核查"} {}`,
		`{"expected_freeze_event_id":20,"note":"` + strings.Repeat("a", 5000) + `"}`,
	} {
		t.Run(body[:min(len(body), 75)], func(t *testing.T) {
			unifiedChannelMock(t)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(middleware.ContextKeyUserID, uint(5))
				c.Set(middleware.ContextKeyUserRole, "admin")
			})
			router.POST("/users/:id/billing-account/unfreeze", UnfreezeUserBillingAccount)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("POST", "/users/1/billing-account/unfreeze", strings.NewReader(body)))
			if response.Code != 400 {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
		})
	}
}

func TestBillingAccountDetailDoesNotExposeMissingAccounts(t *testing.T) {
	mock := unifiedChannelMock(t)
	mock.ExpectQuery("SELECT a.id,a.user_id,a.status,a.currency_code").WithArgs(uint64(99)).WillReturnRows(sqlmock.NewRows([]string{"id", "user", "status", "currency", "posted", "held", "credit", "event", "reason", "at"}))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.ContextKeyUserID, uint(5))
		c.Set(middleware.ContextKeyUserRole, "admin")
	})
	router.GET("/users/:id/billing-account", GetUserBillingAccount)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/users/99/billing-account", nil))
	if response.Code != 404 || strings.Contains(response.Body.String(), "SELECT") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body)
	}
}
