package console

import (
	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/middleware"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/internal/service"
	pkgErrors "github.com/mirainya/Prism/pkg/errors"
)

func GetMyBillingAccount(c *gin.Context) {
	userID := middleware.GetUserID(c)
	if userID == 0 {
		resp.Unauthorized(c, pkgErrors.ErrUnauthorized)
		return
	}
	db, err := model.DB().DB()
	if err != nil {
		resp.InternalError(c, pkgErrors.ErrInternalError)
		return
	}
	store, err := repository.New(db)
	if err != nil {
		resp.InternalError(c, pkgErrors.ErrInternalError)
		return
	}
	accounts, err := store.LoadBillingAccountSummaries(c.Request.Context(), []uint64{uint64(userID)})
	if err != nil {
		resp.InternalError(c, pkgErrors.ErrInternalError)
		return
	}
	account, exists := accounts[uint64(userID)]
	if !exists {
		resp.Success(c, nil)
		return
	}
	resp.Success(c, account)
}

func ListFundsEvents(c *gin.Context) {
	var req service.ListFundsEventsRequest
	if !bindObservabilityQuery(c, &req.ObservabilityListFilter, &req) {
		return
	}
	result, err := observabilityService.ListFundsEvents(&req, observabilityScope(c))
	writeObservabilityResult(c, result, err)
}
