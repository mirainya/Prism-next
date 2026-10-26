package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

// CanonicalProtocolJSON keeps the digest stable across MySQL JSON formatting.
func CanonicalProtocolJSON(raw []byte) ([]byte, error) {
	var value map[string]any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&value); err != nil {
		return nil, ErrInvalidInput
	}
	if value == nil {
		return nil, ErrInvalidInput
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, ErrInvalidInput
	}
	return json.Marshal(value)
}

// ProtocolDefinition is the versioned, database-owned description of an
// upstream catalog or availability protocol. DefinitionJSON is returned only
// to the worker; it is never exposed as part of a public model response.
type ProtocolDefinition struct {
	ID               uint64
	ProtocolCode     string
	ProtocolVersion  uint32
	ProtocolKind     string
	DisplayName      string
	DefinitionJSON   []byte
	DefinitionDigest string
	Status           string
	ConfigVersion    uint64
	CreatedAt        sql.NullTime
	UpdatedAt        sql.NullTime
}

type ProtocolDefinitionInput struct {
	ProtocolCode     string `json:"protocol_code"`
	ProtocolVersion  uint32 `json:"protocol_version"`
	ProtocolKind     string `json:"protocol_kind"`
	DisplayName      string `json:"display_name"`
	DefinitionJSON   []byte `json:"definition_json"`
	DefinitionDigest string `json:"definition_digest"`
}

func nullableProtocolDefinitionID(id uint64) any {
	if id == 0 {
		return nil
	}
	return id
}

func (in *ProtocolDefinitionInput) Normalize() {
	in.ProtocolCode = strings.ToLower(strings.TrimSpace(in.ProtocolCode))
	in.ProtocolKind = strings.ToLower(strings.TrimSpace(in.ProtocolKind))
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.DefinitionDigest = strings.ToLower(strings.TrimSpace(in.DefinitionDigest))
}

func (in ProtocolDefinitionInput) Validate() error {
	if !channelCodePattern.MatchString(in.ProtocolCode) || in.ProtocolVersion == 0 ||
		(in.ProtocolKind != "catalog_discovery" && in.ProtocolKind != "availability") ||
		!validDisplayName(in.DisplayName) || len(in.DefinitionJSON) == 0 || len(in.DefinitionJSON) > 1<<20 ||
		!validHexDigest(in.DefinitionDigest, 32) || !utf8.Valid(in.DefinitionJSON) {
		return ErrInvalidInput
	}
	canonical, err := CanonicalProtocolJSON(in.DefinitionJSON)
	if err != nil {
		return err
	}
	var document struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(canonical, &document) != nil || document.Kind != in.ProtocolKind {
		return ErrInvalidInput
	}
	digest := sha256.Sum256(canonical)
	if in.DefinitionDigest != hex.EncodeToString(digest[:]) {
		return ErrInvalidInput
	}
	return nil
}

// CreateProtocolDefinition stores an immutable protocol version. Publishing a
// different document must use a new protocol_version, which keeps old runs
// reproducible and makes rollback a data change rather than a code change.
func (s *Store) CreateProtocolDefinition(ctx context.Context, tx *sql.Tx, in ProtocolDefinitionInput, actorID uint64) (uint64, error) {
	if tx == nil || actorID == 0 {
		return 0, ErrInvalidInput
	}
	in.Normalize()
	canonical, err := CanonicalProtocolJSON(in.DefinitionJSON)
	if err != nil {
		return 0, err
	}
	in.DefinitionJSON = canonical
	if err := in.Validate(); err != nil {
		return 0, err
	}
	now := nowUTC()
	result, err := tx.ExecContext(ctx, `INSERT INTO gw_protocol_definitions
(protocol_code,protocol_version,protocol_kind,display_name,definition_json,definition_digest,status,config_version,created_at,updated_at)
VALUES (?,?,?,?,?,?,'active',1,?,?)`, in.ProtocolCode, in.ProtocolVersion, in.ProtocolKind, in.DisplayName,
		in.DefinitionJSON, in.DefinitionDigest, now, now)
	if err != nil {
		return 0, err
	}
	id, err := lastID(result)
	if err != nil {
		return 0, err
	}
	return id, recordCatalogAdminChange(ctx, tx, actorID, "unified.protocol_definition.create", "protocol_definition", id, ginSafeMetadata{
		"protocol_code": in.ProtocolCode, "protocol_version": in.ProtocolVersion,
		"protocol_kind": in.ProtocolKind, "display_name": in.DisplayName,
		"definition_digest": in.DefinitionDigest,
	})
}

func (s *Store) TransitionProtocolDefinition(ctx context.Context, tx *sql.Tx, id, expectedVersion uint64, next string, actorID uint64) error {
	if tx == nil || id == 0 || expectedVersion == 0 || actorID == 0 || next != "active" && next != "disabled" {
		return ErrInvalidInput
	}
	var current string
	var version uint64
	if err := tx.QueryRowContext(ctx, `SELECT status,config_version FROM gw_protocol_definitions WHERE id=? FOR UPDATE`, id).Scan(&current, &version); err == sql.ErrNoRows {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if current == next || version != expectedVersion {
		return ErrConflict
	}
	if err := requireOneRow(tx.ExecContext(ctx, `UPDATE gw_protocol_definitions SET status=?,config_version=config_version+1,updated_at=? WHERE id=? AND config_version=?`, next, nowUTC(), id, version)); err != nil {
		return err
	}
	// Version every bound monitor, including on re-enable, so an old request
	// cannot repopulate its cache after a disable/enable cycle.
	if _, err := tx.ExecContext(ctx, `UPDATE gw_availability_sources SET config_version=config_version+1,updated_at=? WHERE protocol_definition_id=?`, nowUTC(), id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE a FROM gw_upstream_availability a JOIN gw_availability_sources src ON src.id=a.availability_source_id WHERE src.protocol_definition_id=?`, id); err != nil {
		return err
	}
	return recordCatalogAdminChange(ctx, tx, actorID, "unified.protocol_definition.status", "protocol_definition", id,
		ginSafeMetadata{"old_status": current, "new_status": next})
}

func (s *Store) ReadProtocolDefinition(ctx context.Context, id uint64) (ProtocolDefinition, error) {
	if s == nil || id == 0 {
		return ProtocolDefinition{}, ErrInvalidInput
	}
	var item ProtocolDefinition
	err := s.DB().QueryRowContext(ctx, `SELECT id,protocol_code,protocol_version,protocol_kind,display_name,definition_json,definition_digest,status,config_version,created_at,updated_at FROM gw_protocol_definitions WHERE id=?`, id).Scan(&item.ID, &item.ProtocolCode, &item.ProtocolVersion, &item.ProtocolKind, &item.DisplayName, &item.DefinitionJSON, &item.DefinitionDigest, &item.Status, &item.ConfigVersion, &item.CreatedAt, &item.UpdatedAt)
	if err == sql.ErrNoRows {
		return item, ErrNotFound
	}
	return item, err
}

// ReadActiveProtocolDefinition returns a copy suitable for a worker request.
// A disabled definition is intentionally treated as unavailable, so disabling
// a protocol stops new discovery/availability calls without code changes.
func (s *Store) ReadActiveProtocolDefinition(ctx context.Context, id uint64, kind string) ([]byte, error) {
	if s == nil || id == 0 || kind != "catalog_discovery" && kind != "availability" {
		return nil, ErrInvalidInput
	}
	var raw []byte
	err := s.DB().QueryRowContext(ctx, `SELECT definition_json
FROM gw_protocol_definitions
WHERE id=? AND protocol_kind=? AND status='active'`, id, kind).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), raw...), nil
}

func (s *Store) ListProtocolDefinitions(ctx context.Context, kind, status string, limit, offset int) ([]ProtocolDefinition, int64, error) {
	if s == nil || limit < 1 || limit > 100 || offset < 0 ||
		kind != "" && kind != "catalog_discovery" && kind != "availability" ||
		status != "" && status != "active" && status != "disabled" {
		return nil, 0, ErrInvalidInput
	}
	var total int64
	if err := s.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM gw_protocol_definitions WHERE (?='' OR protocol_kind=?) AND (?='' OR status=?)`, kind, kind, status, status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB().QueryContext(ctx, `SELECT id,protocol_code,protocol_version,protocol_kind,display_name,definition_digest,status,config_version,created_at,updated_at
FROM gw_protocol_definitions
WHERE (?='' OR protocol_kind=?) AND (?='' OR status=?)
ORDER BY protocol_code,protocol_version DESC LIMIT ? OFFSET ?`, kind, kind, status, status, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]ProtocolDefinition, 0, limit)
	for rows.Next() {
		var item ProtocolDefinition
		if err := rows.Scan(&item.ID, &item.ProtocolCode, &item.ProtocolVersion, &item.ProtocolKind, &item.DisplayName,
			&item.DefinitionDigest, &item.Status, &item.ConfigVersion, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}
