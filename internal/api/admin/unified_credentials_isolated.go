package admin

import (
	"database/sql"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/api/resp"
	"github.com/mirainya/Prism/internal/gateway/credentials"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

type unifiedCredentialCreateInput struct {
	Code         string                `json:"credential_code"`
	Secret       string                `json:"secret"`
	RequestLimit *uint64               `json:"request_limit"`
	TaskLimit    *uint64               `json:"task_limit"`
	Weight       uint64                `json:"weight"`
	Purposes     []credentials.Purpose `json:"purposes"`
}

func (in unifiedCredentialCreateInput) managedInput() repository.ManagedCredentialInput {
	return repository.ManagedCredentialInput{Code: in.Code, Secret: []byte(in.Secret), RequestLimit: in.RequestLimit, TaskLimit: in.TaskLimit, Weight: in.Weight, Purposes: in.Purposes}
}

// CreateUnifiedIsolatedCredential adds a key without inheriting another key's
// model offerings, prices or permissions. Pool and credential writes are atomic.
func CreateUnifiedIsolatedCredential(c *gin.Context) {
	id, err := resp.ParseUintParam(c, "id")
	if err != nil {
		return
	}
	var in unifiedCredentialCreateInput
	if !unifiedChannelBody(c, &in) {
		return
	}
	fields := in.managedInput()
	in.Secret = ""
	defer clear(fields.Secret)
	if err := fields.Validate(); err != nil {
		unifiedChannelError(c, err)
		return
	}
	var result repository.IsolatedCredentialResult
	if !unifiedGatewayWrite(c, func(store *repository.Store, tx *sql.Tx, actor uint64) error {
		var err error
		result, err = store.CreateIsolatedManagedCredential(c.Request.Context(), tx, uint64(id), fields, actor)
		return err
	}) {
		return
	}
	resp.Success(c, result)
}
