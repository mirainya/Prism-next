package admin

import (
	stdErrors "errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/middleware"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/internal/service"
	"github.com/mirainya/Prism/pkg/errors"
	"gorm.io/gorm"
)

var userService = service.NewUserService()

func ListUsers(c *gin.Context) {
	users, err := userService.ListUsers()
	if err != nil {
		resp.InternalError(c, errors.ErrInternalError)
		return
	}

	db, err := model.DB().DB()
	if err != nil {
		billingAccountError(c, err)
		return
	}
	store, err := repository.New(db)
	if err != nil {
		billingAccountError(c, err)
		return
	}
	userIDs := make([]uint64, len(users))
	for i, user := range users {
		userIDs[i] = uint64(user.ID)
	}
	accounts, err := store.LoadBillingAccountSummaries(c.Request.Context(), userIDs)
	if err != nil {
		billingAccountError(c, err)
		return
	}
	result := make([]gin.H, len(users))
	for i, u := range users {
		result[i] = gin.H{
			"id":              u.ID,
			"username":        u.Username,
			"role":            u.Role,
			"balance":         u.Balance,
			"total_used":      u.TotalUsed,
			"status":          u.Status,
			"created_at":      u.CreatedAt,
			"billing_account": nil,
		}
		if account, found := accounts[uint64(u.ID)]; found {
			result[i]["billing_account"] = account
		}
	}

	resp.Success(c, result)
}

type UpdateRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=admin user"`
}

func UpdateUserRole(c *gin.Context) {
	var req UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, errors.WithMessage(errors.ErrInvalidParams, err.Error()))
		return
	}

	id, err := resp.ParseUintParam(c, "id")
	if err != nil {
		return
	}

	if err := userService.UpdateUserRole(id, model.UserRole(req.Role)); err != nil {
		writeUserAccessError(c, err)
		return
	}

	resp.Success(c, gin.H{"updated": true})
}

type UpdateStatusRequest struct {
	Status *int8 `json:"status" binding:"required,oneof=0 1"`
}

func UpdateUserStatus(c *gin.Context) {
	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, errors.WithMessage(errors.ErrInvalidParams, err.Error()))
		return
	}

	id, err := resp.ParseUintParam(c, "id")
	if err != nil {
		return
	}

	if err := userService.UpdateUserStatus(id, *req.Status); err != nil {
		writeUserAccessError(c, err)
		return
	}

	resp.Success(c, gin.H{"updated": true})
}

func writeUserAccessError(c *gin.Context, err error) {
	switch {
	case stdErrors.Is(err, service.ErrLastAdministrator):
		resp.ErrorMsg(c, http.StatusConflict, 409, "至少保留一位启用的管理员，不能停用或降级最后一位管理员")
	case stdErrors.Is(err, gorm.ErrRecordNotFound):
		resp.ErrorMsg(c, http.StatusNotFound, 404, "用户不存在")
	case stdErrors.Is(err, service.ErrInvalidUserAccess):
		resp.BadRequest(c, errors.ErrInvalidParams)
	default:
		resp.InternalError(c, errors.ErrInternalError)
	}
}

func RechargeUser(c *gin.Context) {
	var req service.FundsAdjustmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.BadRequest(c, errors.WithMessage(errors.ErrInvalidParams, err.Error()))
		return
	}

	id, err := resp.ParseUintParam(c, "id")
	if err != nil {
		return
	}

	if err := userService.ApplyUserCredit(c.Request.Context(), middleware.GetUserID(c), id, req); err != nil {
		if stdErrors.Is(err, service.ErrInvalidBalanceAmount) || stdErrors.Is(err, service.ErrInvalidFundsOperation) {
			resp.BadRequest(c, errors.WithMessage(errors.ErrInvalidParams, err.Error()))
			return
		}
		if stdErrors.Is(err, service.ErrFundsRequestConflict) {
			resp.ErrorMsg(c, http.StatusConflict, 409, "请求编号已用于其他金额或备注，请核查原记录")
			return
		}
		if stdErrors.Is(err, repository.ErrConflict) {
			resp.ErrorMsg(c, http.StatusUnprocessableEntity, 422, "账户状态或金额精度不符合当前计费配置")
			return
		}
		resp.InternalError(c, errors.ErrInternalError)
		return
	}

	resp.Success(c, gin.H{"recharged": true})
}
