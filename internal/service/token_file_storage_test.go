package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/pkg/config"
	"gorm.io/gorm"
)

const (
	testXFSKeyOne = "xfs_0123456789abcdef0123456789abcdef"
	testXFSKeyTwo = "xfs_fedcba9876543210fedcba9876543210"
)

func TestTokenFileStorageReplacesCurrentKey(t *testing.T) {
	setTokenFileStorageConfig(t, config.FileStorageConfig{
		BaseURL:         "https://legacy.example",
		AllowedBaseURLs: []string{"https://storage-one.example/", "https://storage-two.example"},
	})
	db := setupTestDB(t)
	setupUnifiedFundsSchema(t, db)
	owner := &model.User{Username: "storage-owner", Status: 1}
	other := &model.User{Username: "storage-other", Status: 1}
	if err := db.Create(owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(other).Error; err != nil {
		t.Fatal(err)
	}
	token := &model.Token{UserID: owner.ID, Selector: "storage-selector", SecretDigest: make([]byte, 32), SecretDigestVersion: 1, AuthVersion: 1, KeyHint: "****test", Name: "media", Status: 1}
	if err := db.Create(token).Error; err != nil {
		t.Fatal(err)
	}
	seedTokenBudgetForStorageTest(t, db, token.ID)

	service := NewTokenService()
	var probed []string
	service.probeFileStorage = func(_ context.Context, baseURL, apiKey string) error {
		probed = append(probed, baseURL)
		return nil
	}
	if status, err := service.BindFileStorage(context.Background(), owner.ID, token.ID, "https://storage-one.example/", testXFSKeyOne); err != nil || !status.Configured || status.KeyHint != "****cdef" || status.BaseURL != "https://storage-one.example" {
		t.Fatalf("first bind status=%+v err=%v", status, err)
	}
	if status, err := service.BindFileStorage(context.Background(), owner.ID, token.ID, "https://storage-two.example", testXFSKeyTwo); err != nil || !status.Configured || status.KeyHint != "****3210" || status.BaseURL != "https://storage-two.example" {
		t.Fatalf("replacement status=%+v err=%v", status, err)
	}
	if err := db.First(token, token.ID).Error; err != nil {
		t.Fatal(err)
	}
	if token.XFSAPIKey != testXFSKeyTwo || token.XFSBaseURL != "https://storage-two.example" || len(probed) != 2 || probed[0] != "https://storage-one.example" || probed[1] != "https://storage-two.example" {
		t.Fatal("stored binding or probe destinations differ from approved endpoints")
	}

	list, err := service.ListTokens(owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertTokenStorageStatus(t, list[0]["xfs_storage"], true, "****3210", "https://storage-two.example")
	encoded, _ := json.Marshal(list)
	if strings.Contains(string(encoded), testXFSKeyOne) || strings.Contains(string(encoded), testXFSKeyTwo) {
		t.Fatalf("token response exposed XFS key: %s", encoded)
	}
	if _, err := service.BindFileStorage(context.Background(), other.ID, token.ID, "https://storage-one.example", testXFSKeyOne); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("bind as another owner error=%v", err)
	}
	if len(probed) != 2 {
		t.Fatalf("unowned token triggered probe: %#v", probed)
	}
	if _, err := service.UnbindFileStorage(context.Background(), owner.ID, token.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.First(token, token.ID).Error; err != nil {
		t.Fatal(err)
	}
	if token.XFSAPIKey != "" || token.XFSBaseURL != "" {
		t.Fatal("binding remained after unbind")
	}
}

func TestTokenFileStorageRejectsMalformedKeys(t *testing.T) {
	service := NewTokenService()
	for _, apiKey := range []string{"", "xfs_short", "xfs_0123456789ABCDEF0123456789ABCDEF", "xfs_0123456789abcdef0123456789abcdeg"} {
		if _, err := service.BindFileStorage(context.Background(), 1, 1, "https://storage.example", apiKey); !errors.Is(err, ErrInvalidFileStorageAPIKey) {
			t.Fatalf("key %q error=%v", apiKey, err)
		}
	}
}

func TestTokenFileStorageFailedProbeKeepsCurrentKey(t *testing.T) {
	setTokenFileStorageConfig(t, config.FileStorageConfig{AllowedBaseURLs: []string{"https://storage-one.example", "https://storage-two.example"}})
	db := setupTestDB(t)
	owner := &model.User{Username: "storage-probe-owner", Status: 1}
	if err := db.Create(owner).Error; err != nil {
		t.Fatal(err)
	}
	token := &model.Token{UserID: owner.ID, XFSAPIKey: testXFSKeyTwo, XFSBaseURL: "https://storage-two.example", Selector: "storage-probe-selector", SecretDigest: make([]byte, 32), SecretDigestVersion: 1, AuthVersion: 1, KeyHint: "****test", Name: "probe", Status: 1}
	if err := db.Create(token).Error; err != nil {
		t.Fatal(err)
	}
	service := NewTokenService()
	service.probeFileStorage = func(context.Context, string, string) error { return errors.New("probe denied") }
	if _, err := service.BindFileStorage(context.Background(), owner.ID, token.ID, "https://storage-one.example", testXFSKeyOne); !errors.Is(err, ErrFileStorageProbeFailed) {
		t.Fatalf("bind error=%v", err)
	}
	if err := db.First(token, token.ID).Error; err != nil {
		t.Fatal(err)
	}
	if token.XFSAPIKey != testXFSKeyTwo || token.XFSBaseURL != "https://storage-two.example" {
		t.Fatal("failed probe changed current binding")
	}
}

func TestTokenFileStorageRejectsUnapprovedBaseURLWithoutProbe(t *testing.T) {
	setTokenFileStorageConfig(t, config.FileStorageConfig{AllowedBaseURLs: []string{"https://storage.example"}})
	db := setupTestDB(t)
	owner := &model.User{Username: "storage-url-owner", Status: 1}
	if err := db.Create(owner).Error; err != nil {
		t.Fatal(err)
	}
	token := &model.Token{UserID: owner.ID, Selector: "storage-url-selector", SecretDigest: make([]byte, 32), SecretDigestVersion: 1, AuthVersion: 1, KeyHint: "****test", Name: "url", Status: 1}
	if err := db.Create(token).Error; err != nil {
		t.Fatal(err)
	}
	service := NewTokenService()
	service.probeFileStorage = func(context.Context, string, string) error {
		t.Fatal("invalid endpoint reached storage probe")
		return nil
	}
	for _, baseURL := range []string{"", "http://storage.example", "https://storage.example.evil", "https://user@storage.example", "https://storage.example?x=1", "https://storage.example#section", "https://storage.example/../other"} {
		if _, err := service.BindFileStorage(context.Background(), owner.ID, token.ID, baseURL, testXFSKeyOne); !errors.Is(err, ErrInvalidFileStorageBaseURL) {
			t.Errorf("unexpected result for rejected endpoint: %v", err)
		}
	}
}

func TestTokenFileStorageLegacyGlobalURLAndFallback(t *testing.T) {
	setTokenFileStorageConfig(t, config.FileStorageConfig{BaseURL: "http://legacy.example/"})
	service := NewTokenService()
	status := newTokenFileStorageStatus("", testXFSKeyOne)
	if !status.Configured || status.BaseURL != "http://legacy.example" {
		t.Fatalf("legacy status=%+v", status)
	}
	endpoints, err := service.FileStorageEndpoints()
	if err != nil || len(endpoints) != 1 || endpoints[0] != "http://legacy.example" {
		t.Fatalf("fallback endpoints=%v err=%v", endpoints, err)
	}
	if resolved, err := config.Get().FileStorage.ResolveTokenBindingBaseURL("http://legacy.example/"); err != nil || resolved != "http://legacy.example" {
		t.Fatalf("fallback resolve=%q err=%v", resolved, err)
	}
}

func TestTokenFileStorageEndpointConfigurationIsStrict(t *testing.T) {
	for _, endpoint := range []string{"http://storage.example", "https://user@storage.example", "https://storage.example?", "https://storage.example/%2e%2e/", "https://storage.example/a b"} {
		cfg := config.FileStorageConfig{AllowedBaseURLs: []string{endpoint}}
		if _, err := cfg.TokenBindingBaseURLs(); !errors.Is(err, ErrInvalidFileStorageBaseURL) {
			t.Errorf("invalid approved endpoint accepted: %v", err)
		}
	}
	cfg := config.FileStorageConfig{AllowedBaseURLs: []string{"HTTPS://STORAGE.EXAMPLE/", "https://storage.example"}}
	endpoints, err := cfg.TokenBindingBaseURLs()
	if err != nil || len(endpoints) != 1 || endpoints[0] != "https://storage.example" {
		t.Fatalf("normalized endpoints=%v err=%v", endpoints, err)
	}
}

func setTokenFileStorageConfig(t *testing.T, storage config.FileStorageConfig) {
	t.Helper()
	previous := config.C
	config.C = &config.Config{FileStorage: storage}
	t.Cleanup(func() { config.C = previous })
}

func seedTokenBudgetForStorageTest(t *testing.T, db *gorm.DB, tokenID uint) {
	t.Helper()
	now := time.Now().UTC()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO token_budget_policies(id,token_id,policy_code,window_kind,limit_amount,timezone_name,algorithm_version,created_at) VALUES (1,?,'default-lifetime','lifetime','0','UTC',1,?)`, []any{tokenID, now}},
		{`INSERT INTO token_budget_policy_activations(id,token_id,policy_id,activation_seq,effective_at,created_at) VALUES (1,?,1,1,?,?)`, []any{tokenID, now, now}},
		{`INSERT INTO token_budget_windows(id,token_id,policy_id,activation_id,window_start,limit_amount,used_amount,held_amount,created_at) VALUES (1,?,1,1,?,'0','0','0',?)`, []any{tokenID, now, now}},
	} {
		if err := db.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func assertTokenStorageStatus(t *testing.T, value any, configured bool, hint, baseURL string) {
	t.Helper()
	status, ok := value.(TokenFileStorageStatus)
	if !ok || status.Configured != configured || status.KeyHint != hint || status.BaseURL != baseURL {
		t.Fatalf("storage status=%+v", value)
	}
}
