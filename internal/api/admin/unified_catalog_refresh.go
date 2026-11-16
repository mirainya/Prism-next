package admin

import (
	"database/sql"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

func RefreshUnifiedActiveCatalog(c *gin.Context) {
	releaseID, sourceID, ok := catalogResourceIDs(c, "source_id")
	if !ok {
		return
	}
	var in *struct{}
	if !unifiedChannelBody(c, &in) {
		return
	}
	if in == nil {
		unifiedChannelError(c, repository.ErrInvalidInput)
		return
	}
	unifiedChannelWrite(c, func(store *repository.Store, tx *sql.Tx, actor uint64) (uint64, error) {
		return store.ScheduleActiveCatalogDiscovery(c.Request.Context(), tx, releaseID, sourceID, actor)
	})
}

// Scope discovery history to the actual source/Key used by the run. Never
// infer Key membership from a shared pool or from a current model route.
func catalogDiscoveryScope(c *gin.Context, releaseID uint64) (string, []any, bool) {
	where := ` WHERE rs.release_id=? AND r.action='catalog_discovery'`
	args := []any{releaseID}
	for _, filter := range []struct{ query, column string }{
		{"source_id", "rs.catalog_source_id"},
		{"credential_id", "r.credential_id"},
	} {
		raw, present := c.GetQuery(filter.query)
		if !present {
			continue
		}
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || id == 0 {
			unifiedChannelError(c, repository.ErrInvalidInput)
			return "", nil, false
		}
		where += " AND " + filter.column + "=?"
		args = append(args, id)
	}
	return where, args, true
}
