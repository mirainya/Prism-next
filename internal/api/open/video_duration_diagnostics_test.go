package open

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/video/generic"
)

func TestVideoDurationFailurePreservesAccessDetail(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	err := fmt.Errorf("%w: %w", repository.ErrInvalidInput, &generic.ReferenceDurationError{
		Index: 2, Kind: "audio", Actual: 0, Limit: 1.8, Minimum: true,
	})
	setVideoAccessErrorDetail(c, err)
	detail := c.GetString("access_error_detail")
	for _, expected := range []string{"content[2].duration_seconds", "missing or zero", "1.8 seconds"} {
		if !strings.Contains(detail, expected) {
			t.Fatalf("access detail %q does not contain %q", detail, expected)
		}
	}
}
