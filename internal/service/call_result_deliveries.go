package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/mirainya/Prism/internal/gateway/delivery"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/pkg/filestorage"
	"gorm.io/gorm"
)

var ErrAPICallDeliveryUnavailable = errors.New("call result file unavailable")

type APICallResultDelivery struct {
	ID                uint64     `json:"id"`
	AttemptID         uint64     `json:"attempt_id,omitempty"`
	ResultOrdinal     uint32     `json:"result_ordinal"`
	DeliveryMode      string     `json:"delivery_mode"`
	State             string     `json:"state"`
	ContentType       string     `json:"content_type"`
	ContentLength     uint64     `json:"content_length"`
	Filename          string     `json:"filename,omitempty"`
	StoragePath       string     `json:"storage_path,omitempty"`
	ReasonCode        string     `json:"reason_code,omitempty"`
	ReasonMessage     string     `json:"reason_message,omitempty"`
	FailureStage      string     `json:"failure_stage,omitempty"`
	FailureHTTPStatus int        `json:"failure_http_status,omitempty"`
	FailureRetryable  bool       `json:"failure_retryable"`
	RetryStatus       string     `json:"retry_status,omitempty"`
	RetryAttempts     uint32     `json:"retry_attempts"`
	RetryAt           *time.Time `json:"retry_at,omitempty"`
	CanRetry          bool       `json:"can_retry"`
	Available         bool       `json:"available"`
	RetentionUntil    *time.Time `json:"retention_until,omitempty"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type callResultDeliveryRow struct {
	APICallResultDelivery `gorm:"embedded"`
	Locator               string
	AssetState            string
	SourceAvailable       bool
	RetryErrorCode        string
}

func callResultDeliveriesQuery(db *gorm.DB) *gorm.DB {
	return db.Table("gw_result_deliveries AS d").
		Select(`d.id,d.attempt_id,d.result_ordinal,d.delivery_mode,d.state,
			COALESCE(ma.content_type,d.content_type,'') AS content_type,
			COALESCE(ma.content_length,d.content_length,0) AS content_length,
			COALESCE(d.reason_code,'') AS reason_code,d.updated_at,
			COALESCE(NULLIF(ma.storage_locator,''),ma.object_key,'') AS locator,
			COALESCE(ma.state,'') AS asset_state,ma.retention_until,d.retry_at,
			CASE WHEN d.source_kind='inline_response' THEN EXISTS (
			  SELECT 1 FROM gw_channel_request_logs l WHERE l.attempt_id=d.attempt_id
			  AND l.action IN ('submit','query') AND l.status='response_recorded' AND l.http_status BETWEEN 200 AND 299
			  AND l.response_bytes_complete=1 AND l.response_payload_blob_id IS NOT NULL)
			ELSE EXISTS (SELECT 1 FROM gw_result_delivery_sources ds WHERE ds.id=d.current_source_id AND ds.result_delivery_id=d.id AND ds.state='active') END AS source_available,
			COALESCE((SELECT o.status FROM gw_async_outbox o WHERE o.result_delivery_id=d.id AND o.state_version=d.state_version AND o.action='reconcile_delivery' ORDER BY o.id DESC LIMIT 1),'') AS retry_status,
			COALESCE((SELECT o.attempt_count FROM gw_async_outbox o WHERE o.result_delivery_id=d.id AND o.state_version=d.state_version AND o.action='reconcile_delivery' ORDER BY o.id DESC LIMIT 1),0) AS retry_attempts,
			COALESCE((SELECT o.last_error_code FROM gw_async_outbox o WHERE o.result_delivery_id=d.id AND o.state_version=d.state_version AND o.action='reconcile_delivery' ORDER BY o.id DESC LIMIT 1),'') AS retry_error_code`).
		Joins("JOIN gw_api_calls AS c ON c.id=d.call_id AND c.user_id=d.user_id AND c.token_id=d.token_id").
		Joins("JOIN gw_api_call_attempts AS a ON a.id=d.attempt_id AND a.call_id=d.call_id").
		Joins("LEFT JOIN gw_media_asset_refs AS mr ON mr.id=d.media_asset_ref_id AND mr.user_id=d.user_id AND mr.token_id=d.token_id AND mr.result_delivery_id=d.id").
		Joins("LEFT JOIN gw_media_assets AS ma ON ma.id=mr.media_asset_id AND ma.user_id=d.user_id AND ma.token_id=d.token_id AND ma.purpose='result'")
}

func (row callResultDeliveryRow) detail(isAdmin bool, now time.Time) APICallResultDelivery {
	result := row.APICallResultDelivery
	result.Available = result.DeliveryMode == "managed_copy" && result.State == "ready" &&
		row.AssetState == "active" && row.Locator != "" &&
		(result.RetentionUntil == nil || result.RetentionUntil.After(now))
	if parsed, err := url.Parse(row.Locator); err == nil && parsed.Path != "" {
		result.Filename = path.Base(parsed.Path)
		if isAdmin {
			result.StoragePath = parsed.Path
		}
	}
	if result.DeliveryMode == "managed_copy" && result.State != "ready" {
		result.ReasonMessage = callDeliveryFailureMessage(result.ReasonCode)
		result.FailureRetryable = delivery.RetryableManagedCopyFailure(result.ReasonCode)
		if failure, ok := delivery.ParseTransferFailure(result.ReasonCode); ok {
			result.FailureStage, result.FailureHTTPStatus = failure.Stage, failure.HTTPStatus
		}
		result.CanRetry = result.State == "delivery_failed" && row.SourceAvailable && delivery.ManagedCopyFailure(result.ReasonCode) && result.RetryStatus != "pending" && result.RetryStatus != "dispatching"
		if row.RetryErrorCode == "managed_copy_source_unavailable" || row.RetryErrorCode == "managed_copy_evidence_invalid" {
			result.CanRetry = false
			result.ReasonMessage = "原结果已清理或无法验证，不能重新转存"
		}
		if result.State == "pending" {
			result.ReasonMessage = ""
		}
	}
	if result.State == "ready" && result.DeliveryMode == "managed_copy" && !result.Available {
		result.ReasonMessage = "转存文件不可用或已清理"
	}
	if !isAdmin {
		result.AttemptID = 0
		result.ReasonCode = ""
	}
	return result
}

func callDeliveryFailureMessage(reason string) string {
	if failure, ok := delivery.ParseTransferFailure(reason); ok {
		stage := map[string]string{"upload": "上传", "import": "导入", "verify": "校验"}[failure.Stage]
		if failure.HTTPStatus != 0 {
			switch failure.HTTPStatus {
			case 401, 403:
				return stage + "失败：文件存储认证或权限不足"
			case 413:
				return stage + "失败：文件存储拒绝了过大的文件"
			case 507:
				return stage + "失败：文件存储容量不足"
			default:
				return stage + "失败：文件存储返回 HTTP " + fmt.Sprint(failure.HTTPStatus)
			}
		}
		messages := map[string]string{
			"timeout": "文件存储请求超时", "network": "文件存储连接中断", "service": "文件存储服务暂时异常",
			"permission": "文件存储认证或权限不足", "capacity": "文件存储额度或容量不足",
			"response": "文件存储返回了无法解析的响应", "locator": "文件存储未返回有效地址",
			"length": "读取到的文件长度与原结果不一致", "digest": "文件内容与原结果不一致",
			"hash": "文件存储未返回有效的内容校验值", "rejected": "文件存储拒绝了请求", "unknown": "文件存储操作失败，未取得具体原因",
		}
		return stage + "失败：" + messages[failure.Code]
	}
	switch reason {
	case delivery.ManagedCopyUploadFailed:
		return "上传到文件存储失败"
	case delivery.ManagedCopyVerificationFailed:
		return "转存后的文件读取或校验失败（旧记录未保留具体原因）"
	case delivery.ManagedCopySizeExceeded:
		return "生成文件超过转存大小限制"
	case delivery.ManagedCopyEmpty:
		return "生成文件内容为空"
	case delivery.ManagedCopyContentTypeInvalid:
		return "生成文件格式不符合要求"
	default:
		return "结果文件暂不可用"
	}
}

func (s *APICallService) RetryCallDelivery(ctx context.Context, callID string, deliveryID uint64, actorUserID uint, isAdmin bool) error {
	if strings.TrimSpace(callID) == "" || deliveryID == 0 {
		return ErrAPICallInvalidInput
	}
	if actorUserID == 0 {
		return ErrAPICallAccessDenied
	}
	sqlDB, err := model.DB().DB()
	if err != nil {
		return err
	}
	store, err := repository.New(sqlDB)
	if err != nil {
		return err
	}
	_, err = store.RequestManagedCopyRetry(ctx, callID, deliveryID, uint64(actorUserID), isAdmin)
	if errors.Is(err, repository.ErrNotFound) {
		return ErrAPICallNotFound
	}
	if errors.Is(err, repository.ErrConflict) {
		return ErrAPICallDeliveryUnavailable
	}
	return err
}

func loadUnifiedCallDeliveries(db *gorm.DB, callID uint64, isAdmin bool) ([]APICallResultDelivery, error) {
	var rows []callResultDeliveryRow
	if err := callResultDeliveriesQuery(db).Where("d.call_id=?", callID).
		Order("d.attempt_id ASC,d.result_ordinal ASC,d.id ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	results := make([]APICallResultDelivery, 0, len(rows))
	now := time.Now()
	for _, row := range rows {
		results = append(results, row.detail(isAdmin, now))
	}
	return results, nil
}

type APICallDeliveryURL struct {
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expires_at"`
}

// GetCallDeliveryURL signs only a stored result belonging to the authorized call.
// It deliberately uses the call's storage snapshot rather than the current token binding.
func (s *APICallService) GetCallDeliveryURL(ctx context.Context, callID string, deliveryID uint64, actorUserID uint, isAdmin bool) (*APICallDeliveryURL, error) {
	callID = strings.TrimSpace(callID)
	if callID == "" || deliveryID == 0 {
		return nil, ErrAPICallInvalidInput
	}
	if actorUserID == 0 {
		return nil, ErrAPICallAccessDenied
	}
	query := callResultDeliveriesQuery(model.DB().WithContext(ctx)).Where("c.public_id=? AND d.id=?", callID, deliveryID)
	if !isAdmin {
		query = query.Where("c.user_id=?", actorUserID)
	}
	var row callResultDeliveryRow
	if err := query.Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAPICallNotFound
		}
		return nil, err
	}
	now := time.Now().UTC()
	if !row.detail(isAdmin, now).Available {
		return nil, ErrAPICallDeliveryUnavailable
	}
	sqlDB, err := model.DB().DB()
	if err != nil {
		return nil, err
	}
	store, err := repository.New(sqlDB)
	if err != nil {
		return nil, err
	}
	storage, err := store.ReadAttemptFileStorage(ctx, row.AttemptID)
	if err != nil {
		return nil, err
	}
	ttl := 15 * time.Minute
	if row.RetentionUntil != nil && row.RetentionUntil.Sub(now) < ttl {
		ttl = row.RetentionUntil.Sub(now)
	}
	if ttl < time.Second {
		return nil, ErrAPICallDeliveryUnavailable
	}
	value, err := filestorage.WithBaseURLAndAPIKey(storage.XFSBaseURL, storage.APIKey).PresignedURL(ctx, row.Locator, ttl)
	if err != nil {
		return nil, ErrAPICallDeliveryUnavailable
	}
	return &APICallDeliveryURL{URL: value, ExpiresAt: now.Add(ttl)}, nil
}
