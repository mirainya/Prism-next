package admin

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

func modelNameSuggestionRouter(actor uint) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		if actor != 0 {
			c.Set("user_id", actor)
		}
	})
	router.GET("/model-name-suggestion", SuggestUnifiedCatalogModelName)
	return router
}

func TestModelNameSuggestionRequiresValidInputAndAdmin(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
	}{{"", 400}, {"bad name", 400}, {strings.Repeat("a", 129), 400}, {"model-a", 403}} {
		response := httptest.NewRecorder()
		modelNameSuggestionRouter(0).ServeHTTP(response, httptest.NewRequest("GET", "/model-name-suggestion?base_name="+url.QueryEscape(test.name), nil))
		if response.Code != test.status {
			t.Fatalf("code=%d body=%s", response.Code, response.Body)
		}
	}
}

func TestModelNameSuggestionOnlyReadsGlobalReservations(t *testing.T) {
	mock := unifiedChannelMock(t)
	for index, candidate := range []string{"a/9图%", "a/9图%-2"} {
		mock.ExpectQuery("SELECT EXISTS.*gw_model_names.*gw_models").WithArgs(candidate, candidate).
			WillReturnRows(sqlmock.NewRows([]string{"occupied"}).AddRow(index == 0))
	}
	response := httptest.NewRecorder()
	modelNameSuggestionRouter(1).ServeHTTP(response, httptest.NewRequest("GET", "/model-name-suggestion?base_name="+url.QueryEscape(" a/9图% "), nil))
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"api_name":"a/9图%-2"`) {
		t.Fatalf("code=%d body=%s", response.Code, response.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestModelNameConflictHasSpecificResponse(t *testing.T) {
	router := gin.New()
	router.GET("/", func(c *gin.Context) {
		unifiedChannelError(c, fmt.Errorf("onboard: %w", repository.ErrPublicModelNameConflict))
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/", nil))
	if response.Code != 409 || !strings.Contains(response.Body.String(), "公开调用名或模型标识已占用") {
		t.Fatalf("code=%d body=%s", response.Code, response.Body)
	}
}
