package repository

import (
	"context"
	"database/sql"

	"github.com/mirainya/Prism/internal/gateway/credentials"
)

// Removal retains credential identities, model configuration and accounting.
// Only secret material is erased, after every pinned operation has finished.
type CredentialRemovalImpact struct {
	CredentialID       uint64                   `json:"credential_id"`
	ConfigVersion      uint64                   `json:"config_version"`
	Status             string                   `json:"status"`
	SecretRemoved      bool                     `json:"secret_removed"`
	ActiveSlots        uint64                   `json:"active_slots"`
	UnresolvedAttempts uint64                   `json:"unresolved_attempts"`
	ControlRuns        uint64                   `json:"control_runs"`
	PendingDeliveries  uint64                   `json:"pending_deliveries"`
	OtherUsableKeys    uint64                   `json:"other_usable_keys"`
	Models             []CredentialRemovalModel `json:"models"`
}

type CredentialRemovalModel struct {
	APIName     string `json:"api_name"`
	Unavailable bool   `json:"unavailable"`
}

func (in CredentialRemovalImpact) Pending() bool {
	return in.ActiveSlots != 0 || in.UnresolvedAttempts != 0 || in.ControlRuns != 0 || in.PendingDeliveries != 0
}

// This is the same credential eligibility used by the public catalog: purpose,
// current version, expiry, identity and readable secret must all be valid.
const usableExecutionCredentialsSQL = `SELECT c.id,c.credential_pool_id
FROM gw_credentials c
JOIN gw_credential_pools pool ON pool.id=c.credential_pool_id AND pool.channel_id=c.channel_id AND pool.status='active'
JOIN gateway_channels ch ON ch.id=c.channel_id AND ch.status='active'
JOIN gw_credential_secret_identities identity_row ON identity_row.id=c.secret_identity_id AND identity_row.channel_id=c.channel_id AND identity_row.status='active'
JOIN gw_credential_versions cv ON cv.id=c.current_version_id AND cv.credential_id=c.id AND cv.secret_identity_id=c.secret_identity_id AND cv.status='active' AND (cv.valid_until IS NULL OR cv.valid_until>CURRENT_TIMESTAMP)
WHERE c.status='active'
AND EXISTS (SELECT 1 FROM gw_credential_purpose_grants g WHERE g.credential_id=c.id AND g.purpose='execution' AND g.status='active')
AND ((c.secret IS NOT NULL AND c.secret<>'') OR EXISTS (
 SELECT 1 FROM encrypted_blobs b
 JOIN crypto_keyring_state k ON k.id=b.keyring_id
 JOIN crypto_key_versions kv ON kv.keyring_id=k.id AND kv.key_version=k.current_version AND kv.status='current'
 JOIN encrypted_blob_key_wraps w ON w.encrypted_blob_id=b.id AND w.keyring_id=b.keyring_id AND w.kek_version=k.current_version
 WHERE b.id=cv.encrypted_blob_id AND b.purged_at IS NULL))`

func (s *Store) CredentialRemovalImpact(ctx context.Context, db DB, id uint64) (CredentialRemovalImpact, error) {
	if s == nil || db == nil || id == 0 {
		return CredentialRemovalImpact{}, ErrInvalidInput
	}
	impact := CredentialRemovalImpact{CredentialID: id, Models: []CredentialRemovalModel{}}
	var poolID uint64
	var hasSecret bool
	err := db.QueryRowContext(ctx, `SELECT c.credential_pool_id,c.status,c.config_version,
((c.secret IS NOT NULL AND c.secret<>'') OR EXISTS (SELECT 1 FROM gw_credential_versions v JOIN encrypted_blobs b ON b.id=v.encrypted_blob_id WHERE v.credential_id=c.id AND b.purged_at IS NULL))
FROM gw_credentials c WHERE c.id=?`, id).Scan(&poolID, &impact.Status, &impact.ConfigVersion, &hasSecret)
	if err == sql.ErrNoRows {
		return impact, ErrNotFound
	}
	if err != nil {
		return impact, err
	}
	impact.SecretRemoved = impact.Status == "disabled" && !hasSecret
	if err := credentialRemovalPending(ctx, db, id, &impact); err != nil {
		return impact, err
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM (`+usableExecutionCredentialsSQL+`) usable WHERE usable.credential_pool_id=? AND usable.id<>?`, poolID, id).Scan(&impact.OtherUsableKeys); err != nil {
		return impact, err
	}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT mn.api_name,
NOT EXISTS (
 SELECT 1 FROM gw_model_operations other_mo
 JOIN gw_skus other_sku ON other_sku.release_id=other_mo.release_id AND other_sku.model_operation_id=other_mo.id
 JOIN gw_routes other_route ON other_route.release_id=other_sku.release_id AND other_route.sku_id=other_sku.id
 JOIN gw_offerings other_offering ON other_offering.release_id=other_route.release_id AND other_offering.id=other_route.offering_id
 JOIN gw_offering_runtime_state other_state ON other_state.release_id=other_offering.release_id AND other_state.offering_id=other_offering.id AND other_state.state='active'
 JOIN (`+usableExecutionCredentialsSQL+`) usable ON usable.credential_pool_id=other_offering.credential_pool_id AND usable.id<>?
 WHERE other_mo.release_id=cm.release_id AND other_mo.catalog_model_id=cm.id
) AS unavailable
FROM gw_catalog_runtime_state runtime
JOIN gw_catalog_models cm ON cm.release_id=runtime.active_release_id
JOIN gw_catalog_model_names names ON names.release_id=cm.release_id AND names.catalog_model_id=cm.id AND names.is_primary=TRUE
JOIN gw_model_names mn ON mn.id=names.model_name_id
JOIN gw_model_operations mo ON mo.release_id=cm.release_id AND mo.catalog_model_id=cm.id
JOIN gw_skus sku ON sku.release_id=mo.release_id AND sku.model_operation_id=mo.id
JOIN gw_routes route ON route.release_id=sku.release_id AND route.sku_id=sku.id
JOIN gw_offerings offering ON offering.release_id=route.release_id AND offering.id=route.offering_id AND offering.credential_pool_id=?
WHERE runtime.id=1 ORDER BY mn.api_name`, id, poolID)
	if err != nil {
		return impact, err
	}
	defer rows.Close()
	for rows.Next() {
		var item CredentialRemovalModel
		if err := rows.Scan(&item.APIName, &item.Unavailable); err != nil {
			return impact, err
		}
		impact.Models = append(impact.Models, item)
	}
	return impact, rows.Err()
}

func credentialRemovalPending(ctx context.Context, db DB, id uint64, impact *CredentialRemovalImpact) error {
	for _, check := range []struct {
		query string
		count *uint64
	}{
		{`SELECT COUNT(*) FROM gw_credential_slots WHERE credential_id=? AND state IN ('active','recovery_required')`, &impact.ActiveSlots},
		{`SELECT COUNT(*) FROM gw_api_call_attempts WHERE credential_id=? AND state IN ('started','recovery_pending','terminated_unknown')`, &impact.UnresolvedAttempts},
		{`SELECT COUNT(*) FROM gw_control_plane_runs WHERE credential_id=? AND state IN ('scheduled','running','manual_review')`, &impact.ControlRuns},
		{`SELECT COUNT(*) FROM gw_result_deliveries d JOIN gw_api_call_attempts a ON a.id=d.attempt_id WHERE a.credential_id=? AND (d.state='pending' OR EXISTS (SELECT 1 FROM gw_async_outbox o WHERE o.result_delivery_id=d.id AND o.status IN ('pending','dispatching')))`, &impact.PendingDeliveries},
	} {
		if err := db.QueryRowContext(ctx, check.query, id).Scan(check.count); err != nil {
			return err
		}
	}
	return nil
}

// RemoveManagedCredential stops new admission immediately. Calling it again
// after draining completes erases the secret. The transaction uses READ
// COMMITTED; channel/pool/credential locks serialize admission and new Keys.
func (s *Store) RemoveManagedCredential(ctx context.Context, tx *sql.Tx, id, expectedVersion, actorID uint64) (CredentialRemovalImpact, error) {
	if tx == nil || id == 0 || expectedVersion == 0 || actorID == 0 {
		return CredentialRemovalImpact{}, ErrInvalidInput
	}
	var releaseID sql.NullInt64
	if err := tx.QueryRowContext(ctx, s.forShare(`SELECT active_release_id FROM gw_catalog_runtime_state WHERE id=1`)).Scan(&releaseID); err != nil {
		return CredentialRemovalImpact{}, err
	}
	var channelID, poolID uint64
	if err := tx.QueryRowContext(ctx, `SELECT channel_id,credential_pool_id FROM gw_credentials WHERE id=?`, id).Scan(&channelID, &poolID); err == sql.ErrNoRows {
		return CredentialRemovalImpact{}, ErrNotFound
	} else if err != nil {
		return CredentialRemovalImpact{}, err
	}
	var lockedID uint64
	for _, row := range []struct {
		query string
		id    uint64
	}{
		{`SELECT id FROM gateway_channels WHERE id=?`, channelID},
		{`SELECT id FROM gw_credential_pools WHERE id=?`, poolID},
		{`SELECT id FROM gw_credentials WHERE id=?`, id},
	} {
		if err := tx.QueryRowContext(ctx, s.forUpdate(row.query), row.id).Scan(&lockedID); err != nil {
			return CredentialRemovalImpact{}, err
		}
	}
	impact, err := s.CredentialRemovalImpact(ctx, tx, id)
	if err != nil {
		return impact, err
	}
	if impact.ConfigVersion != expectedVersion {
		return impact, ErrConflict
	}
	if impact.SecretRemoved {
		return impact, nil
	}
	if impact.Status == "active" {
		if err := s.TransitionCredential(ctx, tx, id, credentials.CredentialActive, credentials.CredentialDraining, "credential_remove"); err != nil {
			return impact, err
		}
		impact.Status, impact.ConfigVersion = "draining", impact.ConfigVersion+1
	}
	if impact.OtherUsableKeys == 0 && releaseID.Valid {
		if err := s.disableCredentialPoolOfferings(ctx, tx, uint64(releaseID.Int64), poolID, actorID); err != nil {
			return impact, err
		}
	}
	// Bound discovery/monitoring sources stay associated with this Key. They
	// never borrow another pool Key; history remains readable.
	if _, err := tx.ExecContext(ctx, `UPDATE gw_availability_sources SET status='disabled',config_version=config_version+1,updated_at=? WHERE credential_id=? AND status='active'`, nowUTC(), id); err != nil {
		return impact, err
	}
	if impact.Pending() {
		if err := recordCatalogAdminChange(ctx, tx, actorID, "unified.credential.remove_draining", "credential", id, ginSafeMetadata{"pool_id": poolID}); err != nil {
			return impact, err
		}
		return impact, nil
	}
	if impact.Status == "draining" {
		if err := s.TransitionCredential(ctx, tx, id, credentials.CredentialDraining, credentials.CredentialDisabled, "credential_remove"); err != nil {
			return impact, err
		}
		impact.Status, impact.ConfigVersion = "disabled", impact.ConfigVersion+1
	}
	if impact.Status != "disabled" {
		return impact, ErrConflict
	}
	if err := s.eraseCredentialSecrets(ctx, tx, id); err != nil {
		return impact, err
	}
	impact.ConfigVersion++
	impact.SecretRemoved = true
	err = recordCatalogAdminChange(ctx, tx, actorID, "unified.credential.remove", "credential", id, ginSafeMetadata{"pool_id": poolID, "models": impact.Models})
	return impact, err
}

func (s *Store) disableCredentialPoolOfferings(ctx context.Context, tx *sql.Tx, releaseID, poolID, actorID uint64) error {
	rows, err := tx.QueryContext(ctx, `SELECT offering.id,state.state_version FROM gw_offerings offering JOIN gw_offering_runtime_state state ON state.release_id=offering.release_id AND state.offering_id=offering.id WHERE offering.release_id=? AND offering.credential_pool_id=? AND state.state='active' ORDER BY offering.id`, releaseID, poolID)
	if err != nil {
		return err
	}
	type offeringVersion struct{ id, version uint64 }
	items := []offeringVersion{}
	for rows.Next() {
		var item offeringVersion
		if err := rows.Scan(&item.id, &item.version); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := s.SetOfferingRuntimeState(ctx, tx, item.id, OfferingRuntimeStateInput{State: "disabled", ExpectedVersion: item.version, ReasonCode: "credential_remove"}, actorID); err != nil {
			return err
		}
	}
	return nil
}
