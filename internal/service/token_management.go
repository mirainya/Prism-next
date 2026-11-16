package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidTokenManagement = errors.New("invalid token management request")
	ErrTokenResumeBlocked     = errors.New("token cannot resume: check owner, expiration and billing configuration")
)

type TokenManagementQuery struct {
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
	UserID   uint   `form:"user_id"`
	TokenID  uint   `form:"token_id"`
	Search   string `form:"search"`
	State    string `form:"state"`
}

// Explicit projection: management responses never serialize credential material.
type ManagedToken struct {
	ID           uint                   `json:"id"`
	UserID       uint                   `json:"user_id"`
	Username     string                 `json:"username"`
	OwnerEnabled bool                   `json:"owner_enabled"`
	Name         string                 `json:"name"`
	KeyHint      string                 `json:"key_hint"`
	State        string                 `json:"state"`
	AuthVersion  string                 `json:"auth_version"`
	RateLimit    int                    `json:"rate_limit"`
	CreatedAt    time.Time              `json:"created_at"`
	ExpiresAt    *time.Time             `json:"expires_at"`
	RevokedAt    *time.Time             `json:"revoked_at"`
	Budget       *TokenBudgetSummary    `json:"budget"`
	Storage      TokenFileStorageStatus `json:"xfs_storage"`
}

func tokenState(t model.Token, now time.Time) string {
	if t.RevokedAt != nil {
		return "revoked"
	}
	if t.ExpiresAt != nil && !t.ExpiresAt.After(now) {
		return "expired"
	}
	if t.Status != 1 {
		return "paused"
	}
	return "active"
}

func tokenManagementScope(query *gorm.DB, scope ObservabilityScope, userID uint) (*gorm.DB, error) {
	if scope.ActorUserID == 0 || !scope.IsAdmin && userID != 0 && userID != scope.ActorUserID {
		return nil, ErrObservabilityAccessDenied
	}
	if !scope.IsAdmin {
		userID = scope.ActorUserID
	}
	if userID != 0 {
		query = query.Where("t.user_id=?", userID)
	}
	return query, nil
}

func (s *TokenService) ListManagedTokens(ctx context.Context, scope ObservabilityScope, req TokenManagementQuery) (*ObservabilityListResponse[ManagedToken], error) {
	if utf8.RuneCountInString(req.Search) > 100 {
		return nil, ErrInvalidTokenManagement
	}
	now := time.Now().UTC()
	query, err := tokenManagementScope(model.DB().WithContext(ctx).Table("tokens AS t").
		Joins("LEFT JOIN users u ON u.id=t.user_id").Where("t.deleted_at IS NULL"), scope, req.UserID)
	if err != nil {
		return nil, err
	}
	if req.TokenID != 0 {
		query = query.Where("t.id=?", req.TokenID)
	}
	if search := strings.TrimSpace(req.Search); search != "" {
		// Escape LIKE metacharacters so a literal name cannot expand into all keys.
		pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(search) + "%"
		query = query.Where("(t.name LIKE ? ESCAPE '!' OR u.username LIKE ? ESCAPE '!')", pattern, pattern)
	}
	switch req.State {
	case "":
	case "revoked":
		query = query.Where("t.revoked_at IS NOT NULL")
	case "expired":
		query = query.Where("t.revoked_at IS NULL AND t.expires_at<=?", now)
	case "paused", "active":
		query = query.Where("t.revoked_at IS NULL AND (t.expires_at IS NULL OR t.expires_at>?)", now)
		if req.State == "active" {
			query = query.Where("t.status=1")
		} else {
			query = query.Where("t.status<>1")
		}
	default:
		return nil, ErrInvalidTokenManagement
	}
	page, size := normalizeObservabilityPage(req.Page, req.PageSize)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []struct {
		model.Token  `gorm:"embedded"`
		Username     string
		OwnerEnabled bool
	}
	if err := query.Select("t.id,t.user_id,t.name,t.key_hint,t.status,t.auth_version,t.rate_limit,t.created_at,t.expires_at,t.revoked_at,t.xfs_base_url,t.xfs_api_key,COALESCE(u.username,'') AS username,CASE WHEN u.status=1 AND u.deleted_at IS NULL THEN 1 ELSE 0 END AS owner_enabled").
		Order("t.id DESC").Offset((page - 1) * size).Limit(size).Scan(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}
	funds, err := loadTokenFunds(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]ManagedToken, 0, len(rows))
	for _, row := range rows {
		items = append(items, ManagedToken{ID: row.ID, UserID: row.UserID, Username: row.Username, OwnerEnabled: row.OwnerEnabled,
			Name: row.Name, KeyHint: tokenKeyHint(&row.Token), State: tokenState(row.Token, now), AuthVersion: strconv.FormatUint(row.AuthVersion, 10), RateLimit: row.RateLimit,
			CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, RevokedAt: row.RevokedAt, Budget: funds[row.ID].Budget,
			Storage: newTokenFileStorageStatus(row.XFSBaseURL, row.XFSAPIKey)})
	}
	return &ObservabilityListResponse[ManagedToken]{Items: items, Total: total, Page: page, PageSize: size}, nil
}

type TokenAccessRequest struct {
	Action          string `json:"action"`
	ExpectedVersion string `json:"expected_version"`
	Note            string `json:"note"`
}

func (s *TokenService) ChangeTokenAccess(ctx context.Context, scope ObservabilityScope, id uint, req TokenAccessRequest) error {
	version, err := strconv.ParseUint(req.ExpectedVersion, 10, 64)
	req.Note = strings.TrimSpace(req.Note)
	if err != nil || version == 0 || version == math.MaxUint64 || id == 0 || req.Note == "" || utf8.RuneCountInString(req.Note) > 500 ||
		(req.Action != "pause" && req.Action != "resume" && req.Action != "revoke") {
		return ErrInvalidTokenManagement
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
		// Match funding operations' lock order: owner first, then token.
		var owner model.User
		if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).First(&owner, target.UserID).Error; err != nil {
			return err
		}
		var token model.Token
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND user_id=?", id, target.UserID).First(&token).Error; err != nil {
			return err
		}
		if token.AuthVersion != version || token.RevokedAt != nil {
			return repository.ErrConflict
		}
		now := time.Now().UTC()
		nextStatus := int8(0)
		if req.Action == "resume" {
			if token.Status == 1 || owner.Status != 1 || owner.DeletedAt.Valid || token.ExpiresAt != nil && !token.ExpiresAt.After(now) {
				return ErrTokenResumeBlocked
			}
			if err := validateTokenResume(tx, token, now); err != nil {
				return err
			}
			nextStatus = 1
		} else if req.Action == "pause" && token.Status != 1 {
			return repository.ErrConflict
		}
		updates := map[string]any{"status": nextStatus, "auth_version": version + 1}
		if req.Action == "revoke" {
			updates["revoked_at"] = now
		}
		result := tx.Model(&model.Token{}).Where("id=? AND auth_version=? AND revoked_at IS NULL", id, version).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return repository.ErrConflict
		}
		reason := "owner_" + req.Action
		if scope.IsAdmin {
			reason = "admin_" + req.Action
		}
		if err := tx.Exec(`INSERT INTO token_auth_state_events(token_id,auth_version,old_status,new_status,reason_code,actor_user_id,created_at) VALUES (?,?,?,?,?,?,?)`, id, version+1, token.Status, nextStatus, reason, scope.ActorUserID, now).Error; err != nil {
			return err
		}
		metadata, err := json.Marshal(map[string]any{"note": req.Note, "owner_user_id": token.UserID, "previous_status": token.Status, "status": nextStatus, "auth_version": strconv.FormatUint(version+1, 10)})
		if err != nil {
			return err
		}
		return tx.Create(&model.AuditEvent{ActorType: "user", ActorUserID: scope.ActorUserID, Action: "token." + req.Action, ResourceType: "token", ResourceID: strconv.FormatUint(uint64(id), 10), Outcome: "success", HTTPStatus: 200, Metadata: metadata, CreatedAt: now}).Error
	})
}

func validateTokenResume(tx *gorm.DB, token model.Token, now time.Time) error {
	var count int64
	if err := tx.Table("billing_accounts a").Joins("JOIN billing_system_state s ON s.id=1 AND s.currency_code=a.currency_code AND s.currency_version=a.currency_version").
		Where("a.user_id=? AND a.status='open'", token.UserID).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrTokenResumeBlocked
	}
	if err := tx.Table("token_budget_policy_activations a").
		Joins("JOIN token_budget_windows w ON w.token_id=a.token_id AND w.policy_id=a.policy_id AND w.activation_id=a.id").
		Where(`a.token_id=? AND a.effective_at<=? AND w.window_start<=? AND (w.window_end IS NULL OR w.window_end>?)
AND NOT EXISTS (SELECT 1 FROM token_budget_policy_activations newer WHERE newer.token_id=a.token_id AND newer.effective_at<=? AND newer.activation_seq>a.activation_seq)`, token.ID, now, now, now, now).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrTokenResumeBlocked
	}
	return nil
}

type TokenAccessEvent struct {
	ID          uint64    `json:"id"`
	AuthVersion string    `json:"auth_version"`
	OldStatus   int8      `json:"old_status"`
	NewStatus   int8      `json:"new_status"`
	ReasonCode  string    `json:"reason_code"`
	ActorUserID uint      `json:"actor_user_id"`
	CreatedAt   time.Time `json:"created_at"`
	Note        string    `json:"note" gorm:"-"`
}

func (s *TokenService) TokenAccessHistory(ctx context.Context, scope ObservabilityScope, id uint, page, size int) (*ObservabilityListResponse[TokenAccessEvent], error) {
	query, err := tokenManagementScope(model.DB().WithContext(ctx).Table("tokens AS t").Where("t.id=? AND t.deleted_at IS NULL", id), scope, 0)
	if err != nil {
		return nil, err
	}
	var target struct{ ID uint }
	if err := query.Select("t.id").Take(&target).Error; err != nil {
		return nil, err
	}
	page, size = normalizeObservabilityPage(page, size)
	events := model.DB().WithContext(ctx).Table("token_auth_state_events").Where("token_id=?", id)
	var total int64
	if err := events.Count(&total).Error; err != nil {
		return nil, err
	}
	items := make([]TokenAccessEvent, 0)
	if err := events.Order("id DESC").Offset((page - 1) * size).Limit(size).Scan(&items).Error; err != nil {
		return nil, err
	}
	if len(items) > 0 {
		first, last := items[0].CreatedAt, items[0].CreatedAt
		for _, item := range items {
			if item.CreatedAt.Before(first) {
				first = item.CreatedAt
			}
			if item.CreatedAt.After(last) {
				last = item.CreatedAt
			}
		}
		var audits []model.AuditEvent
		if err := model.DB().WithContext(ctx).Select("metadata").Where("resource_type=? AND resource_id=? AND outcome=? AND action IN ? AND created_at>=? AND created_at<=?",
			"token", strconv.FormatUint(uint64(id), 10), "success", []string{"token.pause", "token.resume", "token.revoke", "token.policy_updated"}, first, last).Find(&audits).Error; err != nil {
			return nil, err
		}
		notes := make(map[string]string, len(audits))
		for _, audit := range audits {
			var metadata struct {
				Version string `json:"auth_version"`
				Note    string `json:"note"`
			}
			if json.Unmarshal(audit.Metadata, &metadata) == nil {
				notes[metadata.Version] = metadata.Note
			}
		}
		for i := range items {
			items[i].Note = notes[items[i].AuthVersion]
		}
	}
	return &ObservabilityListResponse[TokenAccessEvent]{Items: items, Total: total, Page: page, PageSize: size}, nil
}
