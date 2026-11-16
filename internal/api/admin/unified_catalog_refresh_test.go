package admin

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
)

func TestCatalogDiscoveryScopeUsesActualKeyNotSharedPool(t *testing.T) {
	for _, test := range []struct {
		query, where string
		args         []any
		valid        bool
	}{
		{"", "", []any{uint64(7)}, true},
		{"?source_id=12&credential_id=40", " AND rs.catalog_source_id=? AND r.credential_id=?", []any{uint64(7), uint64(12), uint64(40)}, true},
		{"?credential_id=0", "", nil, false},
		{"?source_id=-1", "", nil, false},
		{"?credential_id=18446744073709551616", "", nil, false},
		{"?credential_id=40%20OR%201=1", "", nil, false},
	} {
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest("GET", "/"+test.query, nil)
		where, args, ok := catalogDiscoveryScope(ctx, 7)
		if ok != test.valid {
			t.Fatalf("query=%s valid=%v", test.query, ok)
		}
		if ok && (where != ` WHERE rs.release_id=? AND r.action='catalog_discovery'`+test.where || !reflect.DeepEqual(args, test.args)) {
			t.Fatalf("where=%s args=%v", where, args)
		}
		if !ok && response.Code != http.StatusBadRequest {
			t.Fatalf("code=%d", response.Code)
		}
	}
}

func TestCatalogHistoryOnlyReadsPrismForSelectedKey(t *testing.T) {
	mock := unifiedChannelMock(t)
	mock.ExpectQuery("SELECT COUNT.*rs.catalog_source_id=.*r.credential_id=").WithArgs(uint64(7), uint64(12), uint64(40)).WillReturnRows(sqlmock.NewRows([]string{"total"}).AddRow(0))
	mock.ExpectQuery("SELECT r.id,rs.id.*rs.catalog_source_id=.*r.credential_id=").WithArgs(uint64(7), uint64(12), uint64(40), 20, 0).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	router := gin.New()
	router.GET("/catalog/:id/discoveries", ListUnifiedCatalogDiscoveries)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/catalog/7/discoveries?source_id=12&credential_id=40", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"items":[]`) {
		t.Fatalf("code=%d body=%s", response.Code, response.Body)
	}
}

func TestActiveCatalogRefreshRequiresActorAndRejectsUnknownBody(t *testing.T) {
	router := gin.New()
	router.POST("/catalog/:id/catalog-sources/:source_id/refresh", RefreshUnifiedActiveCatalog)
	for _, test := range []struct {
		body   string
		status int
	}{
		{"{}", http.StatusForbidden},
		{`{"credential_id":99}`, http.StatusBadRequest},
		{"null", http.StatusBadRequest},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("POST", "/catalog/7/catalog-sources/12/refresh", strings.NewReader(test.body)))
		if response.Code != test.status {
			t.Fatalf("body=%s code=%d", test.body, response.Code)
		}
	}
}

func TestRoutesIncludeReadOnlyActiveCatalogRefresh(t *testing.T) {
	router := gin.New()
	RegisterRoutes(router.Group("/api/admin"))
	for _, route := range router.Routes() {
		if route.Method == "POST" && route.Path == "/api/admin/unified-gateway/catalog/:id/catalog-sources/:source_id/refresh" {
			return
		}
	}
	t.Fatal("active catalog refresh route not registered")
}
