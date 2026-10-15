package admin

import (
	"errors"
	"github.com/mirainya/Prism/internal/gateway/adapter"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/gateway/repository"
	gatewayruntime "github.com/mirainya/Prism/internal/gateway/runtime"
	"github.com/mirainya/Prism/internal/model"
)

func RecoverUnifiedVideoTask(c *gin.Context) {
	operatorID := currentAdminID(c)
	if operatorID == 0 {
		resp.ErrorMsg(c, http.StatusForbidden, http.StatusForbidden, "需要管理员身份")
		return
	}
	var in struct {
		ProviderTaskID string `json:"provider_task_id"`
	}
	if !unifiedChannelBody(c, &in) {
		return
	}
	in.ProviderTaskID = strings.TrimSpace(in.ProviderTaskID)
	if len(in.ProviderTaskID) > 4096 || strings.ContainsAny(in.ProviderTaskID, "\r\n\x00") {
		resp.ErrorMsg(c, http.StatusBadRequest, http.StatusBadRequest, "请填写有效的上游任务 ID")
		return
	}
	if !model.HasDB() {
		resp.ErrorMsg(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, "数据库尚未就绪")
		return
	}
	db, err := model.DB().DB()
	if err != nil {
		writeUnifiedVideoResolutionError(c, err)
		return
	}
	store, err := repository.New(db)
	if err != nil {
		writeUnifiedVideoResolutionError(c, err)
		return
	}
	var asyncID uint64
	err = db.QueryRowContext(c.Request.Context(), `SELECT x.id FROM gw_api_resources r
JOIN gw_api_calls call_record ON call_record.id=r.call_id
JOIN gw_api_call_attempts a ON a.id=COALESCE(call_record.current_attempt_id,call_record.final_attempt_id)
JOIN gw_async_executions x ON x.attempt_id=a.id
WHERE r.public_id=? AND r.resource_kind='video_task'`, c.Param("id")).Scan(&asyncID)
	if err != nil {
		writeUnifiedVideoResolutionError(c, err)
		return
	}
	kek, err := adminGatewayKey("PRISM_GATEWAY_PAYLOAD_KEK_B64")
	if err != nil {
		writeUnifiedVideoResolutionError(c, err)
		return
	}
	defer clear(kek)
	hmacKey, err := adminGatewayKey("PRISM_GATEWAY_PAYLOAD_HMAC_B64")
	if err != nil {
		writeUnifiedVideoResolutionError(c, err)
		return
	}
	defer clear(hmacKey)
	service, err := gatewayruntime.New(store)
	if err == nil {
		var dispatcher *gatewayruntime.AsyncDispatcher
		dispatcher, err = gatewayruntime.NewAsyncDispatcher(service, nil, gatewayruntime.AsyncKeys{PayloadKEK: kek, PayloadHMAC: hmacKey}, adapter.AsyncCodecs())
		if err == nil {
			err = dispatcher.ContinueAsyncTask(c.Request.Context(), asyncID, uint64(operatorID), in.ProviderTaskID)
		}
	}
	if errors.Is(err, gatewayruntime.ErrProviderTaskIdentityRequired) {
		resp.ErrorMsg(c, http.StatusConflict, http.StatusConflict, "未找到上游任务 ID，请从上游后台查找后补填")
		return
	}

	if err != nil {
		writeUnifiedVideoResolutionError(c, err)
		return
	}
	resp.Success(c, gin.H{"id": c.Param("id"), "provider_task_id": in.ProviderTaskID, "query_scheduled": true})
}
