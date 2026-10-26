package runtime

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

func TestStoredAsyncAssetLoaderUsesReferencedAssetStorageSnapshot(t *testing.T) {
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
	mock.ExpectQuery("FROM gw_media_asset_refs mr").WithArgs(uint64(63), "https://public.example/object").
		WillReturnRows(sqlmock.NewRows([]string{"xfs_base_url", "xfs_api_key", "xfs_object_locator"}).AddRow(bound.URL, "asset-key", "private/object"))
	asset, err := (storedAsyncAssetLoader{store: store, callID: 63}).LoadAsyncAsset(context.Background(), "https://public.example/object")
	if err != nil || string(asset.Data) != "image bytes" || globalCalled.Load() {
		t.Fatalf("asset=%q globalCalled=%v err=%v", asset.Data, globalCalled.Load(), err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStoredAsyncAssetLoaderRejectsUnreferencedLocator(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := repository.New(db)
	mock.ExpectQuery("FROM gw_media_asset_refs mr").WithArgs(uint64(63), "private/other").
		WillReturnRows(sqlmock.NewRows([]string{"xfs_base_url", "xfs_api_key", "xfs_object_locator"}))
	_, err = (storedAsyncAssetLoader{store: store, callID: 63}).LoadAsyncAsset(context.Background(), "private/other")
	if !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("err=%v, want ErrConflict", err)
	}
}

func TestAsyncImageLoaderReadsPublicURLWithoutXFSBinding(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := repository.New(db)
	location := "https://public.example/input.png?signature=original"
	mock.ExpectQuery("FROM gw_media_asset_refs mr").WithArgs(uint64(63), location).
		WillReturnRows(sqlmock.NewRows([]string{"xfs_base_url", "xfs_api_key", "xfs_object_locator"}))
	asset, err := (storedAsyncAssetLoader{store: store, callID: 63}).loadAsyncAsset(context.Background(), location,
		func(_ context.Context, url string) (AsyncAsset, error) {
			if url != location {
				t.Fatalf("URL replaced: %s", url)
			}
			return AsyncAsset{Data: []byte("remote image bytes"), ContentType: "image/png"}, nil
		})
	if err != nil || string(asset.Data) != "remote image bytes" {
		t.Fatalf("asset=%+v err=%v", asset, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAsyncImageLoaderDoesNotDownloadAfterStorageConflict(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := repository.New(db)
	location := "https://public.example/input.png"
	mock.ExpectQuery("FROM gw_media_asset_refs mr").WithArgs(uint64(63), location).
		WillReturnRows(sqlmock.NewRows([]string{"xfs_base_url", "xfs_api_key", "xfs_object_locator"}).AddRow("https://storage.example", "", "private/object"))
	_, err = (storedAsyncAssetLoader{store: store, callID: 63}).loadAsyncAsset(context.Background(), location,
		func(context.Context, string) (AsyncAsset, error) {
			t.Fatal("storage conflict bypassed")
			return AsyncAsset{}, nil
		})
	if !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
