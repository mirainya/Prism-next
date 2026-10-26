package open

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/middleware"
	"github.com/mirainya/Prism/internal/gateway/adapter"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/pkg/logger"
	"go.uber.org/zap"
)

// Planning precedes durable call creation. Record rejected requests separately
// in the server log without prompts, image URLs, credentials or request bodies.
func logOpenAIImagePlanRejection(c *gin.Context, token *model.Token, request adapter.OpenAIImagesRequest, operation string, err error) {
	if logger.L == nil || token == nil ||
		!errors.Is(err, repository.ErrInvalidInput) && !errors.Is(err, adapter.ErrInvalidImageRequest) {
		return
	}
	fields := []zap.Field{
		zap.String("request_id", middleware.GetRequestID(c.Request.Context())),
		zap.String("path", c.FullPath()), zap.String("operation", operation),
		zap.Uint("user_id", token.UserID), zap.Uint("token_id", token.ID),
		zap.String("stage", "image_request_validation"),
		zap.Bool("call_created", false), zap.Bool("upstream_submitted", false),
		zap.Int("image_count", len(request.ImageURLs)), zap.Int("mask_count", len(request.MaskURLs)),
		zap.Int("n", request.N), zap.Bool("stream", request.Stream),
	}
	var fieldErr *adapter.ImageRequestError
	if errors.As(err, &fieldErr) {
		fields = append(fields, zap.String("param", fieldErr.Param), zap.String("reason", fieldErr.Message))
	}
	logger.Warn("image request rejected before call creation", fields...)
}
