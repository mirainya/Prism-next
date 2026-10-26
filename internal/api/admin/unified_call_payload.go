package admin

import (
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/gateway/payloadview"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/internal/service"
	pkgErrors "github.com/mirainya/Prism/pkg/errors"
	"time"
)

// UnifiedCallPayload opens one retained body on an explicit administrator request.
func UnifiedCallPayload(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	callID, err := resp.ParseUintParam(c, "id")
	if err != nil {
		return
	}
	payloadID, err := resp.ParseUintParam(c, "payload_id")
	if err != nil {
		return
	}
	db, err := model.DB().DB()
	if err != nil {
		resp.InternalError(c, pkgErrors.ErrInternalError)
		return
	}
	var kind string
	var expires, purged sql.NullTime
	err = db.QueryRowContext(c.Request.Context(), `SELECT p.kind,p.retention_until,p.purged_at FROM gw_api_call_payloads p JOIN gw_api_calls c ON c.id=p.call_id WHERE p.id=? AND c.id=?`, payloadID, callID).Scan(&kind, &expires, &purged)
	if errors.Is(err, sql.ErrNoRows) {
		resp.ErrorMsg(c, 404, 404, "未找到此调用的内容")
		return
	}
	if err != nil {
		resp.InternalError(c, pkgErrors.ErrInternalError)
		return
	}
	if purged.Valid || (expires.Valid && !expires.Time.After(time.Now())) {
		resp.Success(c, gin.H{"status": "expired"})
		return
	}
	store, err := repository.New(db)
	if err != nil {
		resp.InternalError(c, pkgErrors.ErrInternalError)
		return
	}
	plain, err := payloadview.ReadCallPayload(c.Request.Context(), store, uint64(callID), uint64(payloadID), kind)
	if errors.Is(err, repository.ErrNotFound) {
		resp.Success(c, gin.H{"status": "expired"})
		return
	}
	if err != nil {
		resp.ErrorMsg(c, 500, 500, "内容解密失败，请检查服务端加密配置")
		return
	}
	defer clear(plain)
	sanitized := service.SanitizeAPICallPayload(plain)
	defer clear(sanitized)
	resp.Success(c, gin.H{"status": "available", "data": string(sanitized)})
}
