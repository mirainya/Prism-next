package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestCredentialCatalogPreparationRequiresActorAndStrictConfiguration(t *testing.T) {
	router := gin.New()
	router.POST("/credentials/:id/catalog-source", PrepareUnifiedCredentialCatalogSource)
	valid := `{"base_url":"https://provider.invalid","external_group":"default","request_timeout_ms":15000,"protocol_definition_id":70}`
	for _, test := range []struct {
		body   string
		status int
	}{
		{valid, http.StatusForbidden},
		{"null", http.StatusBadRequest},
		{strings.Replace(valid, `"protocol_definition_id":70`, `"protocol_definition_id":0`, 1), http.StatusBadRequest},
		{strings.Replace(valid, `provider.invalid`, `provider.invalid/v1`, 1), http.StatusBadRequest},
		{strings.Replace(valid, `"base_url"`, `"credential_id":99,"base_url"`, 1), http.StatusBadRequest},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("POST", "/credentials/40/catalog-source", strings.NewReader(test.body)))
		if response.Code != test.status {
			t.Fatalf("code=%d body=%s", response.Code, response.Body)
		}
	}
}

func TestCatalogSourcesReadIsScopedToActualKey(t *testing.T) {
	mock := unifiedChannelMock(t)
	mock.ExpectQuery("SELECT COUNT.*s.credential_id=").WithArgs("", "", uint64(40)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	mock.ExpectQuery("SELECT s.id,s.channel_id.*s.credential_id=").WithArgs("", "", uint64(40), 20, 0).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	router := gin.New()
	router.GET("/catalog-sources", ListUnifiedCatalogSources)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/catalog-sources?credential_id=40", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", response.Code, response.Body)
	}
}

func TestCatalogSnapshotSearchUsesParametersAndPreservesUnknownPrices(t *testing.T) {
	mock := unifiedChannelMock(t)
	query := "model%_'"
	mock.ExpectQuery("SELECT COUNT.*LOCATE").WithArgs(uint64(61), query, query).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(1))
	mock.ExpectQuery("SELECT i.id,i.ordinal.*LOCATE").WithArgs(uint64(61), query, query, 20, 0).WillReturnRows(sqlmock.NewRows([]string{
		"id", "ordinal", "model_code", "description", "tags", "vendor_id", "quota_type", "price", "ratio", "completion_ratio", "owner", "version", "selected", "groups", "endpoints",
	}).AddRow(1, 0, query, "Original", "", nil, nil, nil, nil, nil, "", "", true, "default", ""))
	router := gin.New()
	router.GET("/snapshots/:snapshot_id/items", ListUnifiedCatalogDiscoveryItems)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/snapshots/61/items?q=model%25_%27", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"model_price":null`) || !strings.Contains(response.Body.String(), `"endpoint_types":[]`) {
		t.Fatalf("code=%d body=%s", response.Code, response.Body)
	}
}
