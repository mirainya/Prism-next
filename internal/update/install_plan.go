package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mirainya/Prism/internal/migrate"
)

// MigrationPlan is produced by the candidate binary against the installed database.
// Its digest binds the operator's confirmation to the exact SQL and history.
type MigrationPlan struct {
	Migrations []migrate.Migration `json:"migrations"`
	Digest     string              `json:"digest"`
}

func NewMigrationPlan(migrations []migrate.Migration) MigrationPlan {
	if migrations == nil {
		migrations = []migrate.Migration{}
	}
	data, _ := json.Marshal(migrations)
	digest := sha256.Sum256(data)
	return MigrationPlan{Migrations: migrations, Digest: hex.EncodeToString(digest[:])}
}

// ExtractExecutable writes only the expected regular executable. Archive paths,
// links, duplicates and excessive expansion are rejected; configs are never extracted.
func ExtractExecutable(result DownloadResult, directory string) (string, error) {
	if _, err := parseSemVersion(result.Version); err != nil {
		return "", err
	}
	if result.Asset.Platform != "linux" || result.Asset.Arch != "amd64" {
		return "", errors.New("online install requires Linux AMD64")
	}
	file, err := os.Open(result.Path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest, size, err := fileDigest(io.LimitReader(file, DefaultDownloadMaxSize+1))
	if err != nil || size != result.Size || digest != result.SHA256 {
		return "", errors.New("staged archive verification failed")
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer compressed.Close()
	archive := tar.NewReader(io.LimitReader(compressed, 2*DefaultDownloadMaxSize+1))
	root := "prism-v" + result.Version + "-linux-amd64"
	expected := root + "/prism"
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	output := filepath.Join(directory, "prism")
	found := false
	var expanded int64
	for count := 0; ; count++ {
		entry, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		if count > 4096 {
			return "", errors.New("too many archive entries")
		}
		name := strings.TrimSuffix(entry.Name, "/")
		if name != root && !strings.HasPrefix(name, root+"/") || strings.Contains(name, "\\") || filepath.ToSlash(filepath.Clean(name)) != name {
			return "", errors.New("invalid archive path")
		}
		if entry.Typeflag != tar.TypeReg && entry.Typeflag != tar.TypeDir {
			return "", errors.New("archive links and special files are not allowed")
		}
		if entry.Size < 0 {
			return "", errors.New("invalid archive size")
		}
		expanded += entry.Size
		if expanded > 2*DefaultDownloadMaxSize {
			return "", errors.New("archive expansion exceeds limit")
		}
		if name != expected {
			continue
		}
		if found || entry.Typeflag != tar.TypeReg || entry.Size == 0 || entry.Size > DefaultDownloadMaxSize {
			return "", errors.New("invalid executable entry")
		}
		found = true
		target, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if err != nil {
			return "", err
		}
		written, copyErr := io.Copy(target, archive)
		closeErr := target.Close()
		if copyErr != nil || closeErr != nil || written != entry.Size {
			_ = os.Remove(output)
			return "", errors.New("extract executable failed")
		}
	}
	if !found {
		return "", errors.New("release executable is missing")
	}
	return output, nil
}

func InspectCandidate(ctx context.Context, executable, installedDir, expectedVersion string) (MigrationPlan, error) {
	command := exec.CommandContext(ctx, executable, "version")
	command.Dir = installedDir
	output, err := command.Output()
	if err != nil || !strings.HasPrefix(string(output), "Prism "+expectedVersion+" (") {
		return MigrationPlan{}, errors.New("candidate version does not match release")
	}
	command = exec.CommandContext(ctx, executable, "migrate", "plan")
	command.Dir = installedDir
	output, err = command.Output()
	if err != nil {
		return MigrationPlan{}, fmt.Errorf("inspect candidate migrations: %w", err)
	}
	var plan MigrationPlan
	if err := json.Unmarshal(output, &plan); err != nil {
		return MigrationPlan{}, errors.New("candidate does not provide a migration plan; install a console-update-capable release first")
	}
	if NewMigrationPlan(plan.Migrations).Digest != plan.Digest {
		return MigrationPlan{}, errors.New("invalid candidate migration plan")
	}
	return plan, nil
}
