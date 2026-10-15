package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func retiredHistoryFixture(t *testing.T) (retiredHistory, []AppliedMigration) {
	t.Helper()
	applied := []AppliedMigration{
		{Version: "20260718_150000", Name: "schema_baseline", Checksum: strings.Repeat("a", 64)},
		{Version: "20261015_180000", Name: "add_async_tracking_policy", Checksum: strings.Repeat("b", 64)},
	}
	history := retiredHistory{Schema: 1, BaselineVersion: "20261015_200000", ThroughVersion: "20261015_180000"}
	for _, record := range applied {
		hash := sha256.Sum256([]byte(record.Name))
		history.Migrations = append(history.Migrations, retiredMigration{
			Version: record.Version, Checksum: record.Checksum, NameChecksum: hex.EncodeToString(hash[:]),
		})
	}
	return history, applied
}

func TestRetiredHistoryValidatesEveryRecord(t *testing.T) {
	history, applied := retiredHistoryFixture(t)
	content, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseRetiredHistory(content)
	if err != nil {
		t.Fatal(err)
	}
	// Database ordering is not relied upon; record identity and completeness are.
	applied[0], applied[1] = applied[1], applied[0]
	if err := validateRetiredHistory(parsed, applied); err != nil {
		t.Fatal(err)
	}
	for _, record := range applied {
		if strings.Contains(string(content), record.Name) {
			t.Fatal("public manifest exposes a migration name")
		}
	}
}

func TestRetiredHistoryRejectsUnsafeDatabaseRecords(t *testing.T) {
	cases := []struct {
		name   string
		change func([]AppliedMigration) []AppliedMigration
		want   string
	}{
		{"missing", func(a []AppliedMigration) []AppliedMigration { return a[1:] }, "incomplete"},
		{"unknown", func(a []AppliedMigration) []AppliedMigration { a[0].Version = "20260801_000000"; return a }, "missing"},
		{"duplicate", func(a []AppliedMigration) []AppliedMigration { a[1] = a[0]; return a }, "duplicate"},
		{"dirty", func(a []AppliedMigration) []AppliedMigration { a[0].Dirty = true; return a }, "dirty"},
		{"checksum", func(a []AppliedMigration) []AppliedMigration { a[0].Checksum = strings.Repeat("c", 64); return a }, "checksum"},
		{"name", func(a []AppliedMigration) []AppliedMigration { a[0].Name = "changed"; return a }, "name"},
		{"extra", func(a []AppliedMigration) []AppliedMigration {
			return append(a, AppliedMigration{Version: "20261015_200000"})
		}, "incomplete"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history, applied := retiredHistoryFixture(t)
			err := validateRetiredHistory(history, tc.change(applied))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %s", err, tc.want)
			}
		})
	}
}

func TestRetiredHistoryRejectsMalformedManifest(t *testing.T) {
	cases := []struct {
		name   string
		change func(*retiredHistory)
	}{
		{"schema", func(h *retiredHistory) { h.Schema = 2 }},
		{"empty", func(h *retiredHistory) { h.Migrations = nil }},
		{"baseline", func(h *retiredHistory) { h.BaselineVersion = h.ThroughVersion }},
		{"invalid version", func(h *retiredHistory) { h.Migrations[0].Version = "invalid" }},
		{"missing original baseline", func(h *retiredHistory) { h.Migrations = h.Migrations[1:] }},
		{"cutoff", func(h *retiredHistory) { h.ThroughVersion = "20261015_190000" }},
		{"duplicate", func(h *retiredHistory) { h.Migrations[1] = h.Migrations[0] }},
		{"bad hash", func(h *retiredHistory) { h.Migrations[0].Checksum = "bad" }},
		{"bad name hash", func(h *retiredHistory) { h.Migrations[0].NameChecksum = "bad" }},
		{"unpinned forward SQL", func(h *retiredHistory) {
			h.ForwardMigrations = []retiredForwardMigration{{RetiredVersion: h.ThroughVersion, ForwardVersion: "20261015_210000"}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history, _ := retiredHistoryFixture(t)
			tc.change(&history)
			content, err := json.Marshal(history)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parseRetiredHistory(content); err == nil {
				t.Fatal("accepted invalid manifest")
			}
		})
	}

	history, _ := retiredHistoryFixture(t)
	content, _ := json.Marshal(history)
	for _, bad := range [][]byte{append(content, []byte(" {}")...), []byte(`{"unexpected":"private data"}`)} {
		if _, err := parseRetiredHistory(bad); err == nil {
			t.Fatal("accepted unexpected content")
		}
	}
}
