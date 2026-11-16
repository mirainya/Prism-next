package console

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/mirainya/Prism/internal/api/middleware"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/pkg/config"
	"github.com/mirainya/Prism/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestPlaygroundUploadUsesSelectedTokenStorage(t *testing.T) {
	previousLogger := logger.L
	logger.L = zap.NewNop()
	t.Cleanup(func() { logger.L = previousLogger })
	for _, tc := range []struct {
		name                            string
		bound, legacy, noGlobal, failed bool
		locator, signingFailed          bool
		rawLocator, thumbnailFailed     bool
		actor                           uint
		wantStatus                      int
	}{
		{name: "bound", bound: true, actor: 42, wantStatus: 200},
		{name: "bound_without_global", bound: true, noGlobal: true, actor: 42, wantStatus: 200},
		{name: "unbound", actor: 42, wantStatus: 200},
		{name: "legacy_endpoint", bound: true, legacy: true, actor: 42, wantStatus: 200},
		{name: "bound_failure", bound: true, failed: true, actor: 42, wantStatus: 502},
		{name: "wrong_owner", bound: true, actor: 43, wantStatus: 404},
		{name: "bound_object_locator", bound: true, locator: true, actor: 42, wantStatus: 200},
		{name: "global_object_locator", locator: true, actor: 42, wantStatus: 200},
		{name: "signing_failure", bound: true, locator: true, signingFailed: true, actor: 42, wantStatus: 502},
		{name: "durable_raw_locator", bound: true, locator: true, rawLocator: true, actor: 42, wantStatus: 200},
		{name: "thumbnail_failure", bound: true, locator: true, thumbnailFailed: true, actor: 42, wantStatus: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var globalCalls, boundCalls atomic.Int32
			handler := func(calls *atomic.Int32, expectedKey string) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Header.Get("X-Api-Key") != expectedKey {
						t.Error("wrong storage credential")
					}
					if r.Method == http.MethodGet && r.URL.Path == "/api/v1/file/presigned-url" {
						if tc.signingFailed {
							w.WriteHeader(http.StatusBadGateway)
							return
						}
						location := r.URL.Query().Get("url")
						if tc.thumbnailFailed && location == "Prism/thumb.png" {
							w.WriteHeader(http.StatusInternalServerError)
							return
						}
						if tc.rawLocator && location == "Prism/input.png" {
							t.Error("used display path instead of durable raw locator")
						}
						if tc.rawLocator && location == "private/input.png" {
							location = "Prism/input.png"
						}
						if (location != "Prism/input.png" && location != "Prism/thumb.png") || r.URL.Query().Get("expiration") != "3600" {
							t.Error("wrong signing locator or expiration")
						}
						_, _ = fmt.Fprintf(w, `{"code":200,"data":"https://files.example/%s?signature=bound"}`, location)
						return
					}
					if r.Method != http.MethodPost || r.URL.Path != "/api/v1/upload-image" {
						t.Error("upload sent to wrong endpoint or credential")
					}
					if err := r.ParseMultipartForm(1 << 20); err != nil {
						t.Error(err)
					}
					defer r.MultipartForm.RemoveAll()
					f, _, err := r.FormFile("file")
					if err != nil {
						t.Error(err)
						return
					}
					defer f.Close()
					data, _ := io.ReadAll(f)
					if string(data) != "test image bytes" {
						t.Error("upload bytes changed")
					}
					if tc.failed {
						w.WriteHeader(http.StatusBadGateway)
						_, _ = io.WriteString(w, `{"code":502,"message":"unavailable"}`)
						return
					}
					if tc.locator {
						rawURL := ""
						if tc.rawLocator {
							rawURL = "private/input.png"
						}
						_, _ = fmt.Fprintf(w, `{"code":200,"data":{"url":"Prism/input.png","rawUrl":%q,"thUrl":"Prism/thumb.png","filename":"input.png"}}`, rawURL)
						return
					}
					_, _ = io.WriteString(w, `{"code":200,"data":{"url":"https://files.example/input.png","filename":"input.png"}}`)
				}
			}
			expectedGlobalKey := "global-key"
			if tc.legacy {
				expectedGlobalKey = "bound-key"
			}
			global := httptest.NewServer(handler(&globalCalls, expectedGlobalKey))
			defer global.Close()
			bound := httptest.NewServer(handler(&boundCalls, "bound-key"))
			defer bound.Close()
			previousConfig := config.C
			config.C = &config.Config{FileStorage: config.FileStorageConfig{BaseURL: global.URL, APIKey: "global-key"}}
			if tc.noGlobal {
				config.C.FileStorage.BaseURL, config.C.FileStorage.APIKey = "", ""
			}
			t.Cleanup(func() { config.C = previousConfig })
			db, err := gorm.Open(sqlite.Open("file:upload-"+tc.name+"?mode=memory&cache=shared"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&model.Token{}); err != nil {
				t.Fatal(err)
			}
			var previousDB *gorm.DB
			if model.HasDB() {
				previousDB = model.DB()
			}
			model.SetDB(db)
			t.Cleanup(func() { model.SetDB(previousDB); sqlDB, _ := db.DB(); _ = sqlDB.Close() })
			token := model.Token{UserID: 42, Name: "selected-key", Status: 1}
			if tc.bound {
				token.XFSAPIKey, token.XFSBaseURL = "bound-key", bound.URL+"/"
				if tc.legacy {
					token.XFSBaseURL = ""
				}
			}
			if err := db.Create(&token).Error; err != nil {
				t.Fatal(err)
			}
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			part, _ := writer.CreatePart(textproto.MIMEHeader{"Content-Disposition": {`form-data; name="file"; filename="input.png"`}, "Content-Type": {"image/png"}})
			_, _ = io.WriteString(part, "test image bytes")
			_ = writer.Close()
			router := gin.New()
			router.POST("/upload/:token_id", func(c *gin.Context) { c.Set(middleware.ContextKeyUserID, tc.actor); PlaygroundUploadFile(c) })
			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/upload/%d", token.ID), &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.wantStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if tc.locator && response.Code == http.StatusOK {
				var result struct {
					Data uploadResult `json:"data"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				wantThumbnail := "https://files.example/Prism/thumb.png?signature=bound"
				if tc.thumbnailFailed {
					wantThumbnail = result.Data.URL
				}
				if result.Data.URL != "https://files.example/Prism/input.png?signature=bound" || result.Data.ThURL != wantThumbnail {
					t.Fatalf("object locators not signed: %s", response.Body.String())
				}
			}
			var wantGlobal, wantBound int32
			if tc.actor == 42 {
				var wantCalls int32 = 1
				if tc.locator {
					wantCalls = 3
				}
				if tc.signingFailed {
					wantCalls = 2
				}
				if tc.bound && !tc.legacy {
					wantBound = wantCalls
				} else {
					wantGlobal = wantCalls
				}
			}
			if globalCalls.Load() != wantGlobal || boundCalls.Load() != wantBound {
				t.Fatalf("global calls=%d bound calls=%d", globalCalls.Load(), boundCalls.Load())
			}
		})
	}
}
