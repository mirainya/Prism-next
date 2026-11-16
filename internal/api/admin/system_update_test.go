package admin

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/update"
)

func TestInstallStatusReleasesFailedPreflightReservation(t *testing.T) {
	directory := t.TempDir()
	status := update.InstallStatus{State: "failed", Version: "2.0.14", Message: "安装服务启动失败", UpdatedAt: time.Now()}
	if err := update.WriteInstallJSON(directory, "install-status.json", status); err != nil {
		t.Fatal(err)
	}
	service := systemUpdateService{status: systemUpdateStatus{State: "queued"}}
	data := service.mergeInstallStatus(gin.H{"state": "queued"}, directory)
	if data["state"] != "failed" || data["error"] != status.Message || service.snapshot().State != "failed" {
		t.Fatalf("persisted failure hidden by queued state: %v", data)
	}
}

func TestInstallStatusShowsStalledServiceAsRecoveryRequired(t *testing.T) {
	directory := t.TempDir()
	status := update.InstallStatus{State: "installing", Version: "2.0.14", UpdatedAt: time.Now().Add(-36 * time.Minute)}
	if err := update.WriteInstallJSON(directory, "install-status.json", status); err != nil {
		t.Fatal(err)
	}
	service := systemUpdateService{status: systemUpdateStatus{State: "queued"}}
	data := service.mergeInstallStatus(gin.H{"state": "queued"}, directory)
	if data["state"] != "recovery_required" {
		t.Fatalf("stalled install remained active: %v", data)
	}
}

func TestInstallMustBeExplicitlyConfirmed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/install", func(c *gin.Context) { c.Set("user_id", uint(1)); InstallSystemUpdate(c) })
	request := httptest.NewRequest("POST", "/install", strings.NewReader(`{"version":"2.0.14","plan_digest":"digest","confirmed":false}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 400 {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestInstallRejectsMissingAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/install", InstallSystemUpdate)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("POST", "/install", strings.NewReader(`{}`)))
	if response.Code != 403 {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestStatusUsesEnvelopeAndDoesNotExposeStagingPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/status", GetSystemUpdateStatus)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/status", nil))
	var result struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || result.Code != 0 || result.Data["state"] == nil {
		t.Fatalf("response=%s", response.Body)
	}
	if _, exists := result.Data["path"]; exists {
		t.Fatal("status exposed local path")
	}
}

func TestClaimInstallLocksReviewedVersionAgainstConcurrentDownloads(t *testing.T) {
	systemUpdater.mu.Lock()
	previousStatus, previousResult := systemUpdater.status, systemUpdater.result
	systemUpdater.status = systemUpdateStatus{State: "staged", Version: "2.0.14"}
	systemUpdater.result = update.DownloadResult{Version: "2.0.14"}
	systemUpdater.mu.Unlock()
	t.Cleanup(func() {
		systemUpdater.mu.Lock()
		systemUpdater.status, systemUpdater.result = previousStatus, previousResult
		systemUpdater.mu.Unlock()
	})
	if _, ok := ClaimStagedSystemUpdate("2.0.15"); ok {
		t.Fatal("incorrect version accepted")
	}
	if _, ok := ClaimStagedSystemUpdate("2.0.14"); !ok {
		t.Fatal("reviewed version not claimed")
	}
	if _, ok := ClaimStagedSystemUpdate("2.0.14"); ok {
		t.Fatal("duplicate install accepted")
	}
	if systemUpdater.startDownload(nil, t.TempDir()) {
		t.Fatal("download accepted during installation")
	}
	ReleaseSystemUpdateInstall("start failed")
	if systemUpdater.snapshot().State != "staged" {
		t.Fatal("failed service dispatch did not restore staged state")
	}
}
