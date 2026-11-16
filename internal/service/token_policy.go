package service

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/pkg/config"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MaxTokenRequestsPerMinute = 1000000

type TokenPolicyRequest struct {
	ExpectedVersion string     `json:"expected_version"`
	RateLimit       *int       `json:"rate_limit"`
	ExpiresAt       *time.Time `json:"expires_at"`
	Note            string     `json:"note"`
}

type TokenRateLimitSettings struct {
	Enabled        bool `json:"enabled"`
	RequestsPerMin int  `json:"requests_per_min"`
}

func TokenRateLimitDefaults() TokenRateLimitSettings {
	settings := TokenRateLimitSettings{RequestsPerMin: 60}
	if cfg := config.Get(); cfg != nil {
		settings.Enabled = cfg.RateLimit.Enabled
		if cfg.RateLimit.RequestsPerMin > 0 {
			settings.RequestsPerMin = cfg.RateLimit.RequestsPerMin
		}
	}
	return settings
}

func validateTokenPolicy(rate int, expiresAt *time.Time, now time.Time) error {
	if rate < 0 || rate > MaxTokenRequestsPerMinute || expiresAt != nil &&
		(!expiresAt.After(now) || expiresAt.Year() > 9999) {
		return ErrInvalidTokenManagement
	}
	return nil
}

// Authorization versions are shared with pause/revoke, preventing a stale
// policy form from extending a key that has since been withdrawn.
func (s *TokenService) UpdateTokenPolicy(ctx context.Context, scope ObservabilityScope, id uint, req TokenPolicyRequest) error {
	version, err := strconv.ParseUint(req.ExpectedVersion, 10, 64)
	req.Note = strings.TrimSpace(req.Note)
	now := time.Now().UTC().Truncate(time.Millisecond)
	if err != nil || version == 0 || version == math.MaxUint64 || id == 0 || req.RateLimit == nil ||
		req.Note == "" || utf8.RuneCountInString(req.Note) > 500 {
		return ErrInvalidTokenManagement
	}
	if req.ExpiresAt != nil {
		value := req.ExpiresAt.UTC().Truncate(time.Millisecond)
		req.ExpiresAt = &value
	}
	if err := validateTokenPolicy(*req.RateLimit, req.ExpiresAt, now); err != nil {
		return err
	}
	query, err := tokenManagementScope(model.DB().WithContext(ctx).Table("tokens AS t").Where("t.id=? AND t.deleted_at IS NULL", id), scope, 0)
	if err != nil {
		return err
	}
	var target struct{ UserID uint }
	if err := query.Select("t.user_id").Take(&target).Error; err != nil {
		return err
	}
	return model.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var owner model.User
		if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).First(&owner, target.UserID).Error; err != nil {
			return err
		}
		var token model.Token
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND user_id=?", id, target.UserID).First(&token).Error; err != nil {
			return err
		}
		if token.RevokedAt != nil || token.AuthVersion != version {
			return repository.ErrConflict
		}
		now = time.Now().UTC().Truncate(time.Millisecond)
		if err := validateTokenPolicy(*req.RateLimit, req.ExpiresAt, now); err != nil {
			return err
		}
		// Extending an expired but enabled key grants access again, so apply
		// the same dependency checks as an explicit resume operation.
		if token.Status == 1 && token.ExpiresAt != nil && !token.ExpiresAt.After(now) {
			if owner.Status != 1 || owner.DeletedAt.Valid {
				return ErrTokenResumeBlocked
			}
			if err := validateTokenResume(tx, token, now); err != nil {
				return err
			}
		}
		if token.RateLimit == *req.RateLimit && sameExpiration(token.ExpiresAt, req.ExpiresAt) {
			return nil
		}
		result := tx.Model(&model.Token{}).Where("id=? AND auth_version=? AND revoked_at IS NULL", id, version).
			Updates(map[string]any{"rate_limit": *req.RateLimit, "expires_at": req.ExpiresAt, "auth_version": version + 1})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return repository.ErrConflict
		}
		reason := "owner_policy_updated"
		if scope.IsAdmin {
			reason = "admin_policy_updated"
		}
		if err := tx.Exec(`INSERT INTO token_auth_state_events(token_id,auth_version,old_status,new_status,reason_code,actor_user_id,created_at) VALUES (?,?,?,?,?,?,?)`,
			id, version+1, token.Status, token.Status, reason, scope.ActorUserID, now).Error; err != nil {
			return err
		}
		metadata, err := json.Marshal(map[string]any{
			"owner_user_id": token.UserID, "auth_version": strconv.FormatUint(version+1, 10), "note": req.Note,
			"before": map[string]any{"rate_limit": token.RateLimit, "expires_at": token.ExpiresAt},
			"after":  map[string]any{"rate_limit": *req.RateLimit, "expires_at": req.ExpiresAt},
		})
		if err != nil {
			return err
		}
		return tx.Create(&model.AuditEvent{ActorType: "user", ActorUserID: scope.ActorUserID, Action: "token.policy_updated", ResourceType: "token",
			ResourceID: strconv.FormatUint(uint64(id), 10), Outcome: "success", HTTPStatus: 200, Metadata: metadata, CreatedAt: now}).Error
	})
}

func sameExpiration(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
