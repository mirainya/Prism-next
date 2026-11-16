package admin

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/gateway/security"
	"github.com/mirainya/Prism/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUnifiedCallPayloadScopedDecryptionAndRedaction(t *testing.T) {
	for _, scenario := range []string{"available", "wrong_call", "expired", "decrypt_failure"} {
		t.Run(scenario, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
			if err != nil {
				t.Fatal(err)
			}
			var previous *gorm.DB
			if model.HasDB() {
				previous = model.DB()
			}
			model.SetDB(gdb)
			defer model.SetDB(previous)
			metadata := mock.ExpectQuery("SELECT p.kind,p.retention_until,p.purged_at").WithArgs(uint(12), uint(11))
			rows := sqlmock.NewRows([]string{"kind", "retention_until", "purged_at"})
			if scenario != "wrong_call" {
				expires := time.Now().Add(time.Hour)
				if scenario == "expired" {
					expires = time.Now().Add(-time.Hour)
				}
				rows.AddRow("request", expires, nil)
			}
			metadata.WillReturnRows(rows)
			key := bytes.Repeat([]byte{42}, 32)
			t.Setenv("PRISM_GATEWAY_PAYLOAD_KEK_B64", base64.StdEncoding.EncodeToString(key))
			t.Setenv("PRISM_GATEWAY_PAYLOAD_HMAC_B64", base64.StdEncoding.EncodeToString(key))
			if scenario == "available" || scenario == "decrypt_failure" {
				mock.ExpectQuery("SELECT encrypted_blob_id FROM gw_api_call_payloads").WithArgs(uint64(12), uint64(11), "request").WillReturnRows(sqlmock.NewRows([]string{"encrypted_blob_id"}).AddRow(13))
				plain := []byte(`{"prompt":"keep this prompt","nested":{"api_key":"do-not-display","Authorization":"Bearer hidden"}}`)
				aad, _ := security.CanonicalAAD(13, "gateway-payload", 1, []byte("call:11:request"))
				sealed, err := security.Seal(plain, aad, key, 1)
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "decrypt_failure" {
					sealed.Ciphertext[0] ^= 1
				}
				digest := security.HMACSHA256(key, plain)
				mock.ExpectQuery("SELECT b.keyring_id,b.purpose,b.schema_version").WithArgs(uint64(13)).WillReturnRows(sqlmock.NewRows([]string{"keyring_id", "purpose", "schema_version", "aad_hash", "nonce", "ciphertext", "content_hmac", "kek_version", "wrap_nonce", "wrapped_dek"}).AddRow(1, "gateway-payload", 1, "", sealed.Nonce, sealed.Ciphertext, hex.EncodeToString(digest[:]), 1, sealed.WrapNonce, sealed.WrappedDEK))
			}
			router := gin.New()
			router.GET("/calls/:id/payloads/:payload_id", UnifiedCallPayload)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("GET", "/calls/11/payloads/12", nil))
			body := response.Body.String()
			want := 200
			if scenario == "wrong_call" {
				want = 404
			}
			if scenario == "decrypt_failure" {
				want = 500
			}
			if response.Code != want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, want, body)
			}
			if strings.Contains(body, "do-not-display") || strings.Contains(body, "Bearer hidden") {
				t.Fatal("credential leaked")
			}
			if scenario == "available" && (!strings.Contains(body, "keep this prompt") || !strings.Contains(body, "REDACTED")) {
				t.Fatal("missing sanitized payload")
			}
			if scenario == "expired" && !strings.Contains(body, "expired") {
				t.Fatal("missing expired state")
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("payload can be cached")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
