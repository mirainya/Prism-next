package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mirainya/Prism/internal/gateway/execution"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

var ErrProviderTaskIdentityRequired = errors.New("provider task identity required")

// ContinueAsyncTask reuses the bound identity. Only an absent binding may be
// repaired from recorded submission evidence or an explicit operator value.
func (d *AsyncDispatcher) ContinueAsyncTask(ctx context.Context, asyncID, operatorID uint64, provided string) error {
	fixed, err := d.service.Store.ReadAsyncDispatch(ctx, asyncID)
	if err != nil {
		return err
	}
	defer clear(fixed.CredentialSecret)
	if fixed.TaskIdentityBlobID != 0 {
		// Verify readability before scheduling; never overwrite a bound identity.
		id, err := d.openBlob(ctx, fixed.TaskIdentityBlobID, fmt.Sprintf("async:%d:task-identity", asyncID), "gateway-task-identity", false)
		if err != nil {
			return err
		}
		defer clear(id)
		if provided != "" && provided != string(id) {
			return repository.ErrConflict
		}
		return d.service.RecoverAsyncTask(ctx, asyncID, operatorID, repository.BlobInput{})
	}
	recovered, err := d.identityFromSubmission(ctx, asyncID, fixed)
	if err != nil && !errors.Is(err, ErrProviderTaskIdentityRequired) {
		return err
	}
	if recovered != "" {
		if provided != "" && provided != recovered {
			return repository.ErrConflict
		}
		provided = recovered
	}
	if strings.TrimSpace(provided) == "" {
		return ErrProviderTaskIdentityRequired
	}
	blob, err := d.payloadBlob(ctx, []byte(provided))
	if err != nil {
		return err
	}
	defer clear(blob.Plaintext)
	return d.service.RecoverAsyncTask(ctx, asyncID, operatorID, blob)
}

func (d *AsyncDispatcher) identityFromSubmission(ctx context.Context, asyncID uint64, fixed repository.AsyncDispatch) (string, error) {
	var requestID, blobID uint64
	err := d.service.Store.DB().QueryRowContext(ctx, `SELECT id,response_payload_blob_id FROM gw_channel_request_logs WHERE async_execution_id=? AND action='submit' AND http_status>=200 AND http_status<300 AND response_bytes_complete=1 AND response_payload_blob_id IS NOT NULL ORDER BY request_seq DESC LIMIT 1`, asyncID).Scan(&requestID, &blobID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrProviderTaskIdentityRequired
	}
	if err != nil {
		return "", err
	}
	response, err := d.openBlob(ctx, blobID, fmt.Sprintf("request-log:%d:response", requestID), "gateway-upstream-response", false)
	if errors.Is(err, repository.ErrNotFound) {
		return "", ErrProviderTaskIdentityRequired
	}
	if err != nil {
		return "", err
	}
	defer clear(response)
	request, err := d.openBlob(ctx, fixed.RequestBlobID, fmt.Sprintf("call:%d:request", fixed.CallID), "gateway-payload", false)
	if err != nil {
		return "", err
	}
	defer clear(request)
	codec, ok := d.codecs[fmt.Sprintf("%s@%d", fixed.AdapterCode, fixed.AdapterVersion)]
	if !ok {
		return "", repository.ErrInvalidInput
	}
	var observation AsyncObservation
	if aware, ok := codec.(DispatchAwareAsyncCodec); ok {
		observation, err = aware.DecodeWithDispatch("submit", fixed, request, response)
	} else {
		observation, err = codec.Decode("submit", request, response)
	}
	defer clear(observation.Result)
	if err != nil || (observation.State != execution.AsyncAccepted && observation.State != execution.AsyncRunning) || strings.TrimSpace(observation.TaskID) == "" {
		return "", ErrProviderTaskIdentityRequired
	}
	return observation.TaskID, nil
}
