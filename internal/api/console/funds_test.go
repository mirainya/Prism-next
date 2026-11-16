package console

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/middleware"
)

func TestBudgetAdjustmentRequiresRequestIDAndNote(t *testing.T) {
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(middleware.ContextKeyUserID, uint(1)) })
	router.POST("/tokens/:id/budget-adjustments", RechargeToken)
	for _, body := range []string{
		`{"amount":"2"}`,
		`{"amount":"2","request_id":"invalid","note":"test"}`,
		`{"amount":"2","request_id":"11111111-1111-4111-8111-111111111111"}`,
		`{"amount":"2","request_id":"11111111-1111-4111-8111-111111111111","note":"  "}`,
	} {
		request := httptest.NewRequest("POST", "/tokens/1/budget-adjustments", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != 400 {
			t.Fatalf("body=%s status=%d response=%s", body, response.Code, response.Body)
		}
	}
}

func TestFundsHistoryRejectsCrossUserQueriesBeforeReadingDatabase(t *testing.T) {
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(middleware.ContextKeyUserID, uint(1)) })
	router.GET("/funds", ListFundsEvents)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/funds?user_id=2", nil))
	if response.Code != 403 {
		t.Fatalf("status=%d response=%s", response.Code, response.Body)
	}
}
