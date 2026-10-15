package open

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/pkg/config"
)

func TestStoredOpenAIImageLoaderUsesAssetStorageSnapshot(t *testing.T) {
	var globalCalled atomic.Bool
	global := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		globalCalled.Store(true)
		http.Error(w, "wrong storage", http.StatusBadGateway)
	}))
	defer global.Close()
	bound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/file/download" || r.URL.Query().Get("url") != "private/object" || r.Header.Get("X-Api-Key") != "asset-key" {
			t.Errorf("unexpected storage request: %s %s", r.Method, r.URL)
			http.Error(w, "wrong request", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte("image bytes"))
	}))
	defer bound.Close()
	previous := config.C
	config.C = &config.Config{FileStorage: config.FileStorageConfig{BaseURL: global.URL, APIKey: "global-key"}}
	t.Cleanup(func() { config.C = previous })

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := repository.New(db)
	mock.ExpectQuery("FROM gw_media_assets").WithArgs(uint64(41), uint64(7), "https://public.example/object", uint64(19)).
		WillReturnRows(sqlmock.NewRows([]string{"xfs_base_url", "xfs_api_key", "xfs_object_locator"}).AddRow(bound.URL, "asset-key", "private/object"))
	asset, err := (storedOpenAIImageLoader{store: store, userID: 41, tokenID: 7, assetIDs: []uint64{19}}).
		LoadImage(context.Background(), "https://public.example/object")
	if err != nil || string(asset.Data) != "image bytes" || globalCalled.Load() {
		t.Fatalf("asset=%q globalCalled=%v err=%v", asset.Data, globalCalled.Load(), err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoredOpenAIImageLoaderRejectsUnlistedLocator(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := repository.New(db)
	mock.ExpectQuery("FROM gw_media_assets").WithArgs(uint64(41), uint64(7), "private/other", uint64(19)).
		WillReturnRows(sqlmock.NewRows([]string{"xfs_base_url", "xfs_api_key", "xfs_object_locator"}))
	_, err = (storedOpenAIImageLoader{store: store, userID: 41, tokenID: 7, assetIDs: []uint64{19}}).
		LoadImage(context.Background(), "private/other")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("err=%v, want ErrNotFound", err)
	}
}
