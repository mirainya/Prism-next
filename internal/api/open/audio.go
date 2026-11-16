package open

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/mirainya/Prism/internal/api/middleware"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/gateway/adapter"
	"github.com/mirainya/Prism/internal/gateway/delivery"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/gateway/routing"
	gatewayruntime "github.com/mirainya/Prism/internal/gateway/runtime"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/internal/service"
	"github.com/mirainya/Prism/pkg/safeurl"
)

// CreateAudioGeneration submits a durable speech task. Provider calls and
// credentials remain owned by the existing asynchronous dispatcher.
func CreateAudioGeneration(c *gin.Context) {
	token := middleware.GetToken(c)
	if token == nil {
		openAIError(c, http.StatusUnauthorized, "unauthorized", "authentication_error")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	request, err := adapter.DecodeSpeechRequest(c.Request.Body)
	if err != nil {
		status := http.StatusBadRequest
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			status = http.StatusRequestEntityTooLarge
		}
		openAIError(c, status, err.Error(), "invalid_request_error")
		return
	}
	references := []string{request.ReferenceAudio}
	if request.Emotion != nil {
		references = append(references, request.Emotion.ReferenceAudio)
	}
	for _, location := range references {
		if location != "" && safeurl.Validate(c.Request.Context(), location) != nil {
			openAIError(c, http.StatusBadRequest, "reference audio must resolve to a public HTTP(S) address", "invalid_request_error")
			return
		}
	}
	submission, err := submitAudioGeneration(c.Request.Context(), token, request, c.GetHeader("Idempotency-Key"), middleware.GetRequestID(c.Request.Context()), c.FullPath())
	if err != nil {
		writeAudioExecutionError(c, err)
		return
	}
	c.Header(prismCallIDHeader, submission.PublicID)
	location := "/v1/audio/tasks/" + submission.PublicID
	c.Header("Location", location)
	c.Header("Retry-After", "3")
	c.JSON(http.StatusAccepted, gin.H{"code": 0, "message": "success", "data": gin.H{"id": submission.PublicID, "status": "queued", "operation": adapter.AudioGenerate, "location": location}})
}

func writeAudioExecutionError(c *gin.Context, err error) {
	markRejectedSubmission(c, err)
	switch {
	case errors.Is(err, service.ErrInsufficientTokenBalance), errors.Is(err, service.ErrInsufficientUserBalance), errors.Is(err, repository.ErrInsufficient):
		openAIError(c, http.StatusBadRequest, "insufficient quota", "insufficient_quota")
	case errors.Is(err, repository.ErrInvalidInput):
		openAIError(c, http.StatusBadRequest, "invalid audio request", "invalid_request_error")
	case errors.Is(err, repository.ErrIdempotencyConflict):
		openAIError(c, http.StatusConflict, "idempotency key conflicts with an existing request", "invalid_request_error")
	case errors.Is(err, gatewayruntime.ErrNotReady), errors.Is(err, routing.ErrNoRoute), errors.Is(err, routing.ErrNoCompatibleTransport), errors.Is(err, routing.ErrCapabilityUnavailable):
		openAIError(c, http.StatusServiceUnavailable, "audio generation is unavailable", "service_unavailable_error")
	default:
		openAIError(c, http.StatusInternalServerError, "audio generation failed", "api_error")
	}
}

func submitAudioGeneration(ctx context.Context, token *model.Token, request adapter.SpeechRequest, idempotencyKey, requestID, endpoint string) (gatewayruntime.Submission, error) {
	if unifiedVideoStore == nil || unifiedVideoRouter == nil {
		return gatewayruntime.Submission{}, gatewayruntime.ErrNotReady
	}
	if err := gatewayruntime.RequireConfiguredReadiness(ctx, unifiedVideoStore.DB()); err != nil {
		return gatewayruntime.Submission{}, err
	}
	route, err := unifiedVideoRouter.SelectTransport(ctx, request.Model, nil, routing.RouteOptions{
		SelectionKey: uuid.NewString(), OperationMethod: http.MethodPost, OperationPath: "/v1/audio/generations",
		AllowedTransports:   []model.UpstreamTransport{model.UpstreamTransportAudioGeneration},
		PreferredTransports: []model.UpstreamTransport{model.UpstreamTransportAudioGeneration},
	})
	if err != nil {
		return gatewayruntime.Submission{}, err
	}
	if route == nil || route.ReleaseID == 0 || route.SellSchedule == nil || route.Transport != model.UpstreamTransportAudioGeneration {
		return gatewayruntime.Submission{}, routing.ErrNoRoute
	}
	policy, err := unifiedVideoStore.ReadRoutePolicy(ctx, uint64(route.ReleaseID), uint64(route.SKUID), uint64(route.ProductTransportID))
	if err != nil {
		return gatewayruntime.Submission{}, err
	}
	codec, ok := adapter.AsyncCodecFor(policy.AdapterCode, policy.AdapterVersion)
	if !ok || policy.TaskScope != "task" || policy.CancelMode != "none" || policy.DeliveryMode != "managed_copy" {
		return gatewayruntime.Submission{}, repository.ErrConflict
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return gatewayruntime.Submission{}, err
	}
	defer clear(payload)
	path, _ := route.TransportConfig["request_path"].(string)
	_, err = codec.Prepare(ctx, "submit", repository.AsyncDispatch{BaseURL: route.BaseURL, Method: routeMethod(route), Path: path, VendorModel: route.VendorModel,
		AdapterConfig: policy.AdapterConfig, DeliveryMode: policy.DeliveryMode}, payload, "")
	if err != nil {
		return gatewayruntime.Submission{}, repository.ErrInvalidInput
	}
	payloadKEK, err := gatewayKey("PRISM_GATEWAY_PAYLOAD_KEK_B64")
	if err != nil {
		return gatewayruntime.Submission{}, err
	}
	defer clear(payloadKEK)
	payloadHMAC, err := gatewayKey("PRISM_GATEWAY_PAYLOAD_HMAC_B64")
	if err != nil {
		return gatewayruntime.Submission{}, err
	}
	defer clear(payloadHMAC)
	var keyringID uint64
	var keyringVersion uint32
	if err := unifiedVideoStore.DB().QueryRowContext(ctx, `SELECT k.id,k.current_version FROM crypto_keyring_state k JOIN crypto_key_versions v ON v.keyring_id=k.id AND v.key_version=k.current_version AND v.status='current' WHERE k.purpose='gateway-payload'`).Scan(&keyringID, &keyringVersion); err != nil {
		return gatewayruntime.Submission{}, err
	}
	idempotency, err := imageIdempotencyWithRequest(idempotencyKey, uint64(token.ID), uint64(route.OperationContractID), keyringVersion, payloadHMAC, policy.IdempotencyMode, payload)
	if err != nil {
		return gatewayruntime.Submission{}, err
	}
	service, err := gatewayruntime.New(unifiedVideoStore)
	if err != nil {
		return gatewayruntime.Submission{}, err
	}
	return service.SubmitCapability(ctx, gatewayruntime.CapabilitySubmitInput{
		PublicID: uuid.NewString(), RequestID: requestID, Endpoint: endpoint, Operation: adapter.AudioGenerate,
		UserID: uint64(token.UserID), TokenID: uint64(token.ID), ServiceTier: "standard",
		RequestPayload: payload, PayloadKeyringID: keyringID, PayloadKEKVersion: keyringVersion, PayloadKEK: payloadKEK, PayloadHMAC: payloadHMAC,
		ResourceSummary: map[string]any{"model": request.Model, "operation": adapter.AudioGenerate}, Idempotency: idempotency,
		Route: gatewayruntime.RouteSnapshot{
			PublicModel: request.Model, VendorModel: route.VendorModel, ReleaseID: uint64(route.ReleaseID), OperationContractID: uint64(route.OperationContractID),
			ModelOperationID: uint64(route.ModelOperationID), SKUID: uint64(route.SKUID), RouteID: uint64(route.RouteID), OfferingID: uint64(route.OfferingID), CostPlanID: uint64(route.CostPlanID),
			ProductTransportID: uint64(route.ProductTransportID), CredentialPoolID: uint64(route.CredentialPoolID), CredentialID: uint64(route.CredentialID), CredentialVersionID: uint64(route.CredentialVersionID),
			PurposeGrantID: uint64(route.PurposeGrantID), Currency: route.Currency, CurrencyVersion: uint32(route.CurrencyVersion), Schedule: *route.SellSchedule,
			DeliveryMode: policy.DeliveryMode, IdempotencyMode: policy.IdempotencyMode, TaskScope: policy.TaskScope, CancelMode: policy.CancelMode,
			SourceURLPolicy: policy.SourceURLPolicy, UpstreamScopeKind: policy.UpstreamScopeKind, UpstreamScopeKey: policy.UpstreamScopeKey, ServiceTiers: policy.ServiceTiers,
		},
	})
}

func GetAudioTask(c *gin.Context) {
	token := middleware.GetToken(c)
	if token == nil {
		resp.ErrorMsg(c, http.StatusUnauthorized, 401, "unauthorized")
		return
	}
	if unifiedVideoStore == nil {
		resp.ErrorMsg(c, http.StatusServiceUnavailable, 503, gatewayruntime.ErrNotReady.Error())
		return
	}
	var callID uint64
	var callStatus, taskStatus string
	var progress uint8
	var summaryRaw []byte
	var createdAt, updatedAt time.Time
	err := unifiedVideoStore.DB().QueryRowContext(c.Request.Context(), `SELECT c.id,c.status,t.status,t.progress,t.parameter_summary,t.created_at,t.updated_at
FROM gw_api_resources r
JOIN gw_api_calls c ON c.id=r.call_id AND c.user_id=r.user_id AND c.token_id=r.token_id
JOIN gw_capability_tasks t ON t.resource_id=r.id
WHERE r.public_id=? AND r.resource_kind='capability_task' AND r.user_id=? AND r.token_id=? AND r.deleted_at IS NULL`, c.Param("id"), token.UserID, token.ID).
		Scan(&callID, &callStatus, &taskStatus, &progress, &summaryRaw, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		resp.ErrorMsg(c, http.StatusNotFound, 404, "audio task not found")
		return
	}
	if err != nil {
		resp.ErrorMsg(c, http.StatusInternalServerError, 500, "audio task lookup failed")
		return
	}
	var summary struct {
		Model     string `json:"model"`
		Operation string `json:"operation"`
	}
	if json.Unmarshal(summaryRaw, &summary) != nil || summary.Operation != adapter.AudioGenerate {
		resp.ErrorMsg(c, http.StatusNotFound, 404, "audio task not found")
		return
	}
	status := imageTaskPublicStatus(callStatus, taskStatus)
	result := gin.H{"id": c.Param("id"), "status": status, "progress": progress, "model": summary.Model, "operation": summary.Operation,
		"created_at": createdAt.UTC().Format(time.RFC3339), "updated_at": updatedAt.UTC().Format(time.RFC3339)}
	if status == "completed" {
		value, err := readAudioResult(c.Request.Context(), callID, uint64(token.UserID), uint64(token.ID))
		if err != nil {
			resp.ErrorMsg(c, http.StatusInternalServerError, 500, "audio task result is unavailable")
			return
		}
		result["result"] = value
	}
	if status == "failed" {
		result["error"] = gin.H{"code": "audio_task_failed", "message": "audio generation failed"}
	}
	resp.Success(c, result)
}

func readAudioResult(ctx context.Context, callID, userID, tokenID uint64) (gin.H, error) {
	var payloadID uint64
	if err := unifiedVideoStore.DB().QueryRowContext(ctx, `SELECT c.result_payload_id FROM gw_api_calls c JOIN gw_api_resources r ON r.call_id=c.id AND r.resource_kind='capability_task' WHERE c.id=? AND c.user_id=? AND c.token_id=? AND c.status='completed'`, callID, userID, tokenID).Scan(&payloadID); err != nil {
		return nil, err
	}
	plain, err := readUnifiedPayload(ctx, payloadID, callID, "result")
	if err != nil {
		return nil, err
	}
	defer clear(plain)
	var result delivery.AudioResult
	if json.Unmarshal(plain, &result) != nil || result.Kind != delivery.AudioResultKind || result.SchemaVersion != delivery.ResultSchemaVersion || !delivery.ValidAudioFormat(result.Format) || result.DeliveryID == 0 {
		return nil, delivery.ErrInvalidResult
	}
	location, err := readUnifiedDeliveryURL(ctx, result.DeliveryID, callID, uint(userID), uint(tokenID))
	if err != nil {
		return nil, err
	}
	return gin.H{"url": location, "format": result.Format}, nil
}
