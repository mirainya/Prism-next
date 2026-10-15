package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/pkg/upstreamerror"
)

func GetUpstreamErrorRules(c *gin.Context) {
	db, err := model.DB().DB()
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	rules, err := upstreamerror.Read(c.Request.Context(), db)
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	resp.Success(c, rules)
}

func UpdateUpstreamErrorRules(c *gin.Context) {
	var rules upstreamerror.Rules
	if !unifiedChannelBody(c, &rules) {
		return
	}
	db, err := model.DB().DB()
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	if err := upstreamerror.Save(c.Request.Context(), db, rules); err != nil {
		if errors.Is(err, upstreamerror.ErrInvalidRules) {
			resp.ErrorMsg(c, http.StatusBadRequest, 400, "匹配规则或替换消息无效")
			return
		}
		unifiedChannelError(c, err)
		return
	}
	resp.Success(c, rules)
}
