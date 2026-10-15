package migrate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
)

// Retired SQL is archived privately. Its public compatibility manifest contains
// hashes rather than SQL, migration names, or provider-specific configuration.
// Validation does not adopt a baseline: the caller must also validate schema
// equivalence under the migration lock before recording the new baseline.
type retiredHistory struct {
	Schema                    int                       `json:"schema"`
	BaselineVersion           string                    `json:"baseline_version"`
	ThroughVersion            string                    `json:"through_version"`
	Migrations                []retiredMigration        `json:"migrations"`
	CompatibleThroughVersions []string                  `json:"compatible_through_versions,omitempty"`
	ForwardMigrations         []retiredForwardMigration `json:"forward_migrations,omitempty"`
}

type retiredForwardMigration struct {
	RetiredVersion  string `json:"retired_version"`
	ForwardVersion  string `json:"forward_version"`
	ForwardChecksum string `json:"forward_checksum"`
}

type retiredMigration struct {
	Version      string `json:"version"`
	Checksum     string `json:"checksum"`
	NameChecksum string `json:"name_checksum"`
}

var historyVersionPattern = regexp.MustCompile(`^\d{8}_\d{6}$`)
var historyChecksumPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func parseRetiredHistory(content []byte) (retiredHistory, error) {
	var history retiredHistory
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&history); err != nil {
		return retiredHistory{}, fmt.Errorf("decode retired migration history: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return retiredHistory{}, fmt.Errorf("retired migration history has trailing content")
	}
	if history.Schema != 1 || !historyVersionPattern.MatchString(history.BaselineVersion) ||
		!historyVersionPattern.MatchString(history.ThroughVersion) ||
		history.BaselineVersion <= history.ThroughVersion || len(history.Migrations) == 0 {
		return retiredHistory{}, fmt.Errorf("invalid retired migration history metadata")
	}
	previous := ""
	for _, migration := range history.Migrations {
		if !historyVersionPattern.MatchString(migration.Version) || migration.Version <= previous ||
			!historyChecksumPattern.MatchString(migration.Checksum) ||
			!historyChecksumPattern.MatchString(migration.NameChecksum) {
			return retiredHistory{}, fmt.Errorf("invalid retired migration history entry %s", migration.Version)
		}
		previous = migration.Version
	}
	if history.Migrations[0].Version != "20260718_150000" || previous != history.ThroughVersion {
		return retiredHistory{}, fmt.Errorf("retired migration history does not cover the expected range")
	}
	known := make(map[string]bool, len(history.Migrations))
	for _, migration := range history.Migrations {
		known[migration.Version] = true
	}
	seen := make(map[string]bool)
	for _, version := range history.CompatibleThroughVersions {
		if !known[version] || seen[version] || version >= history.ThroughVersion {
			return retiredHistory{}, fmt.Errorf("invalid compatible history checkpoint")
		}
		seen[version] = true
	}
	seen = make(map[string]bool)
	for _, migration := range history.ForwardMigrations {
		if !known[migration.RetiredVersion] || !historyVersionPattern.MatchString(migration.ForwardVersion) ||
			migration.ForwardVersion <= history.BaselineVersion || seen[migration.ForwardVersion] ||
			!historyChecksumPattern.MatchString(migration.ForwardChecksum) {
			return retiredHistory{}, fmt.Errorf("invalid retired migration forward mapping")
		}
		seen[migration.ForwardVersion] = true
	}
	return history, nil
}

// validateRetiredHistory requires the entire known history, not just its last
// version. Dirty, modified, missing, repeated, and unknown records all fail.
// A successfully registered new baseline is handled by a separate validation
// path; it must not be silently accepted as an extra retired record here.
func validateRetiredHistory(history retiredHistory, applied []AppliedMigration) error {
	expected := history.Migrations
	latest := ""
	for _, record := range applied {
		if record.Version > latest {
			latest = record.Version
		}
	}
	for _, checkpoint := range history.CompatibleThroughVersions {
		if latest == checkpoint {
			for index, migration := range history.Migrations {
				if migration.Version == checkpoint {
					expected = history.Migrations[:index+1]
					break
				}
			}
		}
	}
	if len(applied) != len(expected) {
		return fmt.Errorf("retired migration history is incomplete or contains unknown versions")
	}
	byVersion := make(map[string]AppliedMigration, len(applied))
	for _, record := range applied {
		if _, duplicate := byVersion[record.Version]; duplicate {
			return fmt.Errorf("duplicate retired migration version %s", record.Version)
		}
		byVersion[record.Version] = record
	}
	for _, migration := range expected {
		record, exists := byVersion[migration.Version]
		if !exists {
			return fmt.Errorf("missing retired migration version %s", migration.Version)
		}
		if record.Dirty {
			return fmt.Errorf("retired migration %s is dirty; finish upgrading with the previous release", migration.Version)
		}
		if record.Checksum != migration.Checksum {
			return fmt.Errorf("retired migration checksum mismatch for %s", migration.Version)
		}
		nameHash := sha256.Sum256([]byte(record.Name))
		if hex.EncodeToString(nameHash[:]) != migration.NameChecksum {
			return fmt.Errorf("retired migration name mismatch for %s", migration.Version)
		}
	}
	return nil
}
