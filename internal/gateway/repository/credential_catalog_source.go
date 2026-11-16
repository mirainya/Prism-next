package repository

import (
	"context"
	"database/sql"
	"strings"

	"github.com/google/uuid"
	"github.com/mirainya/Prism/internal/gateway/credentials"
)

type CredentialCatalogSourceInput struct {
	BaseURL              string              `json:"base_url"`
	ExternalGroup        string              `json:"external_group"`
	RequestTimeoutMS     uint32              `json:"request_timeout_ms"`
	ProtocolDefinitionID uint64              `json:"protocol_definition_id"`
	Adapter              CatalogAdapterInput `json:"-"`
}

func (in *CredentialCatalogSourceInput) Normalize() {
	in.BaseURL = strings.TrimRight(strings.TrimSpace(in.BaseURL), "/")
	in.ExternalGroup = strings.TrimSpace(in.ExternalGroup)
}

func (in CredentialCatalogSourceInput) Validate() error {
	base, err := validateCatalogBaseURL(in.BaseURL)
	if err != nil || base.Path != "" && base.Path != "/" || !validCatalogSourceText(in.ExternalGroup, 128) ||
		in.RequestTimeoutMS < 1000 || in.RequestTimeoutMS > 120000 || in.ProtocolDefinitionID == 0 ||
		in.Adapter.Code != "declarative_catalog_v1" || in.Adapter.Version != 1 || in.Adapter.Protocol != "catalog_discovery" {
		return ErrInvalidInput
	}
	return nil
}

// PrepareCredentialCatalogSource is an explicit admin authorization/configuration
// action. It grants only catalog discovery, never execution or availability,
// and does not contact the upstream or change any model/price bindings.
func (s *Store) PrepareCredentialCatalogSource(ctx context.Context, tx *sql.Tx, credentialID uint64, in CredentialCatalogSourceInput, actorID uint64) (uint64, error) {
	if tx == nil || credentialID == 0 || actorID == 0 {
		return 0, ErrInvalidInput
	}
	in.Normalize()
	if err := in.Validate(); err != nil {
		return 0, err
	}
	var channelID uint64
	var authorized bool
	err := tx.QueryRowContext(ctx, `SELECT c.channel_id,
(c.status='active' AND ch.status='active' AND pool.status='active' AND cv.status='active'
 AND (cv.valid_until IS NULL OR cv.valid_until>CURRENT_TIMESTAMP(3)))
FROM gw_credentials c
JOIN gateway_channels ch ON ch.id=c.channel_id
JOIN gw_credential_pools pool ON pool.id=c.credential_pool_id AND pool.channel_id=c.channel_id
JOIN gw_credential_versions cv ON cv.id=c.current_version_id AND cv.credential_id=c.id
WHERE c.id=? FOR UPDATE`, credentialID).Scan(&channelID, &authorized)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if !authorized {
		return 0, ErrConflict
	}
	source := CatalogSourceInput{
		ChannelID: channelID, CredentialID: credentialID, SourceCode: "key-catalog-" + uuid.NewString(),
		ContractCode: "declarative_catalog_v1", BaseURL: in.BaseURL, ExternalGroup: in.ExternalGroup,
		RequestTimeoutMS: in.RequestTimeoutMS, ProtocolDefinitionID: in.ProtocolDefinitionID, Adapter: in.Adapter,
	}
	if err := validateCatalogProtocolBinding(ctx, tx, source); err != nil {
		return 0, err
	}
	var grantID, sequence uint64
	var grantStatus string
	err = tx.QueryRowContext(ctx, `SELECT id,grant_seq,status FROM gw_credential_purpose_grants
WHERE credential_id=? AND purpose='catalog_discovery' ORDER BY grant_seq DESC LIMIT 1 FOR UPDATE`, credentialID).Scan(&grantID, &sequence, &grantStatus)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	if err == sql.ErrNoRows || grantStatus != "active" {
		grantID, err = s.GrantCredentialPurpose(ctx, tx, credentialID, credentials.PurposeCatalogDiscovery, sequence+1)
		if err != nil {
			return 0, err
		}
		if err := recordCatalogAdminChange(ctx, tx, actorID, "unified.credential.catalog_discovery_authorize", "credential", credentialID,
			ginSafeMetadata{"purpose": "catalog_discovery", "grant_id": grantID}); err != nil {
			return 0, err
		}
	}
	// The Key lock also serializes identical first-time submissions. Do not
	// create duplicate sources, or reuse another Key's matching configuration.
	var sourceID uint64
	err = tx.QueryRowContext(ctx, `SELECT s.id FROM gw_catalog_sources s
JOIN gw_catalog_source_profiles p ON p.catalog_source_id=s.id
WHERE s.credential_id=? AND s.credential_purpose_grant_id=? AND s.status='active'
 AND p.contract_code='declarative_catalog_v1' AND p.protocol_definition_id=?
 AND BINARY p.base_url=BINARY ? AND BINARY p.external_group=BINARY ? AND p.request_timeout_ms=?
ORDER BY s.id DESC LIMIT 1 FOR UPDATE`, credentialID, grantID, in.ProtocolDefinitionID, in.BaseURL, in.ExternalGroup, in.RequestTimeoutMS).Scan(&sourceID)
	if err == nil {
		return sourceID, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	return s.CreateManagedCatalogSource(ctx, tx, source, actorID)
}
