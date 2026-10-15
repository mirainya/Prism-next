package repository

import (
	"context"
	"database/sql"

	"github.com/mirainya/Prism/internal/gateway/credentials"
)

func (s *Store) eraseCredentialSecrets(ctx context.Context, tx *sql.Tx, id uint64) error {
	rows, err := tx.QueryContext(ctx, s.forUpdate(`SELECT id,status FROM gw_credential_purpose_grants WHERE credential_id=? AND status<>'revoked' ORDER BY id`), id)
	if err != nil {
		return err
	}
	type grant struct {
		id    uint64
		state credentials.GrantState
	}
	grants := []grant{}
	for rows.Next() {
		var item grant
		if err := rows.Scan(&item.id, &item.state); err != nil {
			rows.Close()
			return err
		}
		grants = append(grants, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, item := range grants {
		if err := s.TransitionCredentialGrant(ctx, tx, item.id, item.state, credentials.GrantRevoked, "credential_remove"); err != nil {
			return err
		}
	}
	rows, err = tx.QueryContext(ctx, s.forUpdate(`SELECT id,status,encrypted_blob_id FROM gw_credential_versions WHERE credential_id=? ORDER BY id`), id)
	if err != nil {
		return err
	}
	type version struct {
		id    uint64
		state credentials.VersionState
		blob  sql.NullInt64
	}
	versions := []version{}
	for rows.Next() {
		var item version
		if err := rows.Scan(&item.id, &item.state, &item.blob); err != nil {
			rows.Close()
			return err
		}
		versions = append(versions, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	now := nowUTC()
	for _, item := range versions {
		if item.state == credentials.VersionActive {
			if err := requireOneRow(tx.ExecContext(ctx, `UPDATE gw_credential_versions SET status='superseded',retired_at=? WHERE id=? AND status='active'`, now, item.id)); err != nil {
				return err
			}
			if err := appendCredentialVersionAudit(ctx, tx, item.id, "active", "superseded", "credential_remove", now); err != nil {
				return err
			}
			item.state = credentials.VersionSuperseded
		}
		if item.state == credentials.VersionSuperseded {
			if err := requireOneRow(tx.ExecContext(ctx, `UPDATE gw_credential_versions SET status='retired',retired_at=? WHERE id=? AND status='superseded'`, now, item.id)); err != nil {
				return err
			}
			if err := appendCredentialVersionAudit(ctx, tx, item.id, "superseded", "retired", "credential_remove", now); err != nil {
				return err
			}
		} else if item.state != credentials.VersionRetired {
			return ErrConflict
		}
		if item.blob.Valid {
			// Keep the blob identity for historical version foreign keys. Empty
			// ciphertext and destroy DEK wraps; no credential content is retained.
			if err := requireOneRow(tx.ExecContext(ctx, `UPDATE encrypted_blobs SET ciphertext=?,purged_at=? WHERE id=? AND purpose='credential'`, []byte{}, now, item.blob.Int64)); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM encrypted_blob_key_wraps WHERE encrypted_blob_id=?`, item.blob.Int64); err != nil {
				return err
			}
		}
	}
	return requireOneRow(tx.ExecContext(ctx, `UPDATE gw_credentials SET secret=NULL,config_version=config_version+1,updated_at=? WHERE id=? AND status='disabled'`, now, id))
}
