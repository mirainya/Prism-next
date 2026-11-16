package console

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/middleware"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/model"
	pkgErrors "github.com/mirainya/Prism/pkg/errors"
)

func GetAPICallDeliveryURL(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actorUserID := middleware.GetUserID(c)
	if actorUserID == 0 {
		resp.Unauthorized(c, pkgErrors.ErrUnauthorized)
		return
	}
	deliveryID, err := strconv.ParseUint(c.Param("delivery_id"), 10, 64)
	if err != nil || deliveryID == 0 || strings.TrimSpace(c.Param("id")) == "" {
		resp.ErrorMsg(c, http.StatusBadRequest, 400, "无效的结果文件编号")
		return
	}
	isAdmin := middleware.GetUserRole(c) == string(model.UserRoleAdmin)
	result, err := apiCallService.GetCallDeliveryURL(c.Request.Context(), c.Param("id"), deliveryID, actorUserID, isAdmin)
	if err != nil {
		writeAPICallError(c, err)
		return
	}
	resp.Success(c, result)
}

func RetryAPICallDelivery(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	actor := middleware.GetUserID(c)
	if actor == 0 {
		resp.Unauthorized(c, pkgErrors.ErrUnauthorized)
		return
	}
	id, err := strconv.ParseUint(c.Param("delivery_id"), 10, 64)
	if err != nil || id == 0 || strings.TrimSpace(c.Param("id")) == "" {
		resp.ErrorMsg(c, http.StatusBadRequest, 400, "无效的结果文件编号")
		return
	}
	admin := middleware.GetUserRole(c) == string(model.UserRoleAdmin)
	if err := apiCallService.RetryCallDelivery(c.Request.Context(), c.Param("id"), id, actor, admin); err != nil {
		writeAPICallError(c, err)
		return
	}
	resp.Success(c, gin.H{"scheduled": true})
}
