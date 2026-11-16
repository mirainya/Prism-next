package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestCapabilityRecoveryRequiresAuthenticatedOperator(t *testing.T) {
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/recover", strings.NewReader(`{}`))
	RecoverUnifiedCapabilityTask(ctx)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestTaskRecoveryBindsResourceKind(t *testing.T) {
	for _, resourceKind := range []string{"video_task", "capability_task"} {
		t.Run(resourceKind, func(t *testing.T) {
			mock := unifiedChannelMock(t)
			mock.ExpectQuery("SELECT x.id FROM gw_api_resources r").
				WithArgs("task-one", resourceKind).
				WillReturnRows(sqlmock.NewRows([]string{"id"}))
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			ctx.Set("user_id", uint(1))
			ctx.Params = gin.Params{{Key: "id", Value: "task-one"}}
			ctx.Request = httptest.NewRequest(http.MethodPost, "/recover", strings.NewReader(`{}`))
			ctx.Request.Header.Set("Content-Type", "application/json")
			if resourceKind == "video_task" {
				RecoverUnifiedVideoTask(ctx)
			} else {
				RecoverUnifiedCapabilityTask(ctx)
			}
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body)
			}
		})
	}
}
