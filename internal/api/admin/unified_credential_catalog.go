package admin

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/gateway/adapter"
	"github.com/mirainya/Prism/internal/gateway/catalogsource"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

func PrepareUnifiedCredentialCatalogSource(c *gin.Context) {
	id, err := resp.ParseUintParam(c, "id")
	if err != nil {
		return
	}
	var in *repository.CredentialCatalogSourceInput
	if !unifiedChannelBody(c, &in) {
		return
	}
	if in == nil {
		unifiedChannelError(c, repository.ErrInvalidInput)
		return
	}
	descriptor, ok := adapter.DescriptorFor(catalogsource.DeclarativeCatalogV1, 1)
	if !ok {
		unifiedChannelError(c, repository.ErrInvalidInput)
		return
	}
	in.Adapter = repository.CatalogAdapterInput{Code: descriptor.Code, Version: descriptor.Version, Protocol: descriptor.Protocol,
		ImplementationDigest: descriptor.ImplementationDigest, MinimumSemanticVersion: descriptor.MinimumSemanticVersion}
	in.Normalize()
	if err := in.Validate(); err != nil {
		unifiedChannelError(c, err)
		return
	}
	unifiedChannelWrite(c, func(store *repository.Store, tx *sql.Tx, actor uint64) (uint64, error) {
		return store.PrepareCredentialCatalogSource(c.Request.Context(), tx, uint64(id), *in, actor)
	})
}
