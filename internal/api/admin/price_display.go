package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/gateway/billing"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/internal/service"
)

func GetPriceDisplaySettings(c *gin.Context) {
	db, err := model.DB().DB()
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	store, err := repository.New(db)
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	settings, err := store.ReadPriceDisplaySettings(c.Request.Context())
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	resp.Success(c, settings)
}

func UpdatePriceDisplaySettings(c *gin.Context) {
	in := repository.PriceDisplaySettings{ModelMultipliers: map[string]string{}, SKUMultipliers: map[string]string{}}
	if !unifiedGatewayBody(c, &in, 128*1024) {
		return
	}
	if err := in.Validate(); err != nil {
		unifiedChannelError(c, err)
		return
	}
	if in.ConfigVersion == 0 {
		unifiedChannelError(c, repository.ErrInvalidInput)
		return
	}
	if currentAdminID(c) == 0 {
		resp.ErrorMsg(c, http.StatusForbidden, 403, "需要管理员身份")
		return
	}
	if !model.HasDB() {
		resp.ErrorMsg(c, http.StatusServiceUnavailable, 503, "数据库尚未就绪")
		return
	}
	if in.ModelMultipliers == nil {
		in.ModelMultipliers = map[string]string{}
	}
	if in.SKUMultipliers == nil {
		in.SKUMultipliers = map[string]string{}
	}
	db, err := model.DB().DB()
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	if err := service.ValidatePriceDisplayProjection(c.Request.Context(), db, in); err != nil {
		if errors.Is(err, billing.ErrInvalidExpression) || errors.Is(err, billing.ErrExpressionLimit) {
			resp.ErrorMsg(c, 400, 400, "倍率无法用于当前模型的公开价格，请调整配置")
		} else {
			unifiedChannelError(c, err)
		}
		return
	}
	if !unifiedGatewayWrite(c, func(store *repository.Store, tx *sql.Tx, actor uint64) error {
		return store.SavePriceDisplaySettings(c.Request.Context(), tx, in, actor)
	}) {
		return
	}
	in.ConfigVersion++
	resp.Success(c, in)
}

func PreviewPriceDisplaySettings(c *gin.Context) {
	var in struct {
		Settings repository.PriceDisplaySettings `json:"settings"`
		Model    string                          `json:"model"`
	}
	if !unifiedGatewayBody(c, &in, 128*1024) {
		return
	}
	if strings.TrimSpace(in.Model) == "" || in.Settings.Validate() != nil {
		unifiedChannelError(c, repository.ErrInvalidInput)
		return
	}
	db, err := model.DB().DB()
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	store, err := repository.New(db)
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	if err := store.ValidatePriceDisplayTargets(c.Request.Context(), in.Settings); err != nil {
		unifiedChannelError(c, err)
		return
	}
	result, err := service.PreviewPriceDisplay(c.Request.Context(), db, in.Settings, in.Model)
	if err != nil {
		if errors.Is(err, billing.ErrInvalidExpression) || errors.Is(err, billing.ErrExpressionLimit) {
			resp.ErrorMsg(c, 400, 400, "倍率无法用于当前模型的公开价格，请调整配置")
		} else {
			unifiedChannelError(c, err)
		}
		return
	}
	if result == nil {
		unifiedChannelError(c, repository.ErrNotFound)
		return
	}
	resp.Success(c, result.Pricing)
}
