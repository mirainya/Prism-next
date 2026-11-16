package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/pkg/config"
	"github.com/mirainya/Prism/pkg/filestorage"
	"gorm.io/gorm"
)

var (
	ErrInvalidFileStorageAPIKey  = errors.New("invalid XFileStorage API key")
	ErrInvalidFileStorageBaseURL = config.ErrInvalidFileStorageBaseURL
	ErrFileStorageProbeFailed    = errors.New("XFileStorage verification failed")
	ErrFileStorageUpdateConflict = errors.New("XFileStorage setting changed concurrently")
)

var xfsAPIKeyPattern = regexp.MustCompile(`^xfs_[0-9a-f]{32}$`)

const tokenFileStorageProbeTimeout = 30 * time.Second

type TokenFileStorageStatus struct {
	Configured bool   `json:"configured"`
	KeyHint    string `json:"key_hint"`
	BaseURL    string `json:"base_url"`
}

func (s *TokenService) FileStorageEndpoints() ([]string, error) {
	cfg := config.Get()
	if cfg == nil {
		return []string{}, nil
	}
	return cfg.FileStorage.TokenBindingBaseURLs()
}

func (s *TokenService) BindFileStorage(ctx context.Context, userID, tokenID uint, baseURL, apiKey string) (TokenFileStorageStatus, error) {
	apiKey = strings.TrimSpace(apiKey)
	if ctx == nil || userID == 0 || tokenID == 0 || !xfsAPIKeyPattern.MatchString(apiKey) {
		return TokenFileStorageStatus{}, ErrInvalidFileStorageAPIKey
	}
	var token model.Token
	if err := model.DB().WithContext(ctx).Select("id").Where("id = ? AND user_id = ? AND status = 1 AND revoked_at IS NULL", tokenID, userID).First(&token).Error; err != nil {
		return TokenFileStorageStatus{}, err
	}
	cfg := config.Get()
	if cfg == nil {
		return TokenFileStorageStatus{}, ErrInvalidFileStorageBaseURL
	}
	baseURL, err := cfg.FileStorage.ResolveTokenBindingBaseURL(baseURL)
	if err != nil {
		return TokenFileStorageStatus{}, ErrInvalidFileStorageBaseURL
	}
	probe := s.probeFileStorage
	if probe == nil {
		probe = probeTokenFileStorage
	}
	probeCtx, cancelProbe := context.WithTimeout(ctx, tokenFileStorageProbeTimeout)
	defer cancelProbe()
	if err := probe(probeCtx, baseURL, apiKey); err != nil {
		return TokenFileStorageStatus{}, fmt.Errorf("%w: %v", ErrFileStorageProbeFailed, err)
	}
	result := model.DB().WithContext(ctx).Model(&model.Token{}).
		Where("id = ? AND user_id = ? AND status = 1 AND revoked_at IS NULL", tokenID, userID).
		UpdateColumns(map[string]any{"xfs_base_url": baseURL, "xfs_api_key": apiKey})
	if result.Error != nil {
		return TokenFileStorageStatus{}, result.Error
	}
	if result.RowsAffected == 0 {
		if err := confirmTokenFileStorageValue(ctx, userID, tokenID, baseURL, apiKey); err != nil {
			return TokenFileStorageStatus{}, err
		}
	}
	return newTokenFileStorageStatus(baseURL, apiKey), nil
}

func (s *TokenService) UnbindFileStorage(ctx context.Context, userID, tokenID uint) (TokenFileStorageStatus, error) {
	if ctx == nil || userID == 0 || tokenID == 0 {
		return TokenFileStorageStatus{}, gorm.ErrRecordNotFound
	}
	var token model.Token
	if err := model.DB().WithContext(ctx).Select("id").Where("id = ? AND user_id = ? AND status = 1 AND revoked_at IS NULL", tokenID, userID).First(&token).Error; err != nil {
		return TokenFileStorageStatus{}, err
	}
	result := model.DB().WithContext(ctx).Model(&model.Token{}).
		Where("id = ? AND user_id = ? AND status = 1 AND revoked_at IS NULL", tokenID, userID).
		UpdateColumns(map[string]any{"xfs_base_url": "", "xfs_api_key": ""})
	if result.Error != nil {
		return TokenFileStorageStatus{}, result.Error
	}
	if result.RowsAffected == 0 {
		if err := confirmTokenFileStorageValue(ctx, userID, tokenID, "", ""); err != nil {
			return TokenFileStorageStatus{}, err
		}
	}
	return newTokenFileStorageStatus("", ""), nil
}

func confirmTokenFileStorageValue(ctx context.Context, userID, tokenID uint, expectedBaseURL, expectedAPIKey string) error {
	var token model.Token
	if err := model.DB().WithContext(ctx).Select("id", "xfs_base_url", "xfs_api_key").Where("id = ? AND user_id = ? AND status = 1 AND revoked_at IS NULL", tokenID, userID).First(&token).Error; err != nil {
		return err
	}
	if strings.TrimSpace(token.XFSBaseURL) != expectedBaseURL || strings.TrimSpace(token.XFSAPIKey) != expectedAPIKey {
		return ErrFileStorageUpdateConflict
	}
	return nil
}

func probeTokenFileStorage(ctx context.Context, baseURL, apiKey string) error {
	return filestorage.WithBaseURLAndAPIKey(baseURL, apiKey).Probe(ctx)
}

func newTokenFileStorageStatus(baseURL, apiKey string) TokenFileStorageStatus {
	if strings.TrimSpace(baseURL) == "" {
		if cfg := config.Get(); cfg != nil {
			baseURL = strings.TrimRight(strings.TrimSpace(cfg.FileStorage.BaseURL), "/")
		}
	}
	apiKey = strings.TrimSpace(apiKey)
	return TokenFileStorageStatus{Configured: apiKey != "", KeyHint: fileStorageKeyHint(apiKey), BaseURL: baseURL}
}

func fileStorageKeyHint(apiKey string) string {
	if apiKey == "" {
		return ""
	}
	if len(apiKey) < 4 {
		return "****"
	}
	return "****" + apiKey[len(apiKey)-4:]
}
