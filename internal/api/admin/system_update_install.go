package admin

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/update"
)

var installReview struct {
	sync.Mutex
	Version string
	SHA256  string
	Digest  string
}

func installedSupervisor() (update.SupervisorConfig, error) {
	cfg, err := update.ReadSupervisorConfig()
	if err != nil {
		return cfg, err
	}
	executable, err := os.Executable()
	if err != nil {
		return cfg, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil || executable != filepath.Join(cfg.InstallDir, "prism") {
		return cfg, errors.New("updater configuration does not match running installation")
	}
	return cfg, nil
}

func PrepareSystemUpdate(c *gin.Context) {
	if currentAdminID(c) == 0 {
		resp.ErrorMsg(c, 403, 403, "需要管理员身份")
		return
	}
	cfg, err := installedSupervisor()
	if err != nil {
		resp.ErrorMsg(c, 501, 501, "服务器尚未配置独立更新服务")
		return
	}
	result, ok := StagedSystemUpdate()
	if !ok {
		resp.ErrorMsg(c, 409, 409, "请先下载并校验更新包")
		return
	}
	directory, err := os.MkdirTemp(filepath.Join(cfg.InstallDir, ".update"), "review-")
	if err != nil {
		systemUpdateError(c, err)
		return
	}
	defer os.RemoveAll(directory)
	candidate, err := update.ExtractExecutable(result, directory)
	if err != nil {
		systemUpdateError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 90*time.Second)
	defer cancel()
	plan, err := update.InspectCandidate(ctx, candidate, cfg.InstallDir, result.Version)
	if err != nil {
		resp.ErrorMsg(c, 422, 422, "无法读取该版本的迁移计划，请检查更新服务日志或先安装支持运维台更新的版本")
		return
	}
	installReview.Lock()
	installReview.Version, installReview.SHA256, installReview.Digest = result.Version, result.SHA256, plan.Digest
	installReview.Unlock()
	resp.Success(c, gin.H{"version": result.Version, "plan": plan})
}

func InstallSystemUpdate(c *gin.Context) {
	actor := uint64(currentAdminID(c))
	if actor == 0 {
		resp.ErrorMsg(c, 403, 403, "需要管理员身份")
		return
	}
	var input struct {
		Version    string `json:"version"`
		PlanDigest string `json:"plan_digest"`
		Confirmed  bool   `json:"confirmed"`
	}
	if !unifiedChannelBody(c, &input) {
		return
	}
	if !input.Confirmed {
		resp.ErrorMsg(c, 400, 400, "请先确认展示的迁移与服务重启")
		return
	}
	cfg, err := installedSupervisor()
	if err != nil {
		resp.ErrorMsg(c, 501, 501, "服务器尚未配置独立更新服务")
		return
	}
	installReview.Lock()
	defer installReview.Unlock()
	if input.Version != installReview.Version || input.PlanDigest == "" || input.PlanDigest != installReview.Digest {
		resp.ErrorMsg(c, 409, 409, "更新计划已变化，请重新预览")
		return
	}
	result, ok := ClaimStagedSystemUpdate(input.Version)
	if !ok {
		resp.ErrorMsg(c, 409, 409, "更新包不可用或正在安装")
		return
	}
	if result.SHA256 != installReview.SHA256 {
		ReleaseSystemUpdateInstall("更新包已变化")
		resp.ErrorMsg(c, 409, 409, "更新包已变化，请重新预览")
		return
	}
	request := update.InstallRequest{Release: result, PlanDigest: input.PlanDigest, ActorID: actor, RequestedAt: time.Now().UTC()}
	status := update.InstallStatus{State: "queued", Version: input.Version, Message: "等待独立更新服务", UpdatedAt: time.Now().UTC()}
	if err = update.WriteInstallJSON(cfg.InstallDir, "request.json", request); err == nil {
		err = update.WriteInstallJSON(cfg.InstallDir, "install-status.json", status)
	}
	if err != nil {
		ReleaseSystemUpdateInstall("无法保存更新请求")
		systemUpdateError(c, err)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	// Only this fixed service may be started. HTTP never supplies executable,
	// command, destination path, service name or privileged shell arguments.
	err = exec.CommandContext(ctx, "/usr/bin/sudo", "-n", "/usr/bin/systemctl", "start", "--no-block", "prism-update.service").Run()
	if err != nil {
		ReleaseSystemUpdateInstall("更新服务启动失败")
		status.State, status.Message = "failed", "独立更新服务启动失败，请检查服务器权限"
		_ = update.WriteInstallJSON(cfg.InstallDir, "install-status.json", status)
		resp.ErrorMsg(c, 503, 503, status.Message)
		return
	}
	installReview.Digest = ""
	c.JSON(http.StatusAccepted, resp.Response{Code: 0, Message: "success", Data: status})
}

func systemInstallSnapshot() gin.H {
	current := systemUpdater.snapshot()
	data := gin.H{"state": current.State, "version": current.Version, "source": current.Source, "asset": current.Asset, "size": current.Size, "sha256": current.SHA256, "error": current.Error, "install_available": false}
	cfg, err := installedSupervisor()
	if err != nil {
		data["install_reason"] = "服务器尚未配置独立更新服务"
		return data
	}
	data["install_available"] = true
	return systemUpdater.mergeInstallStatus(data, cfg.InstallDir)
}

func (s *systemUpdateService) mergeInstallStatus(data gin.H, installDir string) gin.H {
	current := s.snapshot()
	if current.State == "downloading" || current.State == "staged" || current.State == "failed" {
		return data
	}
	if status, err := update.ReadInstallStatus(installDir); err == nil {
		data["state"], data["version"], data["message"], data["updated_at"] = status.State, status.Version, status.Message, status.UpdatedAt
		if status.State == "failed" {
			data["error"] = status.Message
		}
		// Preflight failure does not restart Prism. Release its in-memory
		// installation reservation when the independent service has finished.
		s.mu.Lock()
		if s.status.State == "queued" || s.status.State == "installing" {
			s.status.State = status.State
			s.status.Error = status.Message
		}
		s.mu.Unlock()
	}
	return data
}
