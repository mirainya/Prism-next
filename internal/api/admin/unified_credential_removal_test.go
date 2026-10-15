package admin

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCredentialRemovalRequiresAdminAndStrictVersion(t *testing.T) {
	for _, test := range []struct {
		method, path, body string
		want               int
	}{
		{"GET", "/credentials/5/removal-impact", "", 403},
		{"POST", "/credentials/5/remove", `{"expected_version":1}`, 403},
		{"POST", "/credentials/5/remove", `{}`, 400},
		{"POST", "/credentials/5/remove", `{"expected_version":0}`, 400},
		{"POST", "/credentials/5/remove", `{"expected_version":1,"secret":"private"}`, 400},
		{"POST", "/credentials/5/remove", `null`, 400},
		{"POST", "/credentials/bad/remove", `{"expected_version":1}`, 400},
	} {
		router := gin.New()
		router.GET("/credentials/:id/removal-impact", UnifiedCredentialRemovalImpact)
		router.POST("/credentials/:id/remove", RemoveUnifiedCredential)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(test.method, test.path, strings.NewReader(test.body)))
		if response.Code != test.want || strings.Contains(response.Body.String(), "private") {
			t.Fatalf("%s %s code=%d body=%s", test.method, test.path, response.Code, response.Body)
		}
	}
}
