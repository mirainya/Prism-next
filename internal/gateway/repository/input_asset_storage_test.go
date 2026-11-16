package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestReadOwnedInputAssetStorageScopesRequestAssets(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := New(db)
	mock.ExpectQuery("FROM gw_media_assets").
		WithArgs(uint64(41), uint64(7), "https://public.example/object", uint64(19), uint64(20)).
		WillReturnRows(sqlmock.NewRows([]string{"xfs_base_url", "xfs_api_key", "xfs_object_locator"}).AddRow("https://bound.example", "bound-key", "private/object"))
	got, err := store.ReadOwnedInputAssetStorage(context.Background(), 41, 7, []uint64{19, 20}, "https://public.example/object")
	if err != nil || got.XFSBaseURL != "https://bound.example" || got.APIKey != "bound-key" || got.ObjectLocator != "private/object" {
		t.Fatalf("storage=%+v err=%v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReadCallInputAssetStorageRequiresCallReference(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := New(db)
	mock.ExpectQuery("FROM gw_media_asset_refs mr").
		WithArgs(uint64(63), "private/object").
		WillReturnRows(sqlmock.NewRows([]string{"xfs_base_url", "xfs_api_key", "xfs_object_locator"}))
	_, err = store.ReadCallInputAssetStorage(context.Background(), 63, "private/object")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v, want ErrNotFound", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInputAssetStorageRejectsAmbiguousBindings(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := New(db)
	mock.ExpectQuery("FROM gw_media_assets").
		WithArgs(uint64(41), uint64(7), "shared/object", uint64(19), uint64(20)).
		WillReturnRows(sqlmock.NewRows([]string{"xfs_base_url", "xfs_api_key", "xfs_object_locator"}).
			AddRow("https://first.example", "first-key", "private/first").
			AddRow("https://second.example", "second-key", "private/second"))
	_, err = store.ReadOwnedInputAssetStorage(context.Background(), 41, 7, []uint64{19, 20}, "shared/object")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v, want ErrConflict", err)
	}
}
