package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func tokenPolicy(rate int, expiration *time.Time, version string) TokenPolicyRequest {
	return TokenPolicyRequest{RateLimit: &rate, ExpiresAt: expiration, ExpectedVersion: version, Note: "核查后调整调用限制"}
}

func TestTokenPolicyChangesPersistAndKeepAudit(t *testing.T) {
	db, owner, id := fundsOperationsFixture(t)
	svc, ctx := NewTokenService(), context.Background()
	expiration := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)
	admin := ObservabilityScope{ActorUserID: 99, IsAdmin: true}
	req := tokenPolicy(3, &expiration, "1")
	if err := svc.UpdateTokenPolicy(ctx, ObservabilityScope{ActorUserID: owner + 1}, id, req); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross owner change: %v", err)
	}
	if err := svc.UpdateTokenPolicy(ctx, admin, id, req); err != nil {
		t.Fatal(err)
	}
	var token model.Token
	if err := db.First(&token, id).Error; err != nil {
		t.Fatal(err)
	}
	if token.RateLimit != 3 || token.ExpiresAt == nil || !token.ExpiresAt.Equal(expiration) || token.AuthVersion != 2 || token.Status != 1 {
		t.Fatal("policy was not persisted")
	}
	if err := svc.UpdateTokenPolicy(ctx, admin, id, req); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("stale policy update: %v", err)
	}
	history, err := svc.TokenAccessHistory(ctx, ObservabilityScope{ActorUserID: owner}, id, 1, 20)
	if err != nil || len(history.Items) != 1 || history.Items[0].ReasonCode != "admin_policy_updated" || history.Items[0].Note != req.Note {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	var audit model.AuditEvent
	if err := db.First(&audit).Error; err != nil {
		t.Fatal(err)
	}
	var details struct {
		Before struct {
			Rate int `json:"rate_limit"`
		}
		After struct {
			Rate int `json:"rate_limit"`
		}
	}
	if json.Unmarshal(audit.Metadata, &details) != nil || details.Before.Rate != 60 || details.After.Rate != 3 || audit.ActorUserID != 99 {
		t.Fatalf("audit=%s", audit.Metadata)
	}
	// An unchanged form does not create a new authorization version or event.
	req.ExpectedVersion = "2"
	if err := svc.UpdateTokenPolicy(ctx, admin, id, req); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Table("token_auth_state_events").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("no-op events=%d err=%v", count, err)
	}
	if err := svc.UpdateTokenPolicy(ctx, ObservabilityScope{ActorUserID: owner}, id, tokenPolicy(0, nil, "2")); err != nil {
		t.Fatal(err)
	}
	token = model.Token{}
	if err := db.First(&token, id).Error; err != nil {
		t.Fatal(err)
	}
	if token.RateLimit != 0 || token.ExpiresAt != nil || token.AuthVersion != 3 {
		t.Fatal("inherit rate or clearing expiration failed")
	}
}

func TestTokenPolicyCannotRestoreRevokedOrBrokenExpiredKey(t *testing.T) {
	for _, scenario := range []string{"revoked", "disabled_owner", "removed_owner", "frozen_account", "missing_budget", "paused"} {
		t.Run(scenario, func(t *testing.T) {
			db, owner, id := fundsOperationsFixture(t)
			past := time.Now().UTC().Add(-time.Hour)
			if err := db.Model(&model.Token{}).Where("id=?", id).Update("expires_at", past).Error; err != nil {
				t.Fatal(err)
			}
			var err error
			switch scenario {
			case "revoked":
				err = db.Model(&model.Token{}).Where("id=?", id).Updates(map[string]any{"status": 0, "revoked_at": past}).Error
			case "disabled_owner":
				err = db.Model(&model.User{}).Where("id=?", owner).Update("status", 0).Error
			case "removed_owner":
				err = db.Delete(&model.User{}, owner).Error
			case "frozen_account":
				err = db.Exec("UPDATE billing_accounts SET status='frozen' WHERE user_id=?", owner).Error
			case "missing_budget":
				err = db.Exec("DELETE FROM token_budget_windows WHERE token_id=?", id).Error
			case "paused":
				err = db.Model(&model.Token{}).Where("id=?", id).Update("status", 0).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			err = NewTokenService().UpdateTokenPolicy(context.Background(), ObservabilityScope{ActorUserID: 99, IsAdmin: true}, id, tokenPolicy(10, nil, "1"))
			if scenario == "paused" {
				if err != nil {
					t.Fatal(err)
				}
				var token model.Token
				if err := db.First(&token, id).Error; err != nil || token.Status != 0 {
					t.Fatal("changing policy restored a paused key")
				}
			} else if scenario == "revoked" {
				if !errors.Is(err, repository.ErrConflict) {
					t.Fatalf("revoked policy=%v", err)
				}
			} else if !errors.Is(err, ErrTokenResumeBlocked) {
				t.Fatalf("invalid expired key policy=%v", err)
			}
		})
	}
}

func TestTokenPolicyRollbackAndValidation(t *testing.T) {
	db, owner, id := fundsOperationsFixture(t)
	ctx, svc := context.Background(), NewTokenService()
	scope := ObservabilityScope{ActorUserID: owner}
	past := time.Now().UTC().Add(-time.Second)
	for _, req := range []TokenPolicyRequest{tokenPolicy(-1, nil, "1"), tokenPolicy(MaxTokenRequestsPerMinute+1, nil, "1"), tokenPolicy(10, &past, "1"), tokenPolicy(10, nil, "0"), {ExpectedVersion: "1", Note: "missing rate"}, {ExpectedVersion: "1", RateLimit: new(int), Note: strings.Repeat("注", 501)}} {
		if err := svc.UpdateTokenPolicy(ctx, scope, id, req); !errors.Is(err, ErrInvalidTokenManagement) {
			t.Fatalf("invalid request: %v", err)
		}
	}
	if err := db.Exec("DROP TABLE audit_events").Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateTokenPolicy(ctx, scope, id, tokenPolicy(10, nil, "1")); err == nil {
		t.Fatal("policy without audit succeeded")
	}
	var token model.Token
	if err := db.First(&token, id).Error; err != nil || token.AuthVersion != 1 || token.RateLimit != 60 {
		t.Fatal("failed policy update was partially committed")
	}
}

func TestCreateTokenPersistsPolicyAndSelectionExcludesExpiredKeys(t *testing.T) {
	db, owner, existingID := fundsOperationsFixture(t)
	expires := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)
	rate := 2
	svc := NewTokenService()
	created, err := svc.CreateToken(owner, &CreateTokenReq{Name: "short-lived", Balance: decimal.NewFromInt(1), RateLimit: &rate, ExpiresAt: &expires})
	if err != nil {
		t.Fatal(err)
	}
	id := created["id"].(uint)
	var token model.Token
	if err := db.First(&token, id).Error; err != nil || token.RateLimit != 2 || token.ExpiresAt == nil || !token.ExpiresAt.Equal(expires) {
		t.Fatal("create policy missing")
	}
	for _, expiryID := range []uint{existingID, id} {
		if err := db.Model(&model.Token{}).Where("id=?", expiryID).Update("expires_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
			t.Fatal(err)
		}
	}
	selection, err := svc.ListTokens(owner)
	if err != nil || len(selection) != 0 {
		t.Fatalf("expired keys in selector=%d err=%v", len(selection), err)
	}
	if err := svc.UpdateTokenPolicy(context.Background(), ObservabilityScope{ActorUserID: owner}, id, tokenPolicy(5, nil, "1")); err != nil {
		t.Fatal(err)
	}
	selection, err = svc.ListTokens(owner)
	if err != nil || len(selection) != 1 || selection[0]["id"].(uint) != id {
		t.Fatalf("renewed selection=%+v err=%v", selection, err)
	}
	badRate, badExpiration := -1, time.Now().Add(-time.Hour)
	for _, invalid := range []CreateTokenReq{{Name: "bad rate", RateLimit: &badRate}, {Name: "bad expiration", ExpiresAt: &badExpiration}} {
		if _, err := svc.CreateToken(owner, &invalid); !errors.Is(err, ErrInvalidTokenManagement) {
			t.Fatalf("invalid create=%v", err)
		}
	}
}
