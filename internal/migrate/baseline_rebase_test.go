package migrate

import (
	"errors"
	"testing"
)

func TestRetiredHistoryRequiresExplicitAdoption(t *testing.T) {
	history, applied := retiredHistoryFixture(t)
	if _, err := filterRetiredHistory(history, applied); !errors.Is(err, ErrBaselineRebaseRequired) {
		t.Fatalf("error=%v", err)
	}
	applied = append(applied, AppliedMigration{Version: history.BaselineVersion, Name: "clean_schema_baseline", Checksum: "new"})
	current, err := filterRetiredHistory(history, applied)
	if err != nil || len(current) != 1 || current[0].Version != history.BaselineVersion {
		t.Fatalf("current=%v error=%v", current, err)
	}
	// Even after adoption, tampering with an old record must remain an error.
	applied[0].Checksum = "changed"
	if _, err := filterRetiredHistory(history, applied); err == nil {
		t.Fatal("accepted altered retired history after adoption")
	}
}

func TestRetiredHistoryOnlyAllowsNamedCompleteCheckpoints(t *testing.T) {
	history, applied := retiredHistoryFixture(t)
	if err := validateRetiredHistory(history, applied[:1]); err == nil {
		t.Fatal("accepted arbitrary partial history")
	}
	history.CompatibleThroughVersions = []string{applied[0].Version}
	if err := validateRetiredHistory(history, applied[:1]); err != nil {
		t.Fatal(err)
	}
	applied[0].Dirty = true
	if err := validateRetiredHistory(history, applied[:1]); err == nil {
		t.Fatal("accepted dirty checkpoint")
	}
}

func TestForwardMigrationNeedsIdenticalPreviouslyAppliedSQL(t *testing.T) {
	history, applied := retiredHistoryFixture(t)
	history.ForwardMigrations = []retiredForwardMigration{{RetiredVersion: "20261015_180000", ForwardVersion: "20261015_210000"}}
	forward := Migration{Version: "20261015_210000", Name: applied[1].Name, Checksum: applied[1].Checksum}
	satisfied, extras, err := provenForwardMigrations(history, applied, []Migration{forward})
	if err != nil || len(satisfied) != 1 || len(extras) != 2 {
		t.Fatalf("satisfied=%v extras=%v error=%v", satisfied, extras, err)
	}
	forward.Checksum = "changed"
	if _, _, err := provenForwardMigrations(history, applied, []Migration{forward}); err == nil {
		t.Fatal("accepted different forward SQL")
	}
	if _, _, err := provenForwardMigrations(history, applied, nil); err == nil {
		t.Fatal("accepted missing forward migration")
	}
	satisfied, extras, err = provenForwardMigrations(history, applied[:1], nil)
	if err != nil || len(satisfied) != 0 || len(extras) != 0 {
		t.Fatal("unapplied historical SQL was treated as completed")
	}
}
