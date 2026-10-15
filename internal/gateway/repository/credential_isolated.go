package repository

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

type IsolatedCredentialResult struct {
	ID     uint64 `json:"id"`
	PoolID uint64 `json:"pool_id"`
}

// CreateIsolatedManagedCredential creates a new pool and its first credential
// in the caller's transaction. It never attaches existing model offerings to
// the pool: prices and permissions must be configured independently.
func (s *Store) CreateIsolatedManagedCredential(ctx context.Context, tx *sql.Tx, channelID uint64, in ManagedCredentialInput, actorID uint64) (IsolatedCredentialResult, error) {
	if tx == nil || channelID == 0 || actorID == 0 {
		return IsolatedCredentialResult{}, ErrInvalidInput
	}
	if err := in.Validate(); err != nil {
		return IsolatedCredentialResult{}, err
	}
	poolID, err := s.CreateManagedCredentialPool(ctx, tx, PoolInput{
		ChannelID: channelID, PoolCode: "key-" + uuid.NewString(), DisplayName: in.Code,
	}, actorID)
	if err != nil {
		return IsolatedCredentialResult{}, err
	}
	credentialID, err := s.CreateManagedCredentialPlaintext(ctx, tx, poolID, in, actorID)
	if err != nil {
		return IsolatedCredentialResult{}, err
	}
	return IsolatedCredentialResult{ID: credentialID, PoolID: poolID}, nil
}
