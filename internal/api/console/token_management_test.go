package console

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mirainya/Prism/internal/api/middleware"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/internal/tokenauth"
	"gorm.io/gorm"
)

var tokenManagementTestID atomic.Uint64

func TestTokenManagementHTTPAndLiveAuthorization(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:token_management_%d?mode=memory&cache=shared", tokenManagementTestID.Add(1))), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Token{}, &model.AuditEvent{}); err != nil {
		t.Fatal(err)
	}
	var previous *gorm.DB
	if model.HasDB() {
		previous = model.DB()
	}
	model.SetDB(db)
	t.Cleanup(func() { model.SetDB(previous); sqlDB, _ := db.DB(); _ = sqlDB.Close() })
	for _, statement := range []string{
		`CREATE TABLE token_auth_state_events(id INTEGER PRIMARY KEY,token_id INTEGER,auth_version INTEGER,old_status INTEGER,new_status INTEGER,reason_code TEXT,actor_user_id INTEGER,created_at DATETIME)`,
		`CREATE TABLE billing_system_state(id INTEGER,currency_code TEXT,currency_version INTEGER)`,
		`INSERT INTO billing_system_state VALUES(1,'CREDIT',1)`,
		`CREATE TABLE billing_accounts(user_id INTEGER,status TEXT,currency_code TEXT,currency_version INTEGER)`,
		`INSERT INTO billing_accounts VALUES(1,'open','CREDIT',1)`,
		`CREATE TABLE token_budget_policy_activations(id INTEGER,token_id INTEGER,policy_id INTEGER,activation_seq INTEGER,effective_at DATETIME)`,
		`CREATE TABLE token_budget_windows(id INTEGER,token_id INTEGER,policy_id INTEGER,activation_id INTEGER,window_start DATETIME,window_end DATETIME,limit_amount TEXT,used_amount TEXT,held_amount TEXT)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&model.User{BaseModel: model.BaseModel{ID: 1}, Username: "token-owner", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	key, selector, digest, err := tokenauth.Generate()
	if err != nil {
		t.Fatal(err)
	}
	token := model.Token{UserID: 1, Name: "owner-key", Selector: selector, SecretDigest: digest, SecretDigestVersion: tokenauth.DigestVersion, KeyHint: tokenauth.KeyHint(key), Status: 1, AuthVersion: 1}
	if err := db.Create(&token).Error; err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Add(-time.Hour)
	if err := db.Exec("INSERT INTO token_budget_policy_activations VALUES(1,?,1,1,?)", token.ID, start).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO token_budget_windows VALUES(1,?,1,1,?,NULL,'10','0','0')", token.ID, start).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/v1/models", middleware.Auth(), func(c *gin.Context) { c.Status(204) })
	console := router.Group("/api", func(c *gin.Context) {
		actor := uint(1)
		if c.GetHeader("Test-Actor") == "other" {
			actor = 2
		}
		if c.GetHeader("Test-Actor") == "anonymous" {
			actor = 0
		}
		c.Set(middleware.ContextKeyUserID, actor)
		c.Set(middleware.ContextKeyUserRole, c.GetHeader("Test-Role"))
	})
	RegisterRoutes(console)
	console.GET("/token-probe/:token_id", func(c *gin.Context) {
		if _, ok := getPlaygroundToken(c); ok {
			c.Status(204)
		}
	})
	request := func(method, path, body, actor, role string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Test-Actor", actor)
		r.Header.Set("Test-Role", role)
		if strings.HasPrefix(path, "/v1/") {
			r.Header.Set("Authorization", "Bearer "+key)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	assertStatus := func(method, path, body, actor, role string, want int) *httptest.ResponseRecorder {
		t.Helper()
		w := request(method, path, body, actor, role)
		if w.Code != want {
			t.Fatalf("%s %s => %d want %d: %s", method, path, w.Code, want, w.Body)
		}
		return w
	}
	path := fmt.Sprintf("/api/tokens/%d/access", token.ID)
	pause := `{"action":"pause","expected_version":"1","note":"HTTP permission test"}`
	assertStatus("GET", "/v1/models", "", "", "", 204)
	assertStatus("POST", path, pause, "other", "user", 404)
	for _, body := range []string{`{}`, `{"action":"pause","expected_version":"1"}`, `{"action":"pause","expected_version":"0","note":"test"}`, `{"action":"delete","expected_version":"1","note":"test"}`} {
		assertStatus("POST", path, body, "", "user", 400)
	}
	assertStatus("GET", "/api/tokens/management?user_id=2", "", "", "user", 403)
	assertStatus("GET", "/api/tokens/management", "", "anonymous", "", 403)
	assertStatus("POST", path, pause, "", "user", 200)
	assertStatus("GET", "/v1/models", "", "", "", 401)
	assertStatus("POST", path, pause, "", "user", 409)
	paused := assertStatus("GET", "/api/tokens/management?state=paused", "", "", "user", 200)
	if !strings.Contains(paused.Body.String(), `"state":"paused"`) || strings.Contains(paused.Body.String(), key) {
		t.Fatal("paused history missing or plaintext key exposed")
	}
	assertStatus("POST", path, `{"action":"resume","expected_version":"2","note":"restored"}`, "", "user", 200)
	assertStatus("GET", "/v1/models", "", "", "", 204)
	assertStatus("POST", path, `{"action":"revoke","expected_version":"3","note":"retired"}`, "other", "admin", 200)
	assertStatus("GET", "/v1/models", "", "", "", 401)
	assertStatus("POST", path, `{"action":"resume","expected_version":"4","note":"cannot restore"}`, "", "user", 409)
	historyPath := fmt.Sprintf("/api/tokens/%d/access-history", token.ID)
	assertStatus("GET", historyPath, "", "other", "user", 404)
	history := assertStatus("GET", historyPath, "", "other", "admin", 200)
	if !strings.Contains(history.Body.String(), `"reason_code":"admin_revoke"`) {
		t.Fatal("admin revocation history missing")
	}
	assertStatus("PUT", fmt.Sprintf("/api/tokens/%d/policy", token.ID), `{"expected_version":"4","rate_limit":1,"expires_at":null,"note":"must stay revoked"}`, "", "user", 409)
	// Exercise the public API and Playground against the same policy changes.
	key, selector, digest, err = tokenauth.Generate()
	if err != nil {
		t.Fatal(err)
	}
	second := model.Token{UserID: 1, Name: "policy-key", Selector: selector, SecretDigest: digest, SecretDigestVersion: tokenauth.DigestVersion, Status: 1, AuthVersion: 1}
	if err := db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	policyPath := fmt.Sprintf("/api/tokens/%d/policy", second.ID)
	probePath := fmt.Sprintf("/api/token-probe/%d", second.ID)
	for _, body := range []string{
		`{"expected_version":"1","rate_limit":1,"note":"missing expiration"}`,
		`{"expected_version":"1","expires_at":null,"note":"missing rate"}`,
		`{"expected_version":"1","rate_limit":-1,"expires_at":null,"note":"bad rate"}`,
		`{"expected_version":"1","rate_limit":1,"expires_at":"2020-01-01T00:00:00Z","note":"past date"}`,
		`{"expected_version":"1","rate_limit":1,"expires_at":null}`,
	} {
		assertStatus("PUT", policyPath, body, "", "user", 400)
	}
	body := fmt.Sprintf(`{"expected_version":"1","rate_limit":3,"expires_at":%q,"note":"short-lived access"}`, time.Now().UTC().Add(time.Hour).Format(time.RFC3339))
	assertStatus("PUT", policyPath, body, "other", "user", 404)
	assertStatus("PUT", policyPath, body, "other", "admin", 200)
	assertStatus("PUT", policyPath, body, "", "user", 409)
	assertStatus("GET", "/v1/models", "", "", "", 204)
	assertStatus("GET", probePath, "", "", "user", 204)
	assertStatus("GET", probePath, "", "other", "admin", 404)
	for _, invalid := range []map[string]any{
		{"expires_at": time.Now().UTC().Add(-time.Second)},
		{"expires_at": nil, "status": 0},
		{"status": 1, "revoked_at": time.Now().UTC()},
		{"revoked_at": nil, "auth_version": 0},
	} {
		if err := db.Model(&model.Token{}).Where("id=?", second.ID).Updates(invalid).Error; err != nil {
			t.Fatal(err)
		}
		assertStatus("GET", "/v1/models", "", "", "", 401)
		assertStatus("GET", probePath, "", "", "user", 404)
	}
}
