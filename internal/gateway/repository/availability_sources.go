package repository

import (
	"context"
	"database/sql"
	"strings"
	"unicode/utf8"
)

type AvailabilitySourceInput struct {
	ChannelID            uint64 `json:"channel_id"`
	CredentialID         uint64 `json:"credential_id"`
	SourceCode           string `json:"source_code"`
	ContractCode         string `json:"contract_code"`
	BaseURL              string `json:"base_url"`
	RequestTimeoutMS     uint32 `json:"request_timeout_ms"`
	ProtocolDefinitionID uint64 `json:"protocol_definition_id"`
}

// AvailabilitySourceUpdate changes connection settings without changing the
// channel or provider contract used to interpret existing model identities.
type AvailabilitySourceUpdate struct {
	ContractCode         string  `json:"contract_code,omitempty"`
	ProtocolDefinitionID *uint64 `json:"protocol_definition_id,omitempty"`
	ExpectedVersion      uint64  `json:"expected_version"`
	CredentialID         uint64  `json:"credential_id"`
	BaseURL              string  `json:"base_url"`
	RequestTimeoutMS     uint32  `json:"request_timeout_ms"`
}

type ManagedAvailabilitySource struct {
	ID                   uint64
	ChannelID            uint64
	ChannelCode          string
	ChannelName          string
	SourceCode           string
	ContractCode         string
	ProtocolDefinitionID uint64
	BaseURL              string
	RequestTimeoutMS     uint32
	CredentialID         uint64
	CredentialCode       string
	Status               string
	ConfigVersion        uint64
	CreatedAt            sql.NullTime
	UpdatedAt            sql.NullTime
}

func (in *AvailabilitySourceInput) Normalize() {
	in.SourceCode = strings.ToLower(strings.TrimSpace(in.SourceCode))
	in.ContractCode = strings.ToLower(strings.TrimSpace(in.ContractCode))
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
}

func (in AvailabilitySourceInput) Validate() error {
	if in.ChannelID == 0 || in.CredentialID == 0 || !channelCodePattern.MatchString(in.SourceCode) ||
		!validAvailabilityContract(in.ContractCode) || in.RequestTimeoutMS < 1000 || in.RequestTimeoutMS > 120000 {
		return ErrInvalidInput
	}
	parsed, err := validateCatalogBaseURL(in.BaseURL)
	if err != nil || parsed.Path != "" && parsed.Path != "/" {
		return ErrInvalidInput
	}
	return nil
}

func validateAvailabilityProtocolBinding(ctx context.Context, tx *sql.Tx, in AvailabilitySourceInput) error {
	if in.ContractCode == "declarative_availability_v1" && in.ProtocolDefinitionID == 0 {
		return ErrInvalidInput
	}
	if in.ProtocolDefinitionID == 0 {
		return nil
	}
	if in.ContractCode != "declarative_availability_v1" {
		return ErrInvalidInput
	}
	var kind, status string
	err := tx.QueryRowContext(ctx, `SELECT protocol_kind,status FROM gw_protocol_definitions WHERE id=? FOR SHARE`, in.ProtocolDefinitionID).Scan(&kind, &status)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if kind != "availability" || status != "active" {
		return ErrConflict
	}
	return nil
}

func validAvailabilityContract(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= 64 && catalogIdentityPattern.MatchString(value)
}

func (s *Store) CreateManagedAvailabilitySource(ctx context.Context, tx *sql.Tx, in AvailabilitySourceInput, actorID uint64) (uint64, error) {
	if tx == nil || actorID == 0 {
		return 0, ErrInvalidInput
	}
	in.Normalize()
	if err := in.Validate(); err != nil {
		return 0, err
	}
	if err := validateAvailabilityProtocolBinding(ctx, tx, in); err != nil {
		return 0, err
	}
	grantID, err := activeAvailabilityGrant(ctx, tx, in.ChannelID, in.CredentialID)
	if err != nil {
		return 0, err
	}
	now := nowUTC()
	result, err := tx.ExecContext(ctx, `INSERT INTO gw_availability_sources
(channel_id,source_code,contract_code,protocol_definition_id,credential_id,credential_purpose_grant_id,base_url,request_timeout_ms,status,config_version,created_at,updated_at)
VALUES (?,?,?,?,?,?,?,?,'active',1,?,?)`, in.ChannelID, in.SourceCode, in.ContractCode, nullableProtocolDefinitionID(in.ProtocolDefinitionID), in.CredentialID, grantID,
		in.BaseURL, in.RequestTimeoutMS, now, now)
	if err != nil {
		return 0, err
	}
	id, err := lastID(result)
	if err != nil {
		return 0, err
	}
	return id, recordCatalogAdminChange(ctx, tx, actorID, "unified.availability_source.create", "availability_source", id,
		ginSafeMetadata{"channel_id": in.ChannelID, "credential_id": in.CredentialID, "source_code": in.SourceCode,
			"contract_code": in.ContractCode, "protocol_definition_id": in.ProtocolDefinitionID, "base_url": in.BaseURL, "request_timeout_ms": in.RequestTimeoutMS})
}

func activeAvailabilityGrant(ctx context.Context, tx *sql.Tx, channelID, credentialID uint64) (uint64, error) {
	var credentialChannelID, grantID uint64
	var credentialStatus, versionStatus, grantStatus string
	err := tx.QueryRowContext(ctx, `SELECT c.channel_id,c.status,v.status,g.id,g.status
FROM gw_credentials c
JOIN gw_credential_versions v ON v.id=c.current_version_id AND v.credential_id=c.id
JOIN gw_credential_purpose_grants g ON g.credential_id=c.id AND g.purpose='availability_read'
WHERE c.id=? ORDER BY g.grant_seq DESC LIMIT 1 FOR SHARE`, credentialID).
		Scan(&credentialChannelID, &credentialStatus, &versionStatus, &grantID, &grantStatus)
	if err == sql.ErrNoRows {
		return 0, ErrConflict
	}
	if err != nil {
		return 0, err
	}
	var channelStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM gateway_channels WHERE id=? FOR SHARE`, channelID).Scan(&channelStatus); err == sql.ErrNoRows {
		return 0, ErrConflict
	} else if err != nil {
		return 0, err
	}
	if credentialChannelID != channelID || credentialStatus != "active" || versionStatus != "active" || grantStatus != "active" || channelStatus != "active" {
		return 0, ErrConflict
	}
	return grantID, nil
}

func (s *Store) UpdateManagedAvailabilitySource(ctx context.Context, tx *sql.Tx, id uint64, in AvailabilitySourceUpdate, actorID uint64) error {
	if tx == nil || id == 0 || in.ExpectedVersion == 0 || actorID == 0 {
		return ErrInvalidInput
	}
	var current AvailabilitySourceInput
	var version uint64
	err := tx.QueryRowContext(ctx, `SELECT channel_id,source_code,contract_code,COALESCE(protocol_definition_id,0),config_version FROM gw_availability_sources WHERE id=? FOR UPDATE`, id).
		Scan(&current.ChannelID, &current.SourceCode, &current.ContractCode, &current.ProtocolDefinitionID, &version)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if version != in.ExpectedVersion {
		return ErrConflict
	}
	current.CredentialID, current.BaseURL, current.RequestTimeoutMS = in.CredentialID, in.BaseURL, in.RequestTimeoutMS
	if in.ContractCode != "" {
		current.ContractCode = in.ContractCode
	}
	if in.ProtocolDefinitionID != nil {
		current.ProtocolDefinitionID = *in.ProtocolDefinitionID
	}
	current.Normalize()
	if err := current.Validate(); err != nil {
		return err
	}
	if err := validateAvailabilityProtocolBinding(ctx, tx, current); err != nil {
		return err
	}
	grantID, err := activeAvailabilityGrant(ctx, tx, current.ChannelID, current.CredentialID)
	if err != nil {
		return err
	}
	if err := requireOneRow(tx.ExecContext(ctx, `UPDATE gw_availability_sources SET credential_id=?,credential_purpose_grant_id=?,base_url=?,request_timeout_ms=?,contract_code=?,protocol_definition_id=?,config_version=config_version+1,updated_at=? WHERE id=? AND config_version=?`,
		current.CredentialID, grantID, current.BaseURL, current.RequestTimeoutMS, current.ContractCode, nullableProtocolDefinitionID(current.ProtocolDefinitionID), nowUTC(), id, version)); err != nil {
		return err
	}
	// Never display observations from the previous account or endpoint as if
	// they were fetched using the edited connection.
	if _, err := tx.ExecContext(ctx, `DELETE FROM gw_upstream_availability WHERE availability_source_id=?`, id); err != nil {
		return err
	}
	return recordCatalogAdminChange(ctx, tx, actorID, "unified.availability_source.update", "availability_source", id,
		ginSafeMetadata{"credential_id": current.CredentialID, "base_url": current.BaseURL, "request_timeout_ms": current.RequestTimeoutMS, "contract_code": current.ContractCode, "protocol_definition_id": current.ProtocolDefinitionID})
}

func (s *Store) TransitionManagedAvailabilitySource(ctx context.Context, tx *sql.Tx, id, expectedVersion uint64, next string, actorID uint64) error {
	if tx == nil || id == 0 || expectedVersion == 0 || actorID == 0 || next != "active" && next != "disabled" {
		return ErrInvalidInput
	}
	var current, contract string
	var protocolID uint64
	var version, channelID, credentialID, grantID uint64
	if err := tx.QueryRowContext(ctx, `SELECT status,config_version,channel_id,credential_id,credential_purpose_grant_id,contract_code,COALESCE(protocol_definition_id,0) FROM gw_availability_sources WHERE id=? FOR UPDATE`, id).
		Scan(&current, &version, &channelID, &credentialID, &grantID, &contract, &protocolID); err == sql.ErrNoRows {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if version != expectedVersion || current == next {
		return ErrConflict
	}
	if next == "active" {
		if err := validateAvailabilityProtocolBinding(ctx, tx, AvailabilitySourceInput{ContractCode: contract, ProtocolDefinitionID: protocolID}); err != nil {
			return err
		}
		var err error
		grantID, err = activeAvailabilityGrant(ctx, tx, channelID, credentialID)
		if err != nil {
			return err
		}
	}
	if err := requireOneRow(tx.ExecContext(ctx, `UPDATE gw_availability_sources SET status=?,credential_purpose_grant_id=?,config_version=config_version+1,updated_at=? WHERE id=? AND config_version=?`, next, grantID, nowUTC(), id, version)); err != nil {
		return err
	}
	// Re-enabling must wait for a fresh observation, rather than exposing the
	// cache left by the previous active period.
	if _, err := tx.ExecContext(ctx, `DELETE FROM gw_upstream_availability WHERE availability_source_id=?`, id); err != nil {
		return err
	}
	return recordCatalogAdminChange(ctx, tx, actorID, "unified.availability_source.status", "availability_source", id,
		ginSafeMetadata{"old_status": current, "new_status": next})
}

func (s *Store) ListManagedAvailabilitySources(ctx context.Context, status string, limit, offset int) ([]ManagedAvailabilitySource, int64, error) {
	if s == nil || limit < 1 || limit > 100 || offset < 0 || status != "" && status != "active" && status != "disabled" {
		return nil, 0, ErrInvalidInput
	}
	db := s.DB()
	var total int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gw_availability_sources WHERE (?='' OR status=?)`, status, status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := db.QueryContext(ctx, `SELECT s.id,s.channel_id,ch.channel_code,ch.display_name,s.source_code,s.contract_code,COALESCE(s.protocol_definition_id,0),s.base_url,s.request_timeout_ms,s.credential_id,c.credential_code,s.status,s.config_version,s.created_at,s.updated_at
FROM gw_availability_sources s
JOIN gateway_channels ch ON ch.id=s.channel_id
JOIN gw_credentials c ON c.id=s.credential_id
WHERE (?='' OR s.status=?) ORDER BY s.id DESC LIMIT ? OFFSET ?`, status, status, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]ManagedAvailabilitySource, 0, limit)
	for rows.Next() {
		var item ManagedAvailabilitySource
		if err := rows.Scan(&item.ID, &item.ChannelID, &item.ChannelCode, &item.ChannelName, &item.SourceCode, &item.ContractCode, &item.ProtocolDefinitionID,
			&item.BaseURL, &item.RequestTimeoutMS, &item.CredentialID, &item.CredentialCode, &item.Status, &item.ConfigVersion,
			&item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}
