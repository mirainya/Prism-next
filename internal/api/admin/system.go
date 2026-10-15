package admin

import (
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/resp"
)

// BuildInfo is the immutable build identity exposed to authenticated
// operators. It does not require a database connection.
type BuildInfo struct {
	Version        string `json:"version"`
	BuildTime      string `json:"build_time"`
	SourceRevision string `json:"source_revision"`
}

var buildInfo atomic.Value

func init() {
	buildInfo.Store(BuildInfo{Version: "dev", BuildTime: "unknown", SourceRevision: "dev"})
}

// SetBuildInfo publishes the process build identity before serving requests.
func SetBuildInfo(info BuildInfo) {
	if info.Version == "" {
		info.Version = "dev"
	}
	if info.BuildTime == "" {
		info.BuildTime = "unknown"
	}
	if info.SourceRevision == "" {
		info.SourceRevision = "dev"
	}
	buildInfo.Store(info)
}

func SystemVersion(c *gin.Context) {
	resp.Success(c, buildInfo.Load().(BuildInfo))
}
