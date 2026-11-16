package console

import (
	stdErrors "errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/middleware"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/internal/service"
	pkgErrors "github.com/mirainya/Prism/pkg/errors"
)

var apiCallService = service.NewAPICallService()

func ListAPICalls(c *gin.Context) {
	actorUserID := middleware.GetUserID(c)
	if actorUserID == 0 {
		resp.Unauthorized(c, pkgErrors.ErrUnauthorized)
		return
	}

	var req service.ListCallsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		resp.BadRequest(c, pkgErrors.WithMessage(pkgErrors.ErrInvalidParams, "invalid query parameters"))
		return
	}

	isAdmin := middleware.GetUserRole(c) == string(model.UserRoleAdmin)
	if !isAdmin && req.UserID != 0 && req.UserID != actorUserID {
		resp.Forbidden(c, pkgErrors.WithMessage(pkgErrors.ErrNoPermission, "no permission to access this user's calls"))
		return
	}

	req.ActorUserID = actorUserID
	req.IsAdmin = isAdmin
	if !isAdmin {
		req.UserID = 0
	}

	result, err := apiCallService.ListCalls(&req)
	if err != nil {
		writeAPICallError(c, err)
		return
	}
	resp.Success(c, result)
}

func GetAPICall(c *gin.Context) {
	actorUserID := middleware.GetUserID(c)
	if actorUserID == 0 {
		resp.Unauthorized(c, pkgErrors.ErrUnauthorized)
		return
	}

	callID := strings.TrimSpace(c.Param("id"))
	if callID == "" {
		resp.BadRequest(c, pkgErrors.WithMessage(pkgErrors.ErrInvalidParams, "invalid call id"))
		return
	}

	isAdmin := middleware.GetUserRole(c) == string(model.UserRoleAdmin)
	detail, err := apiCallService.GetCallDetail(callID, actorUserID, isAdmin)
	if err != nil {
		writeAPICallError(c, err)
		return
	}
	resp.Success(c, detail)
}

func writeAPICallError(c *gin.Context, err error) {
	switch {
	case stdErrors.Is(err, service.ErrAPICallInvalidInput):
		resp.BadRequest(c, pkgErrors.WithMessage(pkgErrors.ErrInvalidParams, "invalid call query"))
	case stdErrors.Is(err, service.ErrAPICallAccessDenied):
		resp.Forbidden(c, pkgErrors.WithMessage(pkgErrors.ErrNoPermission, "no permission to access this call"))
	case stdErrors.Is(err, service.ErrAPICallNotFound):
		resp.NotFound(c, pkgErrors.WithMessage(pkgErrors.ErrTaskNotFound, "api call not found"))
	case stdErrors.Is(err, service.ErrAPICallDeliveryUnavailable):
		resp.ErrorMsg(c, http.StatusConflict, 409, "结果文件暂不可用，请检查转存状态或稍后重试")
	default:
		resp.InternalError(c, pkgErrors.ErrInternalError)
	}
}

// ListCallTokenOptions returns identifiers and names only, never key material.
func ListCallTokenOptions(c *gin.Context) {
	actor := middleware.GetUserID(c)
	if actor == 0 {
		resp.Unauthorized(c, pkgErrors.ErrUnauthorized)
		return
	}
	if middleware.GetUserRole(c) != string(model.UserRoleAdmin) {
		resp.Forbidden(c, pkgErrors.ErrNoPermission)
		return
	}
	var req struct {
		UserID uint `form:"user_id"`
	}
	if err := c.ShouldBindQuery(&req); err != nil {
		resp.BadRequest(c, pkgErrors.ErrInvalidParams)
		return
	}
	type option struct {
		ID     uint   `json:"id"`
		Name   string `json:"name"`
		UserID uint   `json:"user_id"`
	}
	items := make([]option, 0)
	query := model.DB().Table("gw_api_calls AS c").Joins("LEFT JOIN tokens t ON t.id=c.token_id AND t.user_id=c.user_id")
	if req.UserID > 0 {
		query = query.Where("c.user_id=?", req.UserID)
	}
	if err := query.Select("DISTINCT c.token_id AS id, COALESCE(t.name, '') AS name, c.user_id").Order("c.token_id DESC").Scan(&items).Error; err != nil {
		writeAPICallError(c, err)
		return
	}
	resp.Success(c, items)
}
