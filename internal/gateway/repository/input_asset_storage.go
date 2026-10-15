package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// InputAssetStorage is the storage binding captured when an input was uploaded.
// Both fields are empty on assets created before per-token XFS bindings.
type InputAssetStorage struct {
	XFSBaseURL    string
	APIKey        string
	ObjectLocator string
}

// ReadOwnedInputAssetStorage accepts only a locator belonging to one of the
// assets uploaded for this request. The call and its media refs do not exist yet
// while a synchronous image edit is being prepared.
func (s *Store) ReadOwnedInputAssetStorage(ctx context.Context, userID, tokenID uint64, assetIDs []uint64, locator string) (InputAssetStorage, error) {
	if s == nil || s.db == nil || userID == 0 || tokenID == 0 || len(assetIDs) == 0 || len(assetIDs) > 32 || locator == "" || len(locator) > 2048 {
		return InputAssetStorage{}, ErrInvalidInput
	}
	args := make([]any, 0, 3+len(assetIDs))
	args = append(args, userID, tokenID, locator)
	placeholders := make([]string, len(assetIDs))
	for i, id := range assetIDs {
		if id == 0 {
			return InputAssetStorage{}, ErrInvalidInput
		}
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(`SELECT COALESCE(xfs_base_url,''),COALESCE(xfs_api_key,''),COALESCE(xfs_object_locator,'')
FROM gw_media_assets
WHERE user_id=? AND token_id=? AND storage_locator=? AND purpose='input' AND state='active' AND id IN (%s)`, strings.Join(placeholders, ","))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return InputAssetStorage{}, err
	}
	defer rows.Close()
	return scanInputAssetStorage(rows)
}

// ReadCallInputAssetStorage permits only an input locator referenced by the
// persisted call. This excludes unrelated assets owned by the same token.
func (s *Store) ReadCallInputAssetStorage(ctx context.Context, callID uint64, locator string) (InputAssetStorage, error) {
	if s == nil || s.db == nil || callID == 0 || locator == "" || len(locator) > 2048 {
		return InputAssetStorage{}, ErrInvalidInput
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT COALESCE(ma.xfs_base_url,''),COALESCE(ma.xfs_api_key,''),COALESCE(ma.xfs_object_locator,'')
FROM gw_media_asset_refs mr
JOIN gw_api_calls c ON c.id=mr.call_id AND c.user_id=mr.user_id AND c.token_id=mr.token_id
JOIN gw_media_assets ma ON ma.id=mr.media_asset_id AND ma.user_id=mr.user_id AND ma.token_id=mr.token_id
WHERE mr.call_id=? AND mr.role='input' AND mr.result_delivery_id IS NULL
  AND ma.purpose='input' AND ma.state='active' AND ma.storage_locator=?`, callID, locator)
	if err != nil {
		return InputAssetStorage{}, err
	}
	defer rows.Close()
	return scanInputAssetStorage(rows)
}

func scanInputAssetStorage(rows *sql.Rows) (InputAssetStorage, error) {
	var storage InputAssetStorage
	found := false
	for rows.Next() {
		var current InputAssetStorage
		if err := rows.Scan(&current.XFSBaseURL, &current.APIKey, &current.ObjectLocator); err != nil {
			return InputAssetStorage{}, err
		}
		current.XFSBaseURL = strings.TrimSpace(current.XFSBaseURL)
		current.APIKey = strings.TrimSpace(current.APIKey)
		current.ObjectLocator = strings.TrimSpace(current.ObjectLocator)
		if (current.XFSBaseURL == "") != (current.APIKey == "") {
			return InputAssetStorage{}, ErrConflict
		}
		if found && current != storage {
			return InputAssetStorage{}, ErrConflict
		}
		storage, found = current, true
	}
	if err := rows.Err(); err != nil {
		return InputAssetStorage{}, err
	}
	if !found {
		return InputAssetStorage{}, ErrNotFound
	}
	return storage, nil
}
