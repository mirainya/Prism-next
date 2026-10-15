package admin

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverUnifiedVideoTaskRequiresAdmin(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/video/tasks/video/recover", strings.NewReader(`{"provider_task_id":"task_123"}`))
	unifiedVideoResolutionRouter(0).ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestRecoverUnifiedVideoTaskRejectsInvalidIdentity(t *testing.T) {
	for _, body := range []string{`{"provider_task_id":"task\n123"}`, `{"provider_task_id":"task\u0000"}`} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/admin/video/tasks/video/recover", strings.NewReader(body))
		unifiedVideoResolutionRouter(7).ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", response.Code, body)
		}
	}
}

func TestRecoverUnifiedVideoTaskReportsMissingTask(t *testing.T) {
	mock := unifiedChannelMock(t)
	mock.ExpectQuery(`SELECT x.id FROM gw_api_resources`).WithArgs("missing").WillReturnError(sql.ErrNoRows)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/video/tasks/missing/recover", strings.NewReader(`{"provider_task_id":"task_123"}`))
	unifiedVideoResolutionRouter(7).ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestContinueWithoutManualIdentityReachesTaskLookup(t *testing.T) {
	mock := unifiedChannelMock(t)
	mock.ExpectQuery(`SELECT x.id FROM gw_api_resources`).WithArgs("missing").WillReturnError(sql.ErrNoRows)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/video/tasks/missing/recover", strings.NewReader(`{}`))
	unifiedVideoResolutionRouter(7).ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "视频任务不存在") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
