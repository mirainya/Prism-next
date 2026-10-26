package admin

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/internal/update"
	"github.com/mirainya/Prism/pkg/logger"
	"github.com/mirainya/Prism/pkg/safeurl"
	"go.uber.org/zap"
)

// systemUpdateService only checks and stages release archives. Service
// replacement and database migrations remain deployment supervisor actions.
type systemUpdateService struct {
	httpClient *http.Client
	stagingDir string
	mu         sync.RWMutex
	status     systemUpdateStatus
	result     update.DownloadResult
}

var systemUpdater = systemUpdateService{httpClient: safeurl.NewClient(60 * time.Second)}

type systemUpdateStatus struct {
	State   string        `json:"state"`
	Source  string        `json:"source,omitempty"`
	Version string        `json:"version,omitempty"`
	Asset   *update.Asset `json:"asset,omitempty"`
	Size    int64         `json:"size,omitempty"`
	SHA256  string        `json:"sha256,omitempty"`
	Error   string        `json:"error,omitempty"`
}

// StagedSystemUpdate returns only an archive verified by the current process.
// The installer uses this result instead of accepting a filesystem path from HTTP.
func StagedSystemUpdate() (update.DownloadResult, bool) {
	return systemUpdater.staged()
}

// ClaimStagedSystemUpdate atomically reserves the reviewed release for the
// installer, excluding concurrent downloads and duplicate install requests.
func ClaimStagedSystemUpdate(expectedVersion string) (update.DownloadResult, bool) {
	systemUpdater.mu.Lock()
	defer systemUpdater.mu.Unlock()
	if systemUpdater.status.State != "staged" || systemUpdater.result.Version != expectedVersion {
		return update.DownloadResult{}, false
	}
	systemUpdater.status.State = "queued"
	systemUpdater.status.Error = ""
	return systemUpdater.result, true
}

func ReleaseSystemUpdateInstall(message string) {
	systemUpdater.mu.Lock()
	defer systemUpdater.mu.Unlock()
	if systemUpdater.status.State == "queued" || systemUpdater.status.State == "installing" {
		systemUpdater.status.State = "staged"
		systemUpdater.status.Error = message
	}
}

func GetSystemUpdateSources(c *gin.Context) {
	db, ok := systemUpdateDB(c)
	if !ok {
		return
	}
	sources, err := update.LoadSources(c.Request.Context(), db)
	if err != nil {
		systemUpdateError(c, err)
		return
	}
	resp.Success(c, sources)
}

func SaveSystemUpdateSources(c *gin.Context) {
	actor := uint64(currentAdminID(c))
	if actor == 0 {
		resp.ErrorMsg(c, http.StatusForbidden, 403, "需要管理员身份")
		return
	}
	var in struct {
		Sources          []update.SourceConfig `json:"sources"`
		ExpectedRevision uint64                `json:"expected_revision"`
	}
	if !unifiedChannelBody(c, &in) {
		return
	}
	// A missing/null list must not silently clear configured sources. An
	// explicit empty list is the operator's request to use GitHub alone.
	if in.Sources == nil {
		resp.ErrorMsg(c, http.StatusBadRequest, 400, "请提供更新源列表")
		return
	}
	db, ok := systemUpdateDB(c)
	if !ok {
		return
	}
	if err := update.SaveSources(c.Request.Context(), db, in.Sources, actor, in.ExpectedRevision); err != nil {
		systemUpdateError(c, err)
		return
	}
	sources, err := update.LoadSources(c.Request.Context(), db)
	if err != nil {
		systemUpdateError(c, err)
		return
	}
	resp.Success(c, sources)
}

func CheckSystemUpdate(c *gin.Context) {
	systemUpdater.check(c)
}

func DownloadSystemUpdate(c *gin.Context) {
	systemUpdater.download(c)
}

func GetSystemUpdateStatus(c *gin.Context) {
	resp.Success(c, systemInstallSnapshot())
}

func (s *systemUpdateService) check(c *gin.Context) {
	client, ok := s.client(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Minute)
	defer cancel()
	result, err := client.Check(ctx)
	if err != nil {
		systemUpdateError(c, err)
		return
	}
	resp.Success(c, gin.H{
		"current_version": result.CurrentVersion, "latest_version": result.LatestVersion,
		"update_available": result.UpdateAvailable, "source": result.Source, "manifest": result.Manifest,
	})
}

func (s *systemUpdateService) download(c *gin.Context) {
	if currentAdminID(c) == 0 {
		resp.ErrorMsg(c, http.StatusForbidden, 403, "需要管理员身份")
		return
	}
	if cfg, err := installedSupervisor(); err == nil {
		if status, err := update.ReadInstallStatus(cfg.InstallDir); err == nil && (status.State == "queued" || status.State == "installing" || status.State == "recovery_required") {
			resp.ErrorMsg(c, http.StatusConflict, 409, "已有更新正在安装或需要人工恢复")
			return
		}
	}
	// The staging location belongs to the installation, never to HTTP input.
	if c.Request.ContentLength != 0 {
		var in struct{}
		if !unifiedChannelBody(c, &in) {
			return
		}
	}
	client, ok := s.client(c)
	if !ok {
		return
	}
	targetDir := s.stagingDir
	if targetDir == "" {
		var err error
		targetDir, err = installedUpdateStagingDir()
		if err != nil {
			systemUpdateError(c, err)
			return
		}
	}
	if !s.startDownload(client, targetDir) {
		resp.ErrorMsg(c, http.StatusConflict, 409, "已有更新包正在下载")
		return
	}
	c.JSON(http.StatusAccepted, resp.Response{Code: 0, Message: "success", Data: s.snapshot()})
}

func (s *systemUpdateService) snapshot() systemUpdateStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.status.State == "" {
		return systemUpdateStatus{State: "idle"}
	}
	return s.status
}

func (s *systemUpdateService) staged() (update.DownloadResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.result, s.status.State == "staged"
}

func (s *systemUpdateService) startDownload(client *update.Client, targetDir string) bool {
	s.mu.Lock()
	if s.status.State == "downloading" || s.status.State == "queued" || s.status.State == "installing" {
		s.mu.Unlock()
		return false
	}
	s.status = systemUpdateStatus{State: "downloading"}
	s.result = update.DownloadResult{}
	s.mu.Unlock()
	go func() {
		// The transfer outlives its initiating HTTP request. The upper bound
		// prevents an unavailable source from holding the job indefinitely.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		result, err := client.Download(ctx, targetDir)
		s.mu.Lock()
		if err != nil {
			s.status = systemUpdateStatus{State: "failed", Error: systemUpdateMessage(err)}
			s.result = update.DownloadResult{}
		} else {
			asset := result.Asset
			s.status = systemUpdateStatus{
				State: "staged", Source: result.Source, Version: result.Version,
				Asset: &asset, Size: result.Size, SHA256: result.SHA256,
			}
			s.result = result
		}
		s.mu.Unlock()
		if err != nil && logger.L != nil {
			logger.Error("system update download failed", zap.Error(err))
		}
	}()
	return true
}

func (s *systemUpdateService) client(c *gin.Context) (*update.Client, bool) {
	db, ok := systemUpdateDB(c)
	if !ok {
		return nil, false
	}
	sources, err := update.LoadSources(c.Request.Context(), db)
	if err != nil {
		systemUpdateError(c, err)
		return nil, false
	}
	client, err := update.NewClient(update.Options{
		CurrentVersion: buildInfo.Load().(BuildInfo).Version,
		Sources:        update.Sources(sources.Sources), HTTPClient: s.httpClient,
	})
	if err != nil {
		systemUpdateError(c, err)
		return nil, false
	}
	return client, true
}

func systemUpdateDB(c *gin.Context) (*sql.DB, bool) {
	if !model.HasDB() {
		resp.ErrorMsg(c, http.StatusServiceUnavailable, 503, "数据库尚未就绪")
		return nil, false
	}
	db, err := model.DB().DB()
	if err != nil {
		systemUpdateError(c, err)
		return nil, false
	}
	return db, true
}

func installedUpdateStagingDir() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(executable), ".update", "staging"), nil
}

func systemUpdateError(c *gin.Context, err error) {
	if logger.L != nil {
		logger.Error("system update failed", zap.String("route", c.FullPath()), zap.Error(err))
	}
	switch {
	case errors.Is(err, update.ErrInvalidSource), errors.Is(err, update.ErrSourceVersionMissing):
		resp.ErrorMsg(c, http.StatusBadRequest, 400, "更新源名称、地址、优先级或配置版本无效")
	case errors.Is(err, update.ErrSourceConflict):
		resp.ErrorMsg(c, http.StatusConflict, 409, "更新源已被修改，请刷新后重试")
	case errors.Is(err, update.ErrNoUpdate):
		resp.ErrorMsg(c, http.StatusConflict, 409, "当前已经是最新版本")
	case errors.Is(err, update.ErrDestinationExists):
		resp.ErrorMsg(c, http.StatusConflict, 409, "暂存目录存在内容不同的同名更新包")
	case errors.Is(err, context.DeadlineExceeded):
		resp.ErrorMsg(c, http.StatusGatewayTimeout, 504, "获取更新超时，请重试")
	case errors.Is(err, update.ErrSourcesUnavailable):
		resp.ErrorMsg(c, http.StatusBadGateway, 502, "更新源不可用，请检查更新源配置")
	case errors.Is(err, update.ErrAssetUnavailable):
		resp.ErrorMsg(c, http.StatusBadGateway, 502, "更新包下载或校验失败，请重试")
	default:
		resp.ErrorMsg(c, http.StatusInternalServerError, 500, "读取或处理更新失败")
	}
}

func systemUpdateMessage(err error) string {
	switch {
	case errors.Is(err, update.ErrNoUpdate):
		return "当前已经是最新版本"
	case errors.Is(err, update.ErrDestinationExists):
		return "暂存目录存在内容不同的同名更新包"
	case errors.Is(err, context.DeadlineExceeded):
		return "下载更新超时，请重试"
	case errors.Is(err, update.ErrSourcesUnavailable):
		return "更新源不可用，请检查更新源配置"
	default:
		return "更新包下载或校验失败，请重试"
	}
}
