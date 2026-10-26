package video

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/pkg/config"
	"github.com/mirainya/Prism/pkg/filestorage"
	"github.com/mirainya/Prism/pkg/safeurl"
	"gorm.io/gorm"
)

func newUnifiedAssetTestService(t *testing.T) (*UnifiedAssetService, *gorm.DB, *int) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:unified-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.AIFile{}, &model.MediaAsset{}, &model.MediaAssetRef{}, &model.MediaAssetStateEvent{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE gw_result_deliveries (
id integer PRIMARY KEY, media_asset_ref_id integer, delivery_mode text NOT NULL, state text NOT NULL,
state_version integer NOT NULL, reason_code text, expires_at datetime, updated_at datetime
)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE gw_state_transition_events (
id integer PRIMARY KEY AUTOINCREMENT, result_delivery_id integer NOT NULL, old_state text,
new_state text NOT NULL, state_version integer NOT NULL, reason_code text NOT NULL, created_at datetime NOT NULL
)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE gw_api_calls (
	id integer PRIMARY KEY, user_id integer NOT NULL, token_id integer NOT NULL,
	xfs_base_url text NOT NULL DEFAULT '', xfs_api_key text NOT NULL
)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE gw_api_call_attempts (
id integer PRIMARY KEY, call_id integer NOT NULL
)`).Error; err != nil {
		t.Fatal(err)
	}
	uploads := 0
	service := NewUnifiedAssetService(db)
	service.now = func() time.Time { return time.Date(2026, 9, 7, 7, 0, 0, 0, time.UTC) }
	service.readInputStorage = func(context.Context, uint, uint) (managedResultStorageCredentials, error) {
		return managedResultStorageCredentials{XFSBaseURL: "https://storage.example.test", APIKey: "test-key"}, nil
	}
	service.upload = func(_ context.Context, reader io.Reader, _, _ string) (filestorage.UploadResult, error) {
		uploads++
		if _, err := io.Copy(io.Discard, reader); err != nil {
			return filestorage.UploadResult{}, err
		}
		return filestorage.UploadResult{ID: "object-1", URL: "https://cdn.example.test/video-input.png", Platform: "test"}, nil
	}
	service.verify = func(context.Context, string, int64, string) error { return nil }
	service.remove = func(context.Context, string) error { return nil }
	service.download = func(context.Context, string, int64) (*safeurl.Result, error) {
		return &safeurl.Result{Data: []byte("\x89PNG\r\n\x1a\nremote"), ContentType: "image/png"}, nil
	}
	return service, db, &uploads
}

func TestUnifiedAssetServiceCreatesActiveAssetAndDeduplicates(t *testing.T) {
	service, db, uploads := newUnifiedAssetTestService(t)
	request := &CreateAssetRequest{UserID: 2, TokenID: 7, Kind: "image", ContentType: "image/png", Data: []byte("\x89PNG\r\n\x1a\nasset")}
	first, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.ID != second.ID || first.Status != VideoAssetStatusReady || *uploads != 1 {
		t.Fatalf("first=%#v second=%#v uploads=%d", first, second, *uploads)
	}
	var asset model.MediaAsset
	if err := db.First(&asset, "id=?", first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if asset.State != "active" || asset.StorageLocator != first.StoragePath || asset.Purpose != "input" || asset.UserID != 2 {
		t.Fatalf("stored asset=%#v", asset)
	}
	var events int64
	if err := db.Model(&model.MediaAssetStateEvent{}).Where("media_asset_id=?", asset.ID).Count(&events).Error; err != nil || events != 2 {
		t.Fatalf("events=%d err=%v", events, err)
	}
}

func TestUnifiedAssetServiceCopiesRemoteURL(t *testing.T) {
	service, _, uploads := newUnifiedAssetTestService(t)
	asset, err := service.Create(context.Background(), &CreateAssetRequest{
		UserID: 3, TokenID: 8, Kind: "image", ContentType: "image/png", URL: "https://source.example.test/input.png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if asset.SizeBytes == 0 || asset.StoragePath == "https://source.example.test/input.png" || *uploads != 1 {
		t.Fatalf("asset=%#v uploads=%d", asset, *uploads)
	}
}

func TestUnifiedAssetServicePinsInputStorageAcrossTokenRebinding(t *testing.T) {
	service, db, _ := newUnifiedAssetTestService(t)
	payload := []byte("\x89PNG\r\n\x1a\nasset")
	type storageCounts struct{ uploads, reads, deletes int }
	newStorage := func(name, key string, counts *storageCounts) *httptest.Server {
		t.Helper()
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Api-Key") != key {
				t.Errorf("%s received wrong storage key", name)
				w.WriteHeader(http.StatusForbidden)
				return
			}
			switch r.URL.Path {
			case "/api/v1/upload":
				counts.uploads++
				if err := r.ParseMultipartForm(1 << 20); err != nil {
					t.Error(err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"code":200,"data":{"id":"object-`+name+`","url":"`+server.URL+`/object/`+name+`","rawUrl":"private/object-`+name+`","platform":"test"}}`)
			case "/api/v1/file/download":
				if got := r.URL.Query().Get("url"); got != "private/object-"+name {
					t.Errorf("%s read locator=%q", name, got)
				}
				counts.reads++
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(payload)
			case "/api/v1/file/delete":
				if got := r.URL.Query().Get("url"); got != "private/object-"+name {
					t.Errorf("%s delete locator=%q", name, got)
				}
				counts.deletes++
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"code":200}`)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		return server
	}
	aCounts, bCounts := &storageCounts{}, &storageCounts{}
	a := newStorage("a", "key-a", aCounts)
	b := newStorage("b", "key-b", bCounts)
	defer a.Close()
	defer b.Close()
	selected := managedResultStorageCredentials{XFSBaseURL: a.URL, APIKey: "key-a"}
	service.readInputStorage = func(context.Context, uint, uint) (managedResultStorageCredentials, error) { return selected, nil }
	service.upload, service.verify, service.remove = nil, nil, nil
	request := &CreateAssetRequest{UserID: 2, TokenID: 7, Kind: "image", ContentType: "image/png", Data: payload}
	first, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	selected = managedResultStorageCredentials{XFSBaseURL: b.URL, APIKey: "key-b"}
	second, err := service.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || aCounts.uploads != 1 || bCounts.uploads != 1 || aCounts.reads != 1 || bCounts.reads != 1 {
		t.Fatalf("assets=%q/%q storage A=%+v B=%+v", first.ID, second.ID, aCounts, bCounts)
	}
	var firstStored, secondStored model.MediaAsset
	if err := db.First(&firstStored, "id=?", first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&secondStored, "id=?", second.ID).Error; err != nil {
		t.Fatal(err)
	}
	if firstStored.XFSBaseURL != a.URL || firstStored.XFSAPIKey != "key-a" || firstStored.XFSObjectLocator != "private/object-a" ||
		secondStored.XFSBaseURL != b.URL || secondStored.XFSAPIKey != "key-b" || secondStored.XFSObjectLocator != "private/object-b" {
		t.Fatal("input storage credentials were not pinned to their assets")
	}
	if err := service.Delete(context.Background(), 7, first.ID); err != nil {
		t.Fatal(err)
	}
	if aCounts.deletes != 1 || bCounts.deletes != 0 {
		t.Fatalf("old asset deletion used wrong endpoint: A=%+v B=%+v", aCounts, bCounts)
	}
	if err := db.Model(&model.MediaAsset{}).Where("id=?", second.ID).Update("retention_until", service.now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if removed, err := service.PurgeExpired(context.Background(), 10); err != nil || removed != 1 {
		t.Fatalf("expired input purge=%d err=%v", removed, err)
	}
	if bCounts.deletes != 1 {
		t.Fatalf("new asset cleanup used wrong endpoint: A=%+v B=%+v", aCounts, bCounts)
	}
}

func TestUnifiedAssetServiceProtectsReferencedObject(t *testing.T) {
	service, db, _ := newUnifiedAssetTestService(t)
	asset, err := service.Create(context.Background(), &CreateAssetRequest{
		UserID: 4, TokenID: 9, Kind: "image", ContentType: "image/png", Data: []byte("\x89PNG\r\n\x1a\nreferenced"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var stored model.MediaAsset
	if err := db.First(&stored, "id=?", asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	callID := uint64(12)
	if err := db.Create(&model.MediaAssetRef{MediaAssetID: stored.ID, UserID: 4, TokenID: 9, Role: "input", CallID: &callID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(context.Background(), 9, asset.ID); !errors.Is(err, ErrAssetInUse) {
		t.Fatalf("delete error=%v", err)
	}
	if err := db.First(&stored, "id=?", asset.ID).Error; err != nil || stored.State != "active" {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
}

func TestUnifiedAssetServiceDeletesUnreferencedObject(t *testing.T) {
	service, db, _ := newUnifiedAssetTestService(t)
	var removed string
	service.remove = func(_ context.Context, value string) error { removed = value; return nil }
	asset, err := service.Create(context.Background(), &CreateAssetRequest{
		UserID: 5, TokenID: 10, Kind: "image", ContentType: "image/png", Reader: bytes.NewReader([]byte("\x89PNG\r\n\x1a\ndelete")), SizeBytes: 14,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Delete(context.Background(), 10, asset.ID); err != nil {
		t.Fatal(err)
	}
	var stored model.MediaAsset
	if err := db.First(&stored, "id=?", asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	if removed != asset.StoragePath || stored.State != "deleted" || stored.StorageLocator != "" {
		t.Fatalf("removed=%q stored=%#v", removed, stored)
	}
}

func TestUnifiedAssetServiceCleansFailedVerification(t *testing.T) {
	service, db, _ := newUnifiedAssetTestService(t)
	var removed string
	service.verify = func(context.Context, string, int64, string) error { return errors.New("mismatch") }
	service.remove = func(_ context.Context, value string) error { removed = value; return nil }
	_, err := service.Create(context.Background(), &CreateAssetRequest{
		UserID: 6, TokenID: 11, Kind: "image", ContentType: "image/png", Data: []byte("\x89PNG\r\n\x1a\nbad"),
	})
	if err == nil {
		t.Fatal("verification failure accepted")
	}
	var stored model.MediaAsset
	if err := db.Order("id DESC").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if removed == "" || stored.State != "deleted" || stored.StorageLocator != "" {
		t.Fatalf("removed=%q stored=%#v", removed, stored)
	}
}

func TestUnifiedAssetServiceReportsReferencedExpiredAssetAsExpired(t *testing.T) {
	service, db, _ := newUnifiedAssetTestService(t)
	asset, err := service.Create(context.Background(), &CreateAssetRequest{
		UserID: 7, TokenID: 12, Kind: "image", ContentType: "image/png", Data: []byte("\x89PNG\r\n\x1a\nexpired-reference"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var stored model.MediaAsset
	if err := db.First(&stored, "id=?", asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	expiredAt := service.now().Add(-time.Minute)
	if err := db.Model(&stored).Update("retention_until", expiredAt).Error; err != nil {
		t.Fatal(err)
	}
	callID := uint64(13)
	if err := db.Create(&model.MediaAssetRef{MediaAssetID: stored.ID, UserID: 7, TokenID: 12, Role: "input", CallID: &callID}).Error; err != nil {
		t.Fatal(err)
	}

	got, err := service.Get(context.Background(), 12, asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != VideoAssetStatusExpired || got.StoragePath != "" {
		t.Fatalf("asset=%#v", got)
	}
	if err := db.First(&stored, "id=?", asset.ID).Error; err != nil || stored.State != "active" {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
}

func TestUnifiedAssetServicePurgesExpiredUnreferencedAssets(t *testing.T) {
	service, db, _ := newUnifiedAssetTestService(t)
	var removed []string
	service.remove = func(_ context.Context, value string) error {
		removed = append(removed, value)
		return nil
	}
	asset, err := service.Create(context.Background(), &CreateAssetRequest{
		UserID: 8, TokenID: 13, Kind: "image", ContentType: "image/png", Data: []byte("\x89PNG\r\n\x1a\npurge-expired"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.MediaAsset{}).Where("id=?", asset.ID).Update("retention_until", service.now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}

	count, err := service.PurgeExpired(context.Background(), 10)
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	var stored model.MediaAsset
	if err := db.First(&stored, "id=?", asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != asset.StoragePath || stored.State != "deleted" || stored.StorageLocator != "" {
		t.Fatalf("removed=%v stored=%#v", removed, stored)
	}
}

func TestUnifiedAssetServicePurgesUnreferencedAssetsForEveryPurpose(t *testing.T) {
	service, db, _ := newUnifiedAssetTestService(t)
	removed := make(map[string]bool)
	service.remove = func(_ context.Context, value string) error {
		removed[value] = true
		return nil
	}
	const storageAPIKey = "xfs_0123456789abcdef0123456789abcdef"
	const storageBaseURL = "https://original-storage.example"
	if err := db.Exec("INSERT INTO gw_api_calls(id,user_id,token_id,xfs_base_url,xfs_api_key) VALUES (51,20,21,?,?)", storageBaseURL, storageAPIKey).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO gw_api_call_attempts(id,call_id) VALUES (41,51)").Error; err != nil {
		t.Fatal(err)
	}
	service.removeWithStorage = func(_ context.Context, baseURL, apiKey, value string) error {
		if baseURL != storageBaseURL || apiKey != storageAPIKey {
			t.Fatalf("storage endpoint=%q", baseURL)
		}
		removed[value] = true
		return nil
	}
	expiredAt := service.now().Add(-time.Minute)
	for index, purpose := range []string{"input", "result", "file"} {
		state := "active"
		var attemptID *uint64
		if purpose == "result" {
			state = "staging"
			value := uint64(41)
			attemptID = &value
		}
		asset := model.MediaAsset{
			UserID: 20, TokenID: 21, AttemptID: attemptID, Purpose: purpose, ObjectKey: "expired-" + purpose,
			StorageLocator: "https://cdn.example.test/expired-" + purpose, ContentType: "image/png",
			ContentLength: 16, SHA256: "c2c657685f810899d4c3d826d8aba2123121062ebb0ffff937bbb76aaa8a93e8",
			State: state, StateVersion: uint64(index + 1), RetentionUntil: &expiredAt, CreatedAt: service.now(), UpdatedAt: service.now(),
		}
		if err := db.Create(&asset).Error; err != nil {
			t.Fatal(err)
		}
	}

	count, err := service.PurgeExpired(context.Background(), 10)
	if err != nil || count != 3 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	var deleted int64
	if err := db.Model(&model.MediaAsset{}).Where("state='deleted'").Count(&deleted).Error; err != nil || deleted != 3 || len(removed) != 3 {
		t.Fatalf("deleted=%d removed=%v err=%v", deleted, removed, err)
	}
}

func TestUnifiedAssetServiceExpiresManagedResultAndRetriesWithCallStorageKey(t *testing.T) {
	service, db, _ := newUnifiedAssetTestService(t)
	expiredAt := service.now().Add(-time.Minute)
	const storageAPIKey = "xfs_0123456789abcdef0123456789abcdef"
	const storageBaseURL = "https://original-storage.example"
	if err := db.Exec("INSERT INTO gw_api_calls(id,user_id,token_id,xfs_base_url,xfs_api_key) VALUES (51,30,31,?,?)", storageBaseURL, storageAPIKey).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO gw_api_call_attempts(id,call_id) VALUES (41,51)").Error; err != nil {
		t.Fatal(err)
	}
	attemptID := uint64(41)
	asset := model.MediaAsset{
		UserID: 30, TokenID: 31, AttemptID: &attemptID, Purpose: "result", ObjectKey: "gateway-result:41:0",
		StorageLocator: "https://cdn.example.test/result.mp4", ContentType: "video/mp4", ContentLength: 16,
		SHA256: "c2c657685f810899d4c3d826d8aba2123121062ebb0ffff937bbb76aaa8a93e8",
		State:  "active", StateVersion: 2, RetentionUntil: &expiredAt, CreatedAt: service.now(), UpdatedAt: service.now(),
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO gw_result_deliveries(id,delivery_mode,state,state_version,updated_at) VALUES (71,'managed_copy','ready',2,?)", service.now()).Error; err != nil {
		t.Fatal(err)
	}
	deliveryID := uint64(71)
	ref := model.MediaAssetRef{MediaAssetID: asset.ID, UserID: asset.UserID, TokenID: asset.TokenID, Role: "result", ResultDeliveryID: &deliveryID, CreatedAt: service.now()}
	if err := db.Create(&ref).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE gw_result_deliveries SET media_asset_ref_id=? WHERE id=?", ref.ID, deliveryID).Error; err != nil {
		t.Fatal(err)
	}
	removed := ""
	removeAttempts := 0
	service.remove = func(context.Context, string) error {
		t.Fatal("managed result cleanup used the global storage key")
		return nil
	}
	service.removeWithStorage = func(_ context.Context, baseURL, apiKey, value string) error {
		removeAttempts++
		if baseURL != storageBaseURL || apiKey != storageAPIKey {
			t.Fatalf("storage endpoint=%q", baseURL)
		}
		removed = value
		if removeAttempts == 1 {
			return errors.New("storage unavailable")
		}
		return nil
	}

	count, err := service.PurgeExpired(context.Background(), 10)
	if err == nil || count != 0 {
		t.Fatalf("first cleanup count=%d err=%v", count, err)
	}
	var pending model.MediaAsset
	if err := db.First(&pending, "id=?", asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	var pendingRefs int64
	if err := db.Model(&model.MediaAssetRef{}).Where("media_asset_id=?", asset.ID).Count(&pendingRefs).Error; err != nil {
		t.Fatal(err)
	}
	if pending.State != "deleting" || pendingRefs != 0 {
		t.Fatalf("pending=%+v refs=%d", pending, pendingRefs)
	}

	count, err = service.PurgeExpired(context.Background(), 10)
	if err != nil || count != 1 {
		t.Fatalf("retry cleanup count=%d err=%v", count, err)
	}
	var delivery resultDeliveryLifecycle
	if err := db.Table("gw_result_deliveries").Where("id=?", deliveryID).Take(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	var refs int64
	_ = db.Model(&model.MediaAssetRef{}).Where("media_asset_id=?", asset.ID).Count(&refs).Error
	if removed != asset.StorageLocator || removeAttempts != 2 || delivery.State != "expired" || delivery.MediaAssetRefID != 0 || delivery.StateVersion != 3 || delivery.ExpiresAt == nil || !delivery.ExpiresAt.Equal(expiredAt) || refs != 0 {
		t.Fatalf("removed=%q attempts=%d delivery=%+v refs=%d", removed, removeAttempts, delivery, refs)
	}
}

func TestManagedResultStorageForAssetUsesGlobalEndpointForLegacyCall(t *testing.T) {
	_, db, _ := newUnifiedAssetTestService(t)
	previous := config.C
	config.C = &config.Config{FileStorage: config.FileStorageConfig{BaseURL: "https://legacy-storage.example"}}
	t.Cleanup(func() { config.C = previous })
	if err := db.Exec("INSERT INTO gw_api_calls(id,user_id,token_id,xfs_api_key) VALUES (51,30,31,?)", "legacy-key").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO gw_api_call_attempts(id,call_id) VALUES (41,51)").Error; err != nil {
		t.Fatal(err)
	}
	attemptID := uint64(41)
	storage, err := managedResultStorageForAsset(db, &model.MediaAsset{UserID: 30, TokenID: 31, AttemptID: &attemptID, Purpose: "result"})
	if err != nil || storage.XFSBaseURL != "https://legacy-storage.example" || storage.APIKey != "legacy-key" {
		t.Fatalf("endpoint=%q err=%v", storage.XFSBaseURL, err)
	}
}

func TestUnifiedAssetServiceResumesDeletingFileWithOwningReference(t *testing.T) {
	service, db, _ := newUnifiedAssetTestService(t)
	file := model.AIFile{ID: "file_cleanup", UserID: 40, TokenID: 41, Filename: "input.png", Purpose: "vision", Bytes: 16, MimeType: "image/png", Status: "deleting", CreatedAt: service.now(), UpdatedAt: service.now()}
	if err := db.Create(&file).Error; err != nil {
		t.Fatal(err)
	}
	asset := model.MediaAsset{
		UserID: file.UserID, TokenID: file.TokenID, Purpose: "file", ObjectKey: "file-object",
		StorageLocator: "https://cdn.example.test/file.png", ContentType: "image/png", ContentLength: 16,
		SHA256: "c2c657685f810899d4c3d826d8aba2123121062ebb0ffff937bbb76aaa8a93e8",
		State:  "deleting", StateVersion: 3, CreatedAt: service.now(), UpdatedAt: service.now(),
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatal(err)
	}
	ref := model.MediaAssetRef{MediaAssetID: asset.ID, UserID: asset.UserID, TokenID: asset.TokenID, Role: "file", AIFileID: &file.ID, CreatedAt: service.now()}
	if err := db.Create(&ref).Error; err != nil {
		t.Fatal(err)
	}
	removed := ""
	removeAttempts := 0
	service.remove = func(_ context.Context, value string) error {
		removeAttempts++
		removed = value
		if removeAttempts == 1 {
			return errors.New("storage unavailable")
		}
		return nil
	}

	count, err := service.PurgeExpired(context.Background(), 10)
	if err == nil || count != 0 {
		t.Fatalf("first cleanup count=%d err=%v", count, err)
	}
	var pending model.MediaAsset
	if err := db.First(&pending, "id=?", asset.ID).Error; err != nil {
		t.Fatal(err)
	}
	var pendingRefs int64
	_ = db.Model(&model.MediaAssetRef{}).Where("media_asset_id=?", asset.ID).Count(&pendingRefs).Error
	if pending.State != "deleting" || pendingRefs != 1 {
		t.Fatalf("pending=%+v refs=%d", pending, pendingRefs)
	}

	count, err = service.PurgeExpired(context.Background(), 10)
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if err := db.First(&file, "id=?", file.ID).Error; err != nil {
		t.Fatal(err)
	}
	var refs int64
	_ = db.Model(&model.MediaAssetRef{}).Where("media_asset_id=?", asset.ID).Count(&refs).Error
	if removed != asset.StorageLocator || removeAttempts != 2 || file.Status != "deleted" || refs != 0 {
		t.Fatalf("removed=%q attempts=%d file=%+v refs=%d", removed, removeAttempts, file, refs)
	}
}
