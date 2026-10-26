package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestReadCallInputAssetsReturnsStorageSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := New(db)
	digest := sha256.Sum256([]byte("asset"))
	mock.ExpectQuery(`COALESCE\(NULLIF\(a.xfs_object_locator,''\),NULLIF\(a.storage_locator,''\),a.object_key\)`).
		WithArgs(uint64(17), uint64(23), uint64(29)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ordinal", "object_locator", "xfs_base_url", "xfs_api_key", "content_type", "content_length", "sha256"}).
			AddRow(41, 0, "private/object", "https://bound.example", "bound-key", "image/png", 5, hex.EncodeToString(digest[:])).
			AddRow(42, 1, "https://legacy.example/object", "", "", "image/png", 5, hex.EncodeToString(digest[:])))
	assets, err := store.ReadCallInputAssets(context.Background(), 17, 23, 29)
	if err != nil {
		t.Fatal(err)
	}
	if len(assets) != 2 || assets[0].ObjectLocator != "private/object" || assets[0].XFSBaseURL != "https://bound.example" || assets[0].XFSAPIKey != "bound-key" || assets[1].XFSBaseURL != "" || assets[1].XFSAPIKey != "" {
		t.Fatal("storage snapshot was not returned with the ordered assets")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReadCallInputAssetsRejectsIncompleteStorageSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := New(db)
	digest := sha256.Sum256([]byte("asset"))
	mock.ExpectQuery("FROM gw_media_asset_refs ref").
		WithArgs(uint64(17), uint64(23), uint64(29)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ordinal", "object_locator", "xfs_base_url", "xfs_api_key", "content_type", "content_length", "sha256"}).
			AddRow(41, 0, "private/object", "https://bound.example", "", "image/png", 5, hex.EncodeToString(digest[:])))
	_, err = store.ReadCallInputAssets(context.Background(), 17, 23, 29)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v, want ErrConflict", err)
	}
}
