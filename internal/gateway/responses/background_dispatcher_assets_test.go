package responses

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/canonical"
	"github.com/mirainya/Prism/internal/gateway/repository"
	gatewayruntime "github.com/mirainya/Prism/internal/gateway/runtime"
	"github.com/mirainya/Prism/pkg/config"
)

func TestBackgroundDispatcherReadsInputUsingAssetStorageSnapshot(t *testing.T) {
	boundContent := []byte("bound")
	legacyContent := []byte("legacy")
	bound := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/file/download" || r.URL.Query().Get("url") != "private/bound" || r.Header.Get("X-Api-Key") != "bound-key" {
			http.Error(w, "wrong storage request", http.StatusBadRequest)
			return
		}
		_, _ = w.Write(boundContent)
	}))
	defer bound.Close()
	legacy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/file/download" || r.URL.Query().Get("url") != "https://legacy.example/object" || r.Header.Get("X-Api-Key") != "legacy-key" {
			http.Error(w, "wrong legacy storage request", http.StatusBadRequest)
			return
		}
		_, _ = w.Write(legacyContent)
	}))
	defer legacy.Close()
	previousConfig := config.C
	config.C = &config.Config{FileStorage: config.FileStorageConfig{BaseURL: legacy.URL, APIKey: "legacy-key"}}
	t.Cleanup(func() { config.C = previousConfig })

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := repository.New(db)
	boundDigest := sha256.Sum256(boundContent)
	legacyDigest := sha256.Sum256(legacyContent)
	mock.ExpectQuery("FROM gw_media_asset_refs ref").
		WithArgs(uint64(17), uint64(23), uint64(29)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ordinal", "object_locator", "xfs_base_url", "xfs_api_key", "content_type", "content_length", "sha256"}).
			AddRow(41, 0, "private/bound", bound.URL, "bound-key", "image/png", len(boundContent), hex.EncodeToString(boundDigest[:])).
			AddRow(42, 1, "https://legacy.example/object", "", "", "image/png", len(legacyContent), hex.EncodeToString(legacyDigest[:])))
	dispatcher := &BackgroundDispatcher{service: &gatewayruntime.Service{Store: store}}
	request := canonical.Request{Items: []canonical.Item{
		{Content: []canonical.Content{{Type: "input_image", FileID: "file_bound"}}},
		{Content: []canonical.Content{{Type: "input_image", FileID: "file_legacy"}}},
	}}
	resolved, err := dispatcher.resolveInputAssets(context.Background(), repository.DeferredDispatch{CallID: 17, UserID: 23, TokenID: 29}, request)
	if err != nil {
		t.Fatal(err)
	}
	for index, content := range []struct {
		payload []byte
	}{
		{boundContent}, {legacyContent},
	} {
		got := resolved.Items[index].Content[0]
		want := "data:image/png;base64," + base64.StdEncoding.EncodeToString(content.payload)
		if got.URL != want || got.FileID != "" || request.Items[index].Content[0].FileID == "" {
			t.Fatalf("item %d was not resolved from its original storage", index)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
