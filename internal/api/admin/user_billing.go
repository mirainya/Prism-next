package admin

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/middleware"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
)

func billingAdminStore(c *gin.Context) (*repository.Store, bool) {
	if middleware.GetUserID(c) == 0 || middleware.GetUserRole(c) != string(model.UserRoleAdmin) {
		resp.ErrorMsg(c, http.StatusForbidden, 403, "仅管理员可管理计费账户")
		return nil, false
	}
	db, err := model.DB().DB()
	if err != nil {
		billingAccountError(c, err)
		return nil, false
	}
	store, err := repository.New(db)
	if err != nil {
		billingAccountError(c, err)
		return nil, false
	}
	return store, true
}

func billingAccountError(c *gin.Context, err error) {
	var blocked *repository.BillingUnfreezeBlockedError
	switch {
	case errors.As(err, &blocked):
		resp.ErrorMsg(c, http.StatusConflict, 409, blocked.Reason)
	case errors.Is(err, repository.ErrInvalidInput):
		resp.ErrorMsg(c, http.StatusBadRequest, 400, "请提供当前冻结记录和 1–500 字的处理备注")
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, sql.ErrNoRows):
		resp.ErrorMsg(c, http.StatusNotFound, 404, "用户或计费账户不存在")
	case errors.Is(err, repository.ErrConflict):
		resp.ErrorMsg(c, http.StatusConflict, 409, "账户冻结状态已变化，请重新查看后操作")
	default:
		resp.ErrorMsg(c, http.StatusInternalServerError, 500, "读取或恢复计费账户失败")
	}
}

func GetUserBillingAccount(c *gin.Context) {
	store, ok := billingAdminStore(c)
	if !ok {
		return
	}
	id, err := resp.ParseUintParam(c, "id")
	if err != nil {
		return
	}
	detail, err := store.GetBillingAccountDetail(c.Request.Context(), uint64(id))
	if err != nil {
		billingAccountError(c, err)
		return
	}
	resp.Success(c, detail)
}

func UnfreezeUserBillingAccount(c *gin.Context) {
	store, ok := billingAdminStore(c)
	if !ok {
		return
	}
	id, err := resp.ParseUintParam(c, "id")
	if err != nil {
		return
	}
	var input repository.BillingUnfreezeInput
	if !unifiedGatewayBody(c, &input, 4096) {
		return
	}
	if err := input.Validate(); err != nil {
		billingAccountError(c, err)
		return
	}
	ctx := c.Request.Context()
	if err := store.WithTx(ctx, func(tx *sql.Tx) error {
		return store.UnfreezeBillingAccount(ctx, tx, uint64(id), input, uint64(middleware.GetUserID(c)))
	}); err != nil {
		billingAccountError(c, err)
		return
	}
	resp.Success(c, gin.H{"updated": true, "status": "open"})
}
