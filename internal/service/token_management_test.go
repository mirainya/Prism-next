package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
	"gorm.io/gorm"
)

func TestManagedTokenListIncludesHistoryAndRestrictsOwners(t *testing.T) {
	db, owner, tokenID := fundsOperationsFixture(t)
	svc, ctx := NewTokenService(), context.Background()
	other := model.User{Username: "another-owner", Status: 1}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Token{}).Where("id=?", tokenID).Updates(map[string]any{"status": 0, "xfs_api_key": "synthetic-storage-secret", "key_hint": "****test"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Token{UserID: other.ID, Name: "foreign-key", Status: 1, Selector: "synthetic-selector", SecretDigest: []byte("synthetic-digest")}).Error; err != nil {
		t.Fatal(err)
	}
	own := ObservabilityScope{ActorUserID: owner}
	result, err := svc.ListManagedTokens(ctx, own, TokenManagementQuery{})
	if err != nil || result.Total != 1 || len(result.Items) != 1 || result.Items[0].State != "paused" || result.Items[0].AuthVersion != "1" || !result.Items[0].OwnerEnabled {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	encoded, _ := json.Marshal(result)
	for _, secret := range []string{"synthetic-storage-secret", "synthetic-selector", "synthetic-digest", "secret_digest", "selector", "xfs_api_key"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("management serialized credential material: %s", secret)
		}
	}
	if _, err := svc.ListManagedTokens(ctx, own, TokenManagementQuery{UserID: other.ID}); !errors.Is(err, ErrObservabilityAccessDenied) {
		t.Fatalf("cross owner query: %v", err)
	}
	admin := ObservabilityScope{ActorUserID: 99, IsAdmin: true}
	all, err := svc.ListManagedTokens(ctx, admin, TokenManagementQuery{PageSize: 1})
	if err != nil || all.Total != 2 || len(all.Items) != 1 {
		t.Fatalf("all=%+v err=%v", all, err)
	}
	for _, filter := range []TokenManagementQuery{{Search: "finance"}, {State: "paused"}, {UserID: owner}, {TokenID: tokenID}} {
		found, err := svc.ListManagedTokens(ctx, admin, filter)
		if err != nil || found.Total != 1 || found.Items[0].ID != tokenID {
			t.Fatalf("filter=%+v result=%+v err=%v", filter, found, err)
		}
	}
	if found, err := svc.ListManagedTokens(ctx, admin, TokenManagementQuery{Search: "%"}); err != nil || found.Total != 0 {
		t.Fatalf("literal wildcard: %+v %v", found, err)
	}
	if err := svc.ChangeTokenAccess(ctx, own, tokenID, TokenAccessRequest{Action: "revoke", ExpectedVersion: "1", Note: "permanent retirement"}); err != nil {
		t.Fatal(err)
	}
	found, err := svc.ListManagedTokens(ctx, own, TokenManagementQuery{State: "revoked"})
	if err != nil || found.Total != 1 || found.Items[0].RevokedAt == nil {
		t.Fatalf("revoked history=%+v %v", found, err)
	}
}

func TestTokenAccessVersionsPermissionsAndAudit(t *testing.T) {
	db, owner, tokenID := fundsOperationsFixture(t)
	svc, ctx := NewTokenService(), context.Background()
	admin := ObservabilityScope{ActorUserID: 99, IsAdmin: true}
	request := TokenAccessRequest{Action: "pause", ExpectedVersion: "1", Note: "investigate failed requests"}
	if err := svc.ChangeTokenAccess(ctx, ObservabilityScope{ActorUserID: owner + 1}, tokenID, request); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("unauthorized change=%v", err)
	}
	for index, action := range []string{"pause", "resume", "revoke"} {
		request.Action, request.ExpectedVersion = action, fmt.Sprint(index+1)
		if err := svc.ChangeTokenAccess(ctx, admin, tokenID, request); err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		if err := svc.ChangeTokenAccess(ctx, admin, tokenID, request); !errors.Is(err, repository.ErrConflict) {
			t.Fatalf("stale %s: %v", action, err)
		}
	}
	request.Action, request.ExpectedVersion = "resume", "4"
	if err := svc.ChangeTokenAccess(ctx, admin, tokenID, request); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("revocation was restored: %v", err)
	}
	history, err := svc.TokenAccessHistory(ctx, ObservabilityScope{ActorUserID: owner}, tokenID, 1, 2)
	if err != nil || history.Total != 3 || len(history.Items) != 2 || history.Items[0].AuthVersion != "4" || history.Items[0].ActorUserID != 99 || history.Items[0].Note != request.Note {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	if _, err := svc.TokenAccessHistory(ctx, ObservabilityScope{ActorUserID: owner + 1}, tokenID, 1, 20); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross owner history=%v", err)
	}
	var audits []model.AuditEvent
	if err := db.Find(&audits).Error; err != nil || len(audits) != 3 {
		t.Fatalf("audits=%d err=%v", len(audits), err)
	}
	if audits[0].ActorUserID != 99 || !strings.Contains(string(audits[0].Metadata), request.Note) {
		t.Fatal("audit must record actor and processing note")
	}
}

func TestTokenAccessRollsBackIfAuditFails(t *testing.T) {
	db, owner, tokenID := fundsOperationsFixture(t)
	if err := db.Exec("DROP TABLE audit_events").Error; err != nil {
		t.Fatal(err)
	}
	err := NewTokenService().ChangeTokenAccess(context.Background(), ObservabilityScope{ActorUserID: owner}, tokenID, TokenAccessRequest{Action: "pause", ExpectedVersion: "1", Note: "test atomicity"})
	if err == nil {
		t.Fatal("missing audit must fail the operation")
	}
	var token model.Token
	if err := db.First(&token, tokenID).Error; err != nil {
		t.Fatal(err)
	}
	var events int64
	if err := db.Table("token_auth_state_events").Count(&events).Error; err != nil {
		t.Fatal(err)
	}
	if token.Status != 1 || token.AuthVersion != 1 || events != 0 {
		t.Fatalf("partial operation: status=%d version=%d events=%d", token.Status, token.AuthVersion, events)
	}
}

func TestTokenResumeRequiresValidDependencies(t *testing.T) {
	for _, scenario := range []string{"disabled_owner", "removed_owner", "expired", "frozen_account", "missing_account", "wrong_currency", "missing_budget", "duplicate_budget"} {
		t.Run(scenario, func(t *testing.T) {
			db, owner, tokenID := fundsOperationsFixture(t)
			if err := db.Model(&model.Token{}).Where("id=?", tokenID).Update("status", 0).Error; err != nil {
				t.Fatal(err)
			}
			var err error
			switch scenario {
			case "disabled_owner":
				err = db.Model(&model.User{}).Where("id=?", owner).Update("status", 0).Error
			case "removed_owner":
				err = db.Delete(&model.User{}, owner).Error
			case "expired":
				err = db.Model(&model.Token{}).Where("id=?", tokenID).Update("expires_at", time.Now().Add(-time.Hour)).Error
			case "frozen_account":
				err = db.Exec("UPDATE billing_accounts SET status='frozen' WHERE user_id=?", owner).Error
			case "missing_account":
				err = db.Exec("DELETE FROM billing_accounts WHERE user_id=?", owner).Error
			case "wrong_currency":
				err = db.Exec("UPDATE billing_accounts SET currency_version=2 WHERE user_id=?", owner).Error
			case "missing_budget":
				err = db.Exec("DELETE FROM token_budget_windows WHERE token_id=?", tokenID).Error
			case "duplicate_budget":
				err = db.Exec("INSERT INTO token_budget_windows(token_id,policy_id,activation_id,window_start,limit_amount,used_amount,held_amount,created_at) SELECT token_id,policy_id,activation_id,window_start,limit_amount,used_amount,held_amount,created_at FROM token_budget_windows WHERE token_id=?", tokenID).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			err = NewTokenService().ChangeTokenAccess(context.Background(), ObservabilityScope{ActorUserID: owner}, tokenID, TokenAccessRequest{Action: "resume", ExpectedVersion: "1", Note: "resume after review"})
			if !errors.Is(err, ErrTokenResumeBlocked) {
				t.Fatalf("resume error=%v", err)
			}
			var token model.Token
			if err := db.First(&token, tokenID).Error; err != nil {
				t.Fatal(err)
			}
			if token.Status != 0 || token.AuthVersion != 1 {
				t.Fatal("invalid key became active")
			}
		})
	}
}
