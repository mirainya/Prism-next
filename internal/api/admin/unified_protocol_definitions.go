package admin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/gateway/catalogsource"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
)

func unifiedProtocolDefinitionOptions(ctx context.Context, db *sql.DB, kind string) ([]gin.H, error) {
	rows, err := db.QueryContext(ctx, `SELECT id,protocol_code,protocol_version,display_name FROM gw_protocol_definitions WHERE protocol_kind=? AND status='active' ORDER BY protocol_code,protocol_version DESC`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]gin.H, 0)
	for rows.Next() {
		var id uint64
		var code, name string
		var version uint32
		if err := rows.Scan(&id, &code, &version, &name); err != nil {
			return nil, err
		}
		items = append(items, gin.H{"id": id, "protocol_code": code, "protocol_version": version, "display_name": name, "protocol_kind": kind})
	}
	return items, rows.Err()
}

func ListUnifiedProtocolDefinitions(c *gin.Context) {
	page, size, ok := unifiedGatewayPagination(c)
	if !ok {
		return
	}
	kind := strings.ToLower(strings.TrimSpace(c.Query("kind")))
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
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
	items, total, err := store.ListProtocolDefinitions(c.Request.Context(), kind, status, size, (page-1)*size)
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	values := make([]gin.H, 0, len(items))
	for _, item := range items {
		values = append(values, gin.H{
			"id": item.ID, "protocol_code": item.ProtocolCode, "protocol_version": item.ProtocolVersion,
			"protocol_kind": item.ProtocolKind, "display_name": item.DisplayName,
			"definition_digest": item.DefinitionDigest, "status": item.Status,
			"config_version": item.ConfigVersion, "created_at": nullableTime(item.CreatedAt), "updated_at": nullableTime(item.UpdatedAt),
		})
	}
	resp.Success(c, unifiedGatewayPage{Items: values, Page: page, PageSize: size, Total: total})
}

type createUnifiedProtocolDefinitionRequest struct {
	ProtocolCode    string          `json:"protocol_code"`
	ProtocolVersion uint32          `json:"protocol_version"`
	ProtocolKind    string          `json:"protocol_kind"`
	DisplayName     string          `json:"display_name"`
	Definition      json.RawMessage `json:"definition"`
}

func CreateUnifiedProtocolDefinition(c *gin.Context) {
	var request createUnifiedProtocolDefinitionRequest
	if !unifiedChannelBody(c, &request) {
		return
	}
	request.ProtocolCode = strings.ToLower(strings.TrimSpace(request.ProtocolCode))
	request.ProtocolKind = strings.ToLower(strings.TrimSpace(request.ProtocolKind))
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	if len(request.Definition) == 0 {
		unifiedChannelError(c, repository.ErrInvalidInput)
		return
	}
	definition, err := catalogsource.ValidateDeclarativeProtocol(request.Definition)
	if err != nil || definition.Kind != request.ProtocolKind {
		unifiedChannelError(c, repository.ErrInvalidInput)
		return
	}
	canonical, err := repository.CanonicalProtocolJSON(request.Definition)
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	digest := sha256.Sum256(canonical)
	in := repository.ProtocolDefinitionInput{
		ProtocolCode: request.ProtocolCode, ProtocolVersion: request.ProtocolVersion,
		ProtocolKind: request.ProtocolKind, DisplayName: request.DisplayName,
		DefinitionJSON: canonical, DefinitionDigest: hex.EncodeToString(digest[:]),
	}
	unifiedChannelWrite(c, func(store *repository.Store, tx *sql.Tx, actor uint64) (uint64, error) {
		return store.CreateProtocolDefinition(c.Request.Context(), tx, in, actor)
	})
}

func ReadUnifiedProtocolDefinition(c *gin.Context) {
	id, err := resp.ParseUintParam(c, "definition_id")
	if err != nil {
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
	item, err := store.ReadProtocolDefinition(c.Request.Context(), uint64(id))
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	resp.Success(c, gin.H{"id": item.ID, "protocol_code": item.ProtocolCode, "protocol_version": item.ProtocolVersion, "protocol_kind": item.ProtocolKind, "display_name": item.DisplayName, "definition": json.RawMessage(item.DefinitionJSON), "definition_digest": item.DefinitionDigest, "status": item.Status, "config_version": item.ConfigVersion})
}

func TransitionUnifiedProtocolDefinition(c *gin.Context) {
	id, err := resp.ParseUintParam(c, "definition_id")
	if err != nil {
		return
	}
	var in struct {
		Status          string `json:"status"`
		ExpectedVersion uint64 `json:"expected_version"`
	}
	if !unifiedChannelBody(c, &in) {
		return
	}
	in.Status = strings.ToLower(strings.TrimSpace(in.Status))
	if in.ExpectedVersion == 0 || in.Status != "active" && in.Status != "disabled" {
		unifiedChannelError(c, repository.ErrInvalidInput)
		return
	}
	unifiedChannelWrite(c, func(store *repository.Store, tx *sql.Tx, actor uint64) (uint64, error) {
		return uint64(id), store.TransitionProtocolDefinition(c.Request.Context(), tx, uint64(id), in.ExpectedVersion, in.Status, actor)
	})
}
