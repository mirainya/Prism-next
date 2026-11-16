package open

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/gateway/routing"
	gatewayruntime "github.com/mirainya/Prism/internal/gateway/runtime"
)

// A missing call ID alone is not evidence of rejection. Advertise only
// failures known to occur before call creation, never an ambiguous commit.
func markRejectedSubmission(c *gin.Context, err error) {
	if c.Writer.Written() || c.Writer.Header().Get(prismCallIDHeader) != "" || c.Writer.Header().Get("Location") != "" {
		return
	}
	if errors.Is(err, gatewayruntime.ErrSubmissionRejected) ||
		errors.Is(err, gatewayruntime.ErrNotReady) || errors.Is(err, routing.ErrNoRoute) ||
		errors.Is(err, routing.ErrNoCompatibleTransport) || errors.Is(err, routing.ErrCapabilityUnavailable) {
		c.Header("X-Prism-Submission-Status", "rejected")
	}
}
