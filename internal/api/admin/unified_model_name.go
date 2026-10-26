package admin

import (
	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
)

func SuggestUnifiedCatalogModelName(c *gin.Context) {
	in := repository.CatalogModelNameSuggestionInput{BaseName: c.Query("base_name")}
	in.Normalize()
	if err := in.Validate(); err != nil {
		unifiedChannelError(c, err)
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
	name, err := store.SuggestCatalogModelName(c.Request.Context(), in)
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	resp.Success(c, gin.H{"api_name": name})
}
