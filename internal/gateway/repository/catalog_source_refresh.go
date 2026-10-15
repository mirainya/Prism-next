package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
)

// ScheduleActiveCatalogDiscovery only records a read-only upstream discovery.
// It never changes models, prices, routes or the active catalog version.
func (s *Store) ScheduleActiveCatalogDiscovery(ctx context.Context, tx *sql.Tx, releaseID, sourceID, actorID uint64) (uint64, error) {
	if tx == nil || releaseID == 0 || sourceID == 0 || actorID == 0 {
		return 0, ErrInvalidInput
	}
	if _, err := lockActiveCatalogRelease(ctx, tx, releaseID, 0); err != nil {
		return 0, err
	}
	var sourceVersion, credentialID, credentialVersionID, grantID, definitionID uint64
	var state, contract string
	var authorized bool
	err := tx.QueryRowContext(ctx, `SELECT s.status,s.state_version,s.credential_id,c.current_version_id,s.credential_purpose_grant_id,
p.contract_code,COALESCE(p.protocol_definition_id,0),
(ch.status='active' AND pool.status='active' AND c.status='active' AND cv.status='active'
 AND (cv.valid_until IS NULL OR cv.valid_until>CURRENT_TIMESTAMP(3))
 AND g.status='active' AND g.purpose='catalog_discovery')
FROM gw_catalog_sources s
JOIN gw_catalog_source_profiles p ON p.catalog_source_id=s.id
JOIN gateway_channels ch ON ch.id=s.channel_id
JOIN gw_credentials c ON c.id=s.credential_id AND c.channel_id=s.channel_id
JOIN gw_credential_pools pool ON pool.id=c.credential_pool_id AND pool.channel_id=c.channel_id
JOIN gw_credential_versions cv ON cv.id=c.current_version_id AND cv.credential_id=c.id
JOIN gw_credential_purpose_grants g ON g.id=s.credential_purpose_grant_id AND g.credential_id=c.id
WHERE s.id=? FOR UPDATE`, sourceID).Scan(&state, &sourceVersion, &credentialID, &credentialVersionID, &grantID, &contract, &definitionID, &authorized)
	if err == sql.ErrNoRows {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if state != "active" || !authorized {
		return 0, ErrConflict
	}
	if err := validateCatalogProtocolBinding(ctx, tx, CatalogSourceInput{ContractCode: contract, ProtocolDefinitionID: definitionID}); err != nil {
		return 0, err
	}

	// Source row locking serializes clicks, including clicks from other tabs.
	// All nonterminal runs for this source count, not only this catalog release.
	var existingID uint64
	var existingState string
	err = tx.QueryRowContext(ctx, `SELECT r.id,r.state FROM gw_control_plane_runs r
JOIN gw_catalog_release_sources rs ON rs.id=r.catalog_release_source_id
WHERE rs.catalog_source_id=? AND r.action='catalog_discovery' AND r.state IN ('scheduled','running','manual_review')
ORDER BY r.id DESC LIMIT 1 FOR UPDATE`, sourceID).Scan(&existingID, &existingState)
	if err == nil {
		if existingState == "manual_review" {
			return 0, ErrConflict
		}
		return existingID, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}

	var releaseSourceID uint64
	err = tx.QueryRowContext(ctx, `SELECT id FROM gw_catalog_release_sources WHERE release_id=? AND catalog_source_id=? FOR UPDATE`, releaseID, sourceID).Scan(&releaseSourceID)
	if err == sql.ErrNoRows {
		result, err := tx.ExecContext(ctx, `INSERT INTO gw_catalog_release_sources(release_id,catalog_source_id,source_state_version,created_at) VALUES (?,?,?,?)`, releaseID, sourceID, sourceVersion, nowUTC())
		if err != nil {
			return 0, err
		}
		releaseSourceID, err = lastID(result)
		if err != nil {
			return 0, err
		}
	} else if err != nil {
		return 0, err
	}
	// Each deliberate refresh is a new run, even for identical results. The
	// stable release/source link is not rewritten; the run pins its version.
	digest := sha256.Sum256([]byte(fmt.Sprintf("catalog-refresh:%d:%d:%d:%s", sourceID, sourceVersion, credentialVersionID, uuid.NewString())))
	if err := s.AdjustCatalogSourceRunCount(ctx, tx, sourceID, 1); err != nil {
		return 0, err
	}
	runID, err := s.CreateControlPlaneRun(ctx, tx, ControlPlaneRunInput{
		Action: "catalog_discovery", CatalogReleaseSourceID: &releaseSourceID, CredentialID: &credentialID,
		TargetFingerprint: hex.EncodeToString(digest[:]), TargetConfigVersion: &sourceVersion,
		AuthCredentialVersionID: credentialVersionID, AuthPurposeGrantID: grantID,
	})
	if err != nil {
		return 0, err
	}
	return runID, recordCatalogAdminChange(ctx, tx, actorID, "unified.catalog_source.refresh", "control_plane_run", runID, ginSafeMetadata{
		"release_id": releaseID, "catalog_source_id": sourceID, "credential_id": credentialID,
		"source_version": sourceVersion, "credential_version_id": credentialVersionID,
	})
}
