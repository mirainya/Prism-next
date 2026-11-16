package repository

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	URLImportDetect  = "detect"
	URLImportAsyncV1 = "async_v1"
	URLImportLegacy  = "legacy_url"
)

// ManagedURLImport fixes the external write identity before transmission.
// SourceSHA256 identifies the original URL/role; neither URL nor key is stored here.
type ManagedURLImport struct {
	AttemptID                                                  uint64
	Ordinal                                                    uint32
	Mode, TaskKey, StoragePath, OriginalFilename, SourceSHA256 string
	StorageBindingSHA256                                       string
	LegacyStarted                                              bool
	CreatedAt                                                  time.Time
}

// SelectManagedURLImportMode resolves a saved, unsent capability probe exactly
// once. Concurrent probes preserve whichever mode was selected first.
func (s *Store) SelectManagedURLImportMode(ctx context.Context, tx *sql.Tx, in ManagedURLImport, mode string) (ManagedURLImport, error) {
	if tx == nil || in.AttemptID == 0 || in.TaskKey == "" || mode != URLImportAsyncV1 && mode != URLImportLegacy {
		return ManagedURLImport{}, ErrInvalidInput
	}
	if _, err := tx.ExecContext(ctx, `UPDATE gw_result_import_jobs SET import_mode=? WHERE attempt_id=? AND result_ordinal=? AND task_key=? AND import_mode='detect'`, mode, in.AttemptID, in.Ordinal, in.TaskKey); err != nil {
		return ManagedURLImport{}, err
	}
	job := in
	if err := tx.QueryRowContext(ctx, `SELECT import_mode,legacy_started FROM gw_result_import_jobs WHERE attempt_id=? AND result_ordinal=? AND task_key=? FOR UPDATE`, in.AttemptID, in.Ordinal, in.TaskKey).Scan(&job.Mode, &job.LegacyStarted); err != nil {
		return ManagedURLImport{}, err
	}
	if job.Mode != URLImportAsyncV1 && job.Mode != URLImportLegacy {
		return ManagedURLImport{}, ErrConflict
	}
	return job, nil
}

const resultImportColumns = `attempt_id,result_ordinal,import_mode,task_key,storage_path,original_filename,source_sha256,storage_binding_sha256,legacy_started,created_at`

func (s *Store) ReadManagedURLImport(ctx context.Context, attemptID uint64, ordinal uint32) (ManagedURLImport, error) {
	if s == nil || s.db == nil || attemptID == 0 {
		return ManagedURLImport{}, ErrInvalidInput
	}
	return readManagedURLImport(s.db.QueryRowContext(ctx, `SELECT `+resultImportColumns+` FROM gw_result_import_jobs WHERE attempt_id=? AND result_ordinal=?`, attemptID, ordinal), attemptID, ordinal)
}

// ReserveManagedURLImport serializes on the original Attempt. Concurrent
// reservations reuse the winner, including its mode even if capabilities
// changed between probes. Conflicting destinations or sources are rejected.
func (s *Store) ReserveManagedURLImport(ctx context.Context, tx *sql.Tx, in ManagedURLImport) (ManagedURLImport, error) {
	if tx == nil || in.AttemptID == 0 || !validManagedURLImportInput(in) {
		return ManagedURLImport{}, ErrInvalidInput
	}
	var attemptID uint64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM gw_api_call_attempts WHERE id=? FOR UPDATE`, in.AttemptID).Scan(&attemptID); err == sql.ErrNoRows {
		return ManagedURLImport{}, ErrNotFound
	} else if err != nil {
		return ManagedURLImport{}, err
	}
	if attemptID != in.AttemptID {
		return ManagedURLImport{}, ErrConflict
	}
	existing, err := readManagedURLImport(tx.QueryRowContext(ctx, `SELECT `+resultImportColumns+` FROM gw_result_import_jobs WHERE attempt_id=? AND result_ordinal=? FOR UPDATE`, in.AttemptID, in.Ordinal), in.AttemptID, in.Ordinal)
	if err == nil {
		if existing.SourceSHA256 != in.SourceSHA256 || existing.StorageBindingSHA256 != in.StorageBindingSHA256 ||
			existing.StoragePath != in.StoragePath || existing.OriginalFilename != in.OriginalFilename {
			return ManagedURLImport{}, ErrConflict
		}
		return existing, nil
	}
	if err != ErrNotFound {
		return ManagedURLImport{}, err
	}
	in.TaskKey = "prism-" + uuid.NewString()
	in.CreatedAt = nowUTC()
	if err := requireOneRow(tx.ExecContext(ctx, `INSERT INTO gw_result_import_jobs(attempt_id,result_ordinal,import_mode,task_key,storage_path,original_filename,source_sha256,storage_binding_sha256,legacy_started,created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`, in.AttemptID, in.Ordinal, in.Mode, in.TaskKey, in.StoragePath, in.OriginalFilename, in.SourceSHA256, in.StorageBindingSHA256, in.LegacyStarted, in.CreatedAt)); err != nil {
		return ManagedURLImport{}, err
	}
	return in, nil
}

// ClaimLegacyURLImport commits the one permitted synchronous transmission.
// A lost response or restart may only reconcile the original destination.
func (s *Store) ClaimLegacyURLImport(ctx context.Context, tx *sql.Tx, in ManagedURLImport) (bool, error) {
	if tx == nil || in.AttemptID == 0 || in.Mode != URLImportLegacy || in.TaskKey == "" {
		return false, ErrInvalidInput
	}
	result, err := tx.ExecContext(ctx, `UPDATE gw_result_import_jobs SET legacy_started=1 WHERE attempt_id=? AND result_ordinal=? AND task_key=? AND import_mode='legacy_url' AND legacy_started=0`, in.AttemptID, in.Ordinal, in.TaskKey)
	if err != nil {
		return false, err
	}
	return affected(result)
}

func readManagedURLImport(row *sql.Row, attemptID uint64, ordinal uint32) (ManagedURLImport, error) {
	var out ManagedURLImport
	if err := row.Scan(&out.AttemptID, &out.Ordinal, &out.Mode, &out.TaskKey, &out.StoragePath, &out.OriginalFilename, &out.SourceSHA256, &out.StorageBindingSHA256, &out.LegacyStarted, &out.CreatedAt); err == sql.ErrNoRows {
		return ManagedURLImport{}, ErrNotFound
	} else if err != nil {
		return ManagedURLImport{}, err
	}
	id, err := uuid.Parse(strings.TrimPrefix(out.TaskKey, "prism-"))
	if out.AttemptID != attemptID || out.Ordinal != ordinal || !validManagedURLImportInput(out) || out.CreatedAt.IsZero() || err != nil || out.TaskKey != "prism-"+id.String() {
		return ManagedURLImport{}, ErrConflict
	}
	return out, nil
}

func validManagedURLImportInput(in ManagedURLImport) bool {
	if in.Mode != URLImportDetect && in.Mode != URLImportAsyncV1 && in.Mode != URLImportLegacy || !validHexDigest(in.SourceSHA256, 32) || !validHexDigest(in.StorageBindingSHA256, 32) ||
		in.StoragePath == "" || len(in.StoragePath) > 1024 || strings.HasPrefix(in.StoragePath, "/") || !strings.HasSuffix(in.StoragePath, "/") ||
		strings.ContainsAny(in.StoragePath, "\\\r\n\x00") || in.OriginalFilename == "" || len(in.OriginalFilename) > 255 ||
		in.OriginalFilename != strings.TrimSpace(in.OriginalFilename) || in.OriginalFilename == "." || in.OriginalFilename == ".." ||
		strings.ContainsAny(in.OriginalFilename, "/\\\r\n\x00") {
		return false
	}
	for _, segment := range strings.Split(in.StoragePath, "/") {
		if strings.TrimSpace(segment) == "." || strings.TrimSpace(segment) == ".." {
			return false
		}
	}
	return true
}
