package console

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/service"
	pkgErrors "github.com/mirainya/Prism/pkg/errors"
	"gorm.io/gorm"
)

func ListManagedTokens(c *gin.Context) {
	var req service.TokenManagementQuery
	if c.ShouldBindQuery(&req) != nil {
		resp.BadRequest(c, pkgErrors.ErrInvalidParams)
		return
	}
	result, err := tokenService.ListManagedTokens(c.Request.Context(), observabilityScope(c), req)
	writeTokenManagementResult(c, result, err)
}

func GetTokenRateLimitSettings(c *gin.Context) {
	resp.Success(c, service.TokenRateLimitDefaults())
}

func UpdateTokenPolicy(c *gin.Context) {
	id, err := resp.ParseUintParam(c, "id")
	if err != nil {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var body struct {
		ExpectedVersion string          `json:"expected_version"`
		RateLimit       *int            `json:"rate_limit"`
		ExpiresAt       json.RawMessage `json:"expires_at"`
		Note            string          `json:"note"`
	}
	if c.ShouldBindJSON(&body) != nil || len(body.ExpiresAt) == 0 {
		resp.BadRequest(c, pkgErrors.ErrInvalidParams)
		return
	}
	req := service.TokenPolicyRequest{ExpectedVersion: body.ExpectedVersion, RateLimit: body.RateLimit, Note: body.Note}
	if json.Unmarshal(body.ExpiresAt, &req.ExpiresAt) != nil {
		resp.BadRequest(c, pkgErrors.ErrInvalidParams)
		return
	}
	err = tokenService.UpdateTokenPolicy(c.Request.Context(), observabilityScope(c), id, req)
	writeTokenManagementResult(c, gin.H{"updated": true}, err)
}

func ChangeTokenAccess(c *gin.Context) {
	id, err := resp.ParseUintParam(c, "id")
	if err != nil {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var req service.TokenAccessRequest
	if c.ShouldBindJSON(&req) != nil {
		resp.BadRequest(c, pkgErrors.ErrInvalidParams)
		return
	}
	err = tokenService.ChangeTokenAccess(c.Request.Context(), observabilityScope(c), id, req)
	writeTokenManagementResult(c, gin.H{"updated": true}, err)
}

func TokenAccessHistory(c *gin.Context) {
	id, err := resp.ParseUintParam(c, "id")
	if err != nil {
		return
	}
	var req service.TokenManagementQuery
	if c.ShouldBindQuery(&req) != nil {
		resp.BadRequest(c, pkgErrors.ErrInvalidParams)
		return
	}
	result, err := tokenService.TokenAccessHistory(c.Request.Context(), observabilityScope(c), id, req.Page, req.PageSize)
	writeTokenManagementResult(c, result, err)
}

func writeTokenManagementResult(c *gin.Context, result any, err error) {
	switch {
	case err == nil:
		resp.Success(c, result)
	case errors.Is(err, service.ErrInvalidTokenManagement):
		resp.BadRequest(c, pkgErrors.ErrInvalidParams)
	case errors.Is(err, service.ErrObservabilityAccessDenied):
		resp.Forbidden(c, pkgErrors.ErrNoPermission)
	case errors.Is(err, gorm.ErrRecordNotFound):
		resp.NotFound(c, pkgErrors.ErrTaskNotFound)
	case errors.Is(err, repository.ErrConflict):
		resp.ErrorMsg(c, http.StatusConflict, 409, "Key 状态已变化，请刷新后重新操作；已撤销的 Key 不能恢复")
	case errors.Is(err, service.ErrTokenResumeBlocked):
		resp.ErrorMsg(c, http.StatusUnprocessableEntity, 422, "无法恢复：请检查用户状态、有效期、账户及预算配置")
	default:
		resp.InternalError(c, pkgErrors.ErrInternalError)
	}
}
