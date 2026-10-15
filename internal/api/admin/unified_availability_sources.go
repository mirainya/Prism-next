package admin

import (
	"database/sql"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/gateway/catalogsource"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/model"
)

func UnifiedAvailabilitySourceOptions(c *gin.Context) {
	db, err := model.DB().DB()
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	ctx := c.Request.Context()
	channelRows, err := db.QueryContext(ctx, `SELECT id,channel_code,display_name FROM gateway_channels WHERE status='active' ORDER BY display_name,id`)
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	channels := make([]gin.H, 0)
	for channelRows.Next() {
		var id uint64
		var code, name string
		if err := channelRows.Scan(&id, &code, &name); err != nil {
			channelRows.Close()
			unifiedChannelError(c, err)
			return
		}
		channels = append(channels, gin.H{"id": id, "code": code, "name": name})
	}
	if err := channelRows.Err(); err != nil {
		channelRows.Close()
		unifiedChannelError(c, err)
		return
	}
	if err := channelRows.Close(); err != nil {
		unifiedChannelError(c, err)
		return
	}
	credentialRows, err := db.QueryContext(ctx, `SELECT DISTINCT c.id,c.channel_id,c.credential_code
FROM gw_credentials c
JOIN gw_credential_versions v ON v.id=c.current_version_id AND v.credential_id=c.id AND v.status='active'
JOIN gw_credential_purpose_grants g ON g.credential_id=c.id AND g.purpose='availability_read' AND g.status='active'
WHERE c.status='active' ORDER BY c.channel_id,c.credential_code,c.id`)
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	defer credentialRows.Close()
	credentials := make([]gin.H, 0)
	for credentialRows.Next() {
		var id, channelID uint64
		var code string
		if err := credentialRows.Scan(&id, &channelID, &code); err != nil {
			unifiedChannelError(c, err)
			return
		}
		credentials = append(credentials, gin.H{"id": id, "channel_id": channelID, "code": code})
	}
	if err := credentialRows.Err(); err != nil {
		unifiedChannelError(c, err)
		return
	}
	providers, err := catalogsource.DefaultProviderRegistry()
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	contracts := make([]gin.H, 0)
	for _, binding := range providers.AvailabilityBindings() {
		name := binding.Contract
		for _, providerContract := range binding.Provider.AvailabilityContracts() {
			if providerContract.Code == binding.Contract {
				name = providerContract.Name
				break
			}
		}
		contracts = append(contracts, gin.H{"code": binding.Contract, "name": name})
	}
	protocolDefinitions, err := unifiedProtocolDefinitionOptions(ctx, db, "availability")
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	resp.Success(c, gin.H{"channels": channels, "credentials": credentials, "contracts": contracts, "protocol_definitions": protocolDefinitions})
}

func ListUnifiedAvailabilitySources(c *gin.Context) {
	page, size, ok := unifiedGatewayPagination(c)
	if !ok {
		return
	}
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	if status != "" && status != "active" && status != "disabled" {
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
	items, total, err := store.ListManagedAvailabilitySources(c.Request.Context(), status, size, (page-1)*size)
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	values := make([]gin.H, 0, len(items))
	for _, item := range items {
		values = append(values, gin.H{
			"id": item.ID, "channel_id": item.ChannelID, "channel_code": item.ChannelCode, "channel_name": item.ChannelName,
			"source_code": item.SourceCode, "contract_code": item.ContractCode, "protocol_definition_id": item.ProtocolDefinitionID, "base_url": item.BaseURL,
			"request_timeout_ms": item.RequestTimeoutMS, "credential_id": item.CredentialID, "credential_code": item.CredentialCode,
			"status": item.Status, "config_version": item.ConfigVersion,
			"created_at": nullableTime(item.CreatedAt), "updated_at": nullableTime(item.UpdatedAt),
		})
	}
	resp.Success(c, unifiedGatewayPage{Items: values, Page: page, PageSize: size, Total: total})
}

func CreateUnifiedAvailabilitySource(c *gin.Context) {
	var in repository.AvailabilitySourceInput
	if !unifiedChannelBody(c, &in) {
		return
	}
	in.Normalize()
	providers, err := catalogsource.DefaultProviderRegistry()
	if err != nil {
		unifiedChannelError(c, err)
		return
	}
	if _, ok := providers.AvailabilityBinding(in.ContractCode); !ok {
		unifiedChannelError(c, repository.ErrInvalidInput)
		return
	}
	unifiedChannelWrite(c, func(store *repository.Store, tx *sql.Tx, actor uint64) (uint64, error) {
		return store.CreateManagedAvailabilitySource(c.Request.Context(), tx, in, actor)
	})
}

func TransitionUnifiedAvailabilitySource(c *gin.Context) {
	id, err := resp.ParseUintParam(c, "source_id")
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
		return uint64(id), store.TransitionManagedAvailabilitySource(c.Request.Context(), tx, uint64(id), in.ExpectedVersion, in.Status, actor)
	})
}

func UpdateUnifiedAvailabilitySource(c *gin.Context) {
	id, err := resp.ParseUintParam(c, "source_id")
	if err != nil {
		return
	}
	var in repository.AvailabilitySourceUpdate
	if !unifiedChannelBody(c, &in) {
		return
	}
	if in.ContractCode != "" {
		in.ContractCode = strings.ToLower(strings.TrimSpace(in.ContractCode))
		providers, err := catalogsource.DefaultProviderRegistry()
		if err != nil {
			unifiedChannelError(c, err)
			return
		}
		if _, ok := providers.AvailabilityBinding(in.ContractCode); !ok {
			unifiedChannelError(c, repository.ErrInvalidInput)
			return
		}
	}
	unifiedChannelWrite(c, func(store *repository.Store, tx *sql.Tx, actor uint64) (uint64, error) {
		return uint64(id), store.UpdateManagedAvailabilitySource(c.Request.Context(), tx, uint64(id), in, actor)
	})
}
