//go:build linux

package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mirainya/Prism/pkg/config"
)

// RunSupervisor is invoked only by the root-owned update service. Candidate
// binaries run as the application user, never as root. No HTTP command is executed.
func RunSupervisor(ctx context.Context) (err error) {
	if os.Geteuid() != 0 {
		return errors.New("update supervisor must run as root")
	}
	info, err := os.Lstat(SupervisorConfigPath)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 || info.Mode().Perm()&0022 != 0 || !info.Mode().IsRegular() {
		return errors.New("updater config must be a root-owned regular file, not writable by other users")
	}
	cfg, err := ReadSupervisorConfig()
	if err != nil {
		return err
	}
	var request InstallRequest
	data, err := os.ReadFile(filepath.Join(cfg.InstallDir, ".update", "request.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &request); err != nil {
		return err
	}
	if request.ActorID == 0 || time.Since(request.RequestedAt) > 10*time.Minute || time.Until(request.RequestedAt) > time.Minute {
		return errors.New("invalid or expired installation request")
	}
	status := InstallStatus{State: "installing", Version: request.Release.Version}
	serviceStopped, migrationsStarted := false, false
	writeStatus := func() error {
		status.UpdatedAt = time.Now().UTC()
		return WriteInstallJSON(cfg.InstallDir, "install-status.json", status)
	}
	if err := writeStatus(); err != nil {
		return err
	}
	defer func() {
		if err != nil && status.State != "rolled_back" {
			if serviceStopped {
				status.State = "recovery_required"
				status.Message = "安装失败，请检查 prism-update 服务日志和备份"
			} else {
				status.State = "failed"
				status.Message = "安装前检查失败，当前服务未停止，请查看 prism-update 服务日志"
			}
			_ = writeStatus()
		}
	}()
	account, err := user.Lookup(cfg.ServiceUser)
	if err != nil {
		return err
	}
	uid, _ := strconv.Atoi(account.Uid)
	gid, _ := strconv.Atoi(account.Gid)
	installed := filepath.Join(cfg.InstallDir, "prism")
	execInfo, err := os.Lstat(installed)
	if err != nil || !execInfo.Mode().IsRegular() {
		return errors.New("installed executable must be a regular file")
	}
	// Check that the privileged supervisor is controlling the intended deployment.
	for property, expected := range map[string]string{"User": cfg.ServiceUser, "WorkingDirectory": cfg.InstallDir} {
		value, e := exec.CommandContext(ctx, "/usr/bin/systemctl", "show", cfg.Service, "--property="+property, "--value").Output()
		if e != nil || strings.TrimSpace(string(value)) != expected {
			return fmt.Errorf("deployment service %s does not match updater configuration", property)
		}
	}
	startCommand, err := exec.CommandContext(ctx, "/usr/bin/systemctl", "show", cfg.Service, "--property=ExecStart", "--value").Output()
	if err != nil || !strings.Contains(string(startCommand), "path="+installed+" ;") {
		return errors.New("service executable does not match updater installation")
	}
	if filepath.Clean(request.Release.Path) != filepath.Join(cfg.InstallDir, ".update", "staging", request.Release.Asset.Name) || filepath.Base(request.Release.Asset.Name) != request.Release.Asset.Name {
		return errors.New("release is outside the staging directory")
	}
	if err := os.MkdirAll("/var/lib/prism-updater", 0755); err != nil {
		return err
	}
	parent, err := os.Lstat("/var/lib/prism-updater")
	if err != nil {
		return err
	}
	parentStat, ok := parent.Sys().(*syscall.Stat_t)
	if !ok || !parent.IsDir() || parentStat.Uid != 0 || parent.Mode().Perm()&0022 != 0 {
		return errors.New("updater workspace must be a root-owned directory, not writable by other users")
	}
	work, err := os.MkdirTemp("/var/lib/prism-updater", "job-")
	if err != nil {
		return err
	}
	if err := os.Chmod(work, 0755); err != nil {
		return err
	}
	// Copy out of the application-writable directory before verification/extraction.
	archive := filepath.Join(work, "release.tar.gz")
	if err := copyInstallFile(request.Release.Path, archive, 0600); err != nil {
		return err
	}
	request.Release.Path = archive
	candidate, err := ExtractExecutable(request.Release, work)
	if err != nil {
		return err
	}
	if err := os.Chmod(candidate, 0755); err != nil {
		return err
	}
	runCandidate := func(args ...string) ([]byte, error) {
		command := exec.CommandContext(ctx, "/usr/sbin/runuser", append([]string{"-u", cfg.ServiceUser, "--", candidate}, args...)...)
		command.Dir = cfg.InstallDir
		command.Stderr = os.Stderr
		return command.Output()
	}
	version, err := runCandidate("version")
	if err != nil || !strings.HasPrefix(string(version), "Prism "+request.Release.Version+" (") {
		return errors.New("candidate version mismatch")
	}
	planData, err := runCandidate("migrate", "plan")
	if err != nil {
		return err
	}
	var plan MigrationPlan
	if err := json.Unmarshal(planData, &plan); err != nil {
		return err
	}
	if plan.Digest != request.PlanDigest || NewMigrationPlan(plan.Migrations).Digest != plan.Digest {
		return errors.New("migration plan changed; review again in the console")
	}
	backup := filepath.Join(work, "backup")
	if err := os.Mkdir(backup, 0700); err != nil {
		return err
	}
	if err := copyInstallFile(installed, filepath.Join(backup, "prism"), execInfo.Mode().Perm()); err != nil {
		return err
	}
	if err := copyInstallFile(filepath.Join(cfg.InstallDir, "configs", "config.yaml"), filepath.Join(backup, "config.yaml"), 0600); err != nil {
		return err
	}
	serviceStopped = true
	if err := exec.CommandContext(ctx, "/usr/bin/systemctl", "stop", cfg.Service).Run(); err != nil {
		return err
	}
	// Once migrations start, an old binary may not understand the new schema.
	// Never claim rollback by restoring only the executable after a schema change.
	defer func() {
		if err == nil {
			return
		}
		if migrationsStarted {
			cleanup, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			_ = exec.CommandContext(cleanup, "/usr/bin/systemctl", "stop", cfg.Service).Run()
			return
		}
		recovery, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		restored := filepath.Join(cfg.InstallDir, ".prism-restore")
		defer os.Remove(restored)
		if copyInstallFile(filepath.Join(backup, "prism"), restored, execInfo.Mode().Perm()) == nil && os.Chown(restored, uid, gid) == nil && os.Rename(restored, installed) == nil && exec.CommandContext(recovery, "/usr/bin/systemctl", "start", cfg.Service).Run() == nil && waitInstallHealth(recovery, cfg.HealthURL) == nil {
			status.State = "rolled_back"
			status.Message = "安装失败，已恢复旧程序"
			_ = writeStatus()
		}
	}()
	if len(plan.Migrations) > 0 {
		if err := backupInstallDatabase(ctx, cfg.InstallDir, backup); err != nil {
			return err
		}
		migrationsStarted = true
		if _, err := runCandidate("migrate", "up"); err != nil {
			return fmt.Errorf("apply reviewed migrations: %w", err)
		}
	}
	// Atomic rename avoids truncating either the old or running executable.
	next := filepath.Join(cfg.InstallDir, ".prism-update-new")
	defer os.Remove(next)
	if err := copyInstallFile(candidate, next, execInfo.Mode().Perm()); err != nil {
		return err
	}
	if err := os.Chown(next, uid, gid); err != nil {
		return err
	}
	if err := os.Rename(next, installed); err != nil {
		return err
	}
	if err := exec.CommandContext(ctx, "/usr/bin/systemctl", "start", cfg.Service).Run(); err != nil {
		return err
	}
	healthCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if err := waitInstallHealth(healthCtx, cfg.HealthURL); err != nil {
		return err
	}
	pidOutput, err := exec.CommandContext(ctx, "/usr/bin/systemctl", "show", cfg.Service, "--property=MainPID", "--value").Output()
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidOutput)))
	if err != nil || pid <= 0 {
		return errors.New("updated service has no main process")
	}
	processExecutable, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil || processExecutable != installed {
		return errors.New("health check did not belong to the updated executable")
	}
	status.State = "completed"
	status.Message = "新版本已启动，健康检查通过"
	return writeStatus()
}

func copyInstallFile(source, target string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(in, DefaultDownloadMaxSize+1))
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	return err
}

func waitInstallHealth(ctx context.Context, rawURL string) error {
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	for {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		response, err := client.Do(request)
		if err == nil {
			var health struct {
				Status  string `json:"status"`
				DB      bool   `json:"db"`
				Redis   bool   `json:"redis"`
				Gateway bool   `json:"gateway"`
			}
			decodeErr := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&health)
			response.Body.Close()
			if response.StatusCode == 200 && decodeErr == nil && health.Status == "ok" && health.DB && health.Redis && health.Gateway {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func backupInstallDatabase(ctx context.Context, root, directory string) error {
	if err := config.Load(filepath.Join(root, "configs", "config.yaml")); err != nil {
		return err
	}
	db := config.Get().Database
	quote := func(value string) string {
		return "\"" + strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\r", "\\r").Replace(value) + "\""
	}
	defaults := filepath.Join(directory, "mysql.cnf")
	content := fmt.Sprintf("[client]\nhost=%s\nport=%d\nuser=%s\npassword=%s\n", quote(db.Host), db.Port, quote(db.User), quote(db.Password))
	if err := os.WriteFile(defaults, []byte(content), 0600); err != nil {
		return err
	}
	defer os.Remove(defaults)
	file, err := os.OpenFile(filepath.Join(directory, "database.sql"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	command := exec.CommandContext(ctx, "/usr/bin/mysqldump", "--defaults-extra-file="+defaults, "--single-transaction", "--no-tablespaces", "--set-gtid-purged=OFF", "--routines", "--events", "--databases", db.DBName)
	command.Stdout = file
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("database backup failed: %w", err)
	}
	return file.Sync()
}
