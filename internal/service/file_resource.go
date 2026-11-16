package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/pkg/filestorage"
	"gorm.io/gorm"
)

const maxResolvedFileBytes int64 = 64 << 20

type ownedFileRow struct {
	model.AIFile
	MediaAssetID uint64 `gorm:"column:media_asset_id"`
	ObjectURL    string `gorm:"column:object_url"`
	AssetBytes   uint64 `gorm:"column:asset_bytes"`
	AssetSHA256  string `gorm:"column:asset_sha256"`
	XFSBaseURL   string `gorm:"column:xfs_base_url"`
	XFSAPIKey    string `gorm:"column:xfs_api_key"`
}

// LoadOwnedAIFile resolves a processed file through its typed, ownership-bound
// media reference. When withContent is true, bytes are fetched from the
// authenticated object-storage endpoint and verified before use.
func LoadOwnedAIFile(ctx context.Context, tokenID uint, fileID string, withContent bool) (*model.AIFile, error) {
	fileID = strings.TrimSpace(fileID)
	if tokenID == 0 || fileID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var row ownedFileRow
	result := model.DB().WithContext(ctx).Raw(`SELECT
		f.id,f.user_id,f.token_id,f.filename,f.purpose,f.bytes,f.mime_type,f.status,f.created_at,f.updated_at,
		a.id AS media_asset_id,COALESCE(NULLIF(a.xfs_object_locator,''),NULLIF(a.storage_locator,''),a.object_key) AS object_url,
		a.content_length AS asset_bytes,a.sha256 AS asset_sha256,
		COALESCE(a.xfs_base_url,'') AS xfs_base_url,COALESCE(a.xfs_api_key,'') AS xfs_api_key
	FROM gw_file_resources f
	JOIN gw_media_asset_refs r
	  ON r.ai_file_id=f.id AND r.user_id=f.user_id AND r.token_id=f.token_id
	 AND r.role='file' AND r.ordinal=0
	JOIN gw_media_assets a
	  ON a.id=r.media_asset_id AND a.user_id=r.user_id AND a.token_id=r.token_id
	WHERE f.id=? AND f.token_id=? AND f.status='processed' AND a.state='active'
	LIMIT 1`, fileID, tokenID).Scan(&row)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 || row.ID == "" || row.ObjectURL == "" {
		return nil, gorm.ErrRecordNotFound
	}
	if row.Bytes <= 0 || uint64(row.Bytes) != row.AssetBytes || row.Bytes > maxResolvedFileBytes {
		return nil, fmt.Errorf("file %s has inconsistent object metadata", fileID)
	}
	row.AIFile.ObjectURL = row.ObjectURL
	row.AIFile.XFSBaseURL = strings.TrimSpace(row.XFSBaseURL)
	row.AIFile.XFSAPIKey = strings.TrimSpace(row.XFSAPIKey)
	if (row.AIFile.XFSBaseURL == "") != (row.AIFile.XFSAPIKey == "") {
		return nil, fmt.Errorf("file %s has incomplete storage snapshot", fileID)
	}
	if !withContent {
		return &row.AIFile, nil
	}
	storage := filestorage.DefaultClient()
	if row.AIFile.XFSBaseURL != "" {
		storage = filestorage.WithBaseURLAndAPIKey(row.AIFile.XFSBaseURL, row.AIFile.XFSAPIKey)
	}
	content, err := storage.ReadURL(ctx, row.ObjectURL, row.Bytes)
	if err != nil {
		return nil, fmt.Errorf("read file %s: %w", fileID, err)
	}
	if int64(len(content)) != row.Bytes {
		return nil, fmt.Errorf("file %s object length mismatch", fileID)
	}
	digest := sha256.Sum256(content)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), row.AssetSHA256) {
		return nil, fmt.Errorf("file %s object digest mismatch", fileID)
	}
	row.AIFile.Content = content
	return &row.AIFile, nil
}
