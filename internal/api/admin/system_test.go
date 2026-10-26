package admin

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSystemVersionReturnsBuildIdentityWithoutDatabase(t *testing.T) {
	previous := buildInfo.Load().(BuildInfo)
	t.Cleanup(func() { buildInfo.Store(previous) })
	SetBuildInfo(BuildInfo{Version: "2.0.13", BuildTime: "2026-10-13T00:00:00Z", SourceRevision: "abc"})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/admin/system/version", SystemVersion)
	request := httptest.NewRequest("GET", "/api/admin/system/version", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("status=%d, want 200", response.Code)
	}
	var got struct {
		Code int       `json:"code"`
		Data BuildInfo `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got.Code != 0 || got.Data != (BuildInfo{Version: "2.0.13", BuildTime: "2026-10-13T00:00:00Z", SourceRevision: "abc"}) {
		t.Fatalf("body=%+v", got)
	}
}
