package open

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/gateway/adapter"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/pkg/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestImagePlanRejectionRetainsFieldAndSafeDiagnostic(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	previous := logger.L
	logger.L = zap.New(core)
	t.Cleanup(func() { logger.L = previous })
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits/async", nil)
	err := fmt.Errorf("%w: %w", repository.ErrInvalidInput, &adapter.ImageRequestError{
		Param: "quality", Message: "quality must be omitted or auto for this model",
	})
	request := adapter.OpenAIImagesRequest{Prompt: "private prompt", Model: "private model",
		ImageURLs: []string{"https://private.example/ref.png?signature=private-signature"}, N: 1, Quality: "high"}
	logOpenAIImagePlanRejection(c, &model.Token{BaseModel: model.BaseModel{ID: 44}, UserID: 1}, request, adapter.ImagesEdit, err)
	writeOpenAIImageExecutionError(c, err)
	var body struct {
		Error struct {
			Param, Message, Type, Code string
		}
	}
	if response.Code != http.StatusBadRequest || json.Unmarshal(response.Body.Bytes(), &body) != nil ||
		body.Error.Param != "quality" || body.Error.Code != "unsupported_image_parameter" {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
	entries := observed.All()
	if len(entries) != 1 {
		t.Fatalf("diagnostic entries = %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["param"] != "quality" || fields["call_created"] != false || fields["upstream_submitted"] != false || fields["token_id"] != uint64(44) {
		t.Fatalf("diagnostic fields = %v", fields)
	}
	encoded, _ := json.Marshal(fields)
	for _, secret := range []string{"private prompt", "private model", "private.example", "private-signature"} {
		if strings.Contains(string(encoded)+response.Body.String(), secret) {
			t.Fatal("request contents escaped into public error or log")
		}
	}
}
