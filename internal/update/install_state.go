package update

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"
)

const SupervisorConfigPath = "/etc/prism/updater.json"

// SupervisorConfig is deployment-owned, not editable through the web API.
// Update sources are deliberately absent: they are managed in the database.
type SupervisorConfig struct {
	InstallDir  string `json:"install_dir"`
	Service     string `json:"service"`
	ServiceUser string `json:"service_user"`
	HealthURL   string `json:"health_url"`
}

type InstallRequest struct {
	Release     DownloadResult `json:"release"`
	PlanDigest  string         `json:"plan_digest"`
	ActorID     uint64         `json:"actor_id"`
	RequestedAt time.Time      `json:"requested_at"`
}

type InstallStatus struct {
	State     string    `json:"state"`
	Version   string    `json:"version,omitempty"`
	Message   string    `json:"message,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

func ReadSupervisorConfig() (SupervisorConfig, error) {
	var config SupervisorConfig
	if runtime.GOOS != "linux" {
		return config, errors.New("online installation requires Linux systemd")
	}
	data, err := os.ReadFile(SupervisorConfigPath)
	if err != nil {
		return config, err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, err
	}
	if !filepath.IsAbs(config.InstallDir) || filepath.Clean(config.InstallDir) != config.InstallDir || config.InstallDir == "/" ||
		!regexp.MustCompile(`^[a-zA-Z0-9_-]+\.service$`).MatchString(config.Service) || config.Service == "prism-update.service" ||
		!regexp.MustCompile(`^[a-z_][a-z0-9_-]*$`).MatchString(config.ServiceUser) || config.ServiceUser == "root" ||
		!regexp.MustCompile(`^http://127\.0\.0\.1:[0-9]{1,5}/health$`).MatchString(config.HealthURL) {
		return config, errors.New("invalid updater deployment configuration")
	}
	return config, nil
}

func WriteInstallJSON(root, name string, value any) error {
	if name != "request.json" && name != "install-status.json" {
		return errors.New("invalid install state file")
	}
	directory := filepath.Join(root, ".update")
	if err := os.MkdirAll(directory, 0750); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := json.NewEncoder(file).Encode(value); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(0644); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(directory, name))
}

func ReadInstallStatus(root string) (InstallStatus, error) {
	var status InstallStatus
	data, err := os.ReadFile(filepath.Join(root, ".update", "install-status.json"))
	if err != nil {
		return status, err
	}
	if err := json.Unmarshal(data, &status); err != nil {
		return status, err
	}
	if (status.State == "queued" || status.State == "installing") && time.Since(status.UpdatedAt) > 35*time.Minute {
		status.State = "recovery_required"
		status.Message = "更新服务未完成，请检查服务器更新日志"
	}
	return status, nil
}
