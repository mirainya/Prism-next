package admin

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
)

func UnifiedCredentialRemovalImpact(c *gin.Context) {
	id, err := resp.ParseUintParam(c, "id")
	if err != nil {
		return
	}
	if currentAdminID(c) == 0 {
		resp.ErrorMsg(c, 403, 403, "需要管理员身份")
		return
	}
	if !model.HasDB() {
		resp.ErrorMsg(c, 503, 503, "数据库尚未就绪")
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
	impact, err := store.CredentialRemovalImpact(c.Request.Context(), db, uint64(id))
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	resp.Success(c, impact)
}

func RemoveUnifiedCredential(c *gin.Context) {
	id, err := resp.ParseUintParam(c, "id")
	if err != nil {
		return
	}
	var in struct {
		ExpectedVersion uint64 `json:"expected_version"`
	}
	if !unifiedChannelBody(c, &in) {
		return
	}
	if in.ExpectedVersion == 0 {
		unifiedChannelError(c, repository.ErrInvalidInput)
		return
	}
	var impact repository.CredentialRemovalImpact
	if !unifiedGatewayWrite(c, func(store *repository.Store, tx *sql.Tx, actor uint64) error {
		var err error
		impact, err = store.RemoveManagedCredential(c.Request.Context(), tx, uint64(id), in.ExpectedVersion, actor)
		return err
	}) {
		return
	}
	resp.Success(c, impact)
}
