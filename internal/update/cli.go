package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// RunCLI handles the update subcommands without touching the database or
// process state. The caller decides how to turn the returned error into an
// exit code.
func RunCLI(ctx context.Context, args []string, currentVersion string, out io.Writer) error {
	if len(args) < 2 || args[0] != "update" {
		return fmt.Errorf("usage: prism update <check|download [staging-dir]>")
	}
	if out == nil {
		out = io.Discard
	}
	client, err := NewClient(Options{CurrentVersion: currentVersion})
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(args[1])) {
	case "check":
		if len(args) != 2 {
			return fmt.Errorf("usage: prism update check")
		}
		result, err := client.Check(ctx)
		if err != nil {
			return err
		}
		return writeJSON(out, struct {
			CurrentVersion  string   `json:"current_version"`
			LatestVersion   string   `json:"latest_version"`
			UpdateAvailable bool     `json:"update_available"`
			Source          string   `json:"source"`
			Manifest        Manifest `json:"manifest"`
		}{result.CurrentVersion, result.LatestVersion, result.UpdateAvailable, result.Source, result.Manifest})
	case "download":
		if len(args) > 3 {
			return fmt.Errorf("usage: prism update download [staging-dir]")
		}
		targetDir := ""
		if len(args) == 3 {
			targetDir = strings.TrimSpace(args[2])
		}
		result, err := client.Download(ctx, targetDir)
		if err != nil {
			return err
		}
		return writeJSON(out, result)
	default:
		return fmt.Errorf("usage: prism update <check|download [staging-dir]>")
	}
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write update result: %w", err)
	}
	return nil
}

// IsNoUpdate reports whether an update command found no newer release.
func IsNoUpdate(err error) bool {
	return errors.Is(err, ErrNoUpdate)
}
