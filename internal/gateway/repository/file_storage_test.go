package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/pkg/config"
)

func TestReadTokenFileStorageReturnsCurrentEndpointAndKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := New(db)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE\\(xfs_base_url,''\\),COALESCE\\(xfs_api_key,''\\) FROM tokens").
		WithArgs(uint64(13), uint64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"xfs_base_url", "xfs_api_key"}).AddRow("https://token-storage.example", "xfs_0123456789abcdef0123456789abcdef"))
	mock.ExpectCommit()

	var storage TokenFileStorage
	err = store.WithTx(context.Background(), func(tx *sql.Tx) error {
		storage, err = store.ReadTokenFileStorage(context.Background(), tx, 11, 13)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if storage.UserID != 11 || storage.TokenID != 13 || storage.XFSBaseURL != "https://token-storage.example" || storage.APIKey != "xfs_0123456789abcdef0123456789abcdef" {
		t.Fatalf("storage=%+v", storage)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReadTokenFileStorageAllowsEmptyKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := New(db)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE\\(xfs_base_url,''\\),COALESCE\\(xfs_api_key,''\\) FROM tokens").
		WithArgs(uint64(13), uint64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"xfs_base_url", "xfs_api_key"}).AddRow("", ""))
	mock.ExpectCommit()

	var storage TokenFileStorage
	err = store.WithTx(context.Background(), func(tx *sql.Tx) error {
		storage, err = store.ReadTokenFileStorage(context.Background(), tx, 11, 13)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if storage.XFSBaseURL != "" || storage.APIKey != "" {
		t.Fatalf("storage=%+v", storage)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReadAttemptFileStorageReadsCallSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := New(db)

	mock.ExpectQuery("SELECT c.user_id,c.token_id,COALESCE\\(c.xfs_base_url,''\\),COALESCE\\(c.xfs_api_key,''\\)").
		WithArgs(uint64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "token_id", "xfs_base_url", "xfs_api_key"}).
			AddRow(11, 13, "https://original-storage.example", "xfs_0123456789abcdef0123456789abcdef"))

	storage, err := store.ReadAttemptFileStorage(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if storage.UserID != 11 || storage.TokenID != 13 || storage.XFSBaseURL != "https://original-storage.example" || storage.APIKey != "xfs_0123456789abcdef0123456789abcdef" {
		t.Fatalf("storage=%+v", storage)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReadAttemptFileStorageFallsBackForLegacyCall(t *testing.T) {
	previous := config.C
	config.C = &config.Config{FileStorage: config.FileStorageConfig{BaseURL: "https://legacy-storage.example"}}
	t.Cleanup(func() { config.C = previous })
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := New(db)
	mock.ExpectQuery("SELECT c.user_id,c.token_id,COALESCE\\(c.xfs_base_url,''\\)").WithArgs(uint64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "token_id", "xfs_base_url", "xfs_api_key"}).
			AddRow(11, 13, "", "xfs_0123456789abcdef0123456789abcdef"))
	storage, err := store.ReadAttemptFileStorage(context.Background(), 7)
	if err != nil || storage.XFSBaseURL != "https://legacy-storage.example" {
		t.Fatalf("endpoint=%q err=%v", storage.XFSBaseURL, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
