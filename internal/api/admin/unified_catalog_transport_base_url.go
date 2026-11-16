package admin

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/gateway/adapter"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

// ChangeUnifiedTransportBaseURL changes the active catalog transport origin
// and rebuilds its allowed-host authorization in the same transaction.
func ChangeUnifiedTransportBaseURL(c *gin.Context) {
	var in repository.TransportBaseURLChange
	if !unifiedChannelBody(c, &in) {
		return
	}
	in.SemanticDigest = adapter.SemanticDigest()
	in.Normalize()
	if err := in.Validate(); err != nil {
		unifiedChannelError(c, err)
		return
	}
	var result repository.CatalogChangeResult
	if !unifiedGatewayWrite(c, func(store *repository.Store, tx *sql.Tx, actor uint64) error {
		var err error
		result, err = store.ChangeTransportBaseURL(c.Request.Context(), tx, in, actor)
		return err
	}) {
		return
	}
	respondCatalogChange(c, result)
}
