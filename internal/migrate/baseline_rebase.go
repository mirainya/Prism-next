package migrate

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

//go:embed retired_history.json
var retiredHistoryJSON []byte

var ErrBaselineRebaseRequired = errors.New("database requires explicit baseline adoption; stop the application and run `prism migrate rebase`")

func activeRetiredHistory(available []Migration) (retiredHistory, bool, error) {
	history, err := parseRetiredHistory(retiredHistoryJSON)
	if err != nil {
		return retiredHistory{}, false, err
	}
	return history, len(available) > 0 && available[0].Version == history.BaselineVersion, nil
}

// Old records stay in the ledger unchanged. They are accepted only as a full,
// verified history alongside a successfully registered new baseline.
func withoutRetiredMigrations(available []Migration, applied []AppliedMigration) ([]AppliedMigration, error) {
	history, active, err := activeRetiredHistory(available)
	if err != nil || !active {
		return applied, err
	}
	return filterRetiredHistory(history, applied)
}

func filterRetiredHistory(history retiredHistory, applied []AppliedMigration) ([]AppliedMigration, error) {
	var retired, current []AppliedMigration
	baselineFound := false
	for _, record := range applied {
		if record.Version < history.BaselineVersion {
			retired = append(retired, record)
		} else {
			current = append(current, record)
			baselineFound = baselineFound || record.Version == history.BaselineVersion
		}
	}
	if len(retired) == 0 {
		return current, nil
	}
	if err := validateRetiredHistory(history, retired); err != nil {
		return nil, err
	}
	if !baselineFound {
		return nil, ErrBaselineRebaseRequired
	}
	return current, nil
}

// Rebase records the new baseline after validating every retired migration and
// the complete application schema. It executes no baseline DDL or seed SQL and
// changes no existing history or business rows. The advisory lock is shared
// with Up; inserting baseline and proven forward records is transactional.
func Rebase(ctx context.Context, db *gorm.DB) (Migration, error) {
	available, err := Load()
	if err != nil {
		return Migration{}, err
	}
	history, active, err := activeRetiredHistory(available)
	if err != nil {
		return Migration{}, err
	}
	if !active {
		return Migration{}, fmt.Errorf("this release does not use the clean baseline")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return Migration{}, err
	}
	connection, err := sqlDB.Conn(ctx)
	if err != nil {
		return Migration{}, err
	}
	defer connection.Close()
	if err := acquireMigrationLock(ctx, connection); err != nil {
		return Migration{}, err
	}
	defer releaseMigrationLock(connection)
	return rebaseWithConnection(ctx, connection, available, history)
}

func rebaseWithConnection(ctx context.Context, connection *sql.Conn, available []Migration, history retiredHistory) (Migration, error) {
	baseline := available[0]
	exists, err := hasTableWithConnection(ctx, connection, migrationTableName)
	if err != nil {
		return Migration{}, err
	}
	if !exists {
		return Migration{}, fmt.Errorf("registered complete migration history is required; upgrade with the previous release first")
	}
	applied, err := loadAppliedWithConnection(ctx, connection)
	if err != nil {
		return Migration{}, err
	}
	for _, record := range applied {
		if record.Version == baseline.Version {
			if _, err := reconcileMigrations(available, applied, false); err != nil {
				return Migration{}, err
			}
			return baseline, nil
		}
	}
	if err := validateRetiredHistory(history, applied); err != nil {
		return Migration{}, err
	}
	satisfied, extras, err := provenForwardMigrations(history, applied, available)
	if err != nil {
		return Migration{}, err
	}
	if err := validateBaselineSchemaWithExtras(ctx, connection, extras); err != nil {
		return Migration{}, err
	}
	transaction, err := connection.BeginTx(ctx, nil)
	if err != nil {
		return Migration{}, err
	}
	defer transaction.Rollback()
	for _, migration := range append([]Migration{baseline}, satisfied...) {
		if _, err := transaction.ExecContext(ctx,
			"INSERT INTO prism_schema_migrations (version, name, checksum, dirty) VALUES (?, ?, ?, 0)",
			migration.Version, migration.Name, migration.Checksum,
		); err != nil {
			return Migration{}, fmt.Errorf("record clean baseline: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return Migration{}, fmt.Errorf("commit clean baseline: %w", err)
	}
	return baseline, nil
}

// A forward migration may be registered without execution only if the exact
// SQL and migration name already exist in verified retired history. Its schema
// additions must also match before the transaction records either version.
func provenForwardMigrations(history retiredHistory, applied []AppliedMigration, available []Migration) ([]Migration, map[string]string, error) {
	var extensions map[string]map[string]string
	if err := json.Unmarshal(retiredSchemaExtensionsJSON, &extensions); err != nil {
		return nil, nil, err
	}
	appliedByVersion := make(map[string]AppliedMigration, len(applied))
	for _, record := range applied {
		appliedByVersion[record.Version] = record
	}
	availableByVersion := make(map[string]Migration, len(available))
	for _, migration := range available {
		availableByVersion[migration.Version] = migration
	}
	var satisfied []Migration
	extras := make(map[string]string)
	for _, mapping := range history.ForwardMigrations {
		record, exists := appliedByVersion[mapping.RetiredVersion]
		if !exists {
			continue
		}
		forward, exists := availableByVersion[mapping.ForwardVersion]
		if !exists || forward.Checksum != record.Checksum || forward.Name != record.Name {
			return nil, nil, fmt.Errorf("unproven forward migration %s", mapping.ForwardVersion)
		}
		addition, exists := extensions[mapping.RetiredVersion]
		if !exists {
			return nil, nil, fmt.Errorf("missing retired schema extension")
		}
		for name, checksum := range addition {
			if _, exists := extras[name]; exists {
				return nil, nil, fmt.Errorf("conflicting retired schema extension")
			}
			extras[name] = checksum
		}
		satisfied = append(satisfied, forward)
	}
	return satisfied, extras, nil
}
