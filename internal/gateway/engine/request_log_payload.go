package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/gateway/security"
	"github.com/mirainya/Prism/pkg/config"
)

// Bound retained evidence without changing the request sent to the provider.
const maxRequestLogPayloadBytes = 1 << 20

func requestLogPayload(ctx context.Context, store *repository.Store, body []byte) (*repository.BlobInput, func(), error) {
	cleanup := func() {}
	if len(body) == 0 {
		return nil, cleanup, nil
	}
	kek, err := decodeGatewayKey("PRISM_GATEWAY_PAYLOAD_KEK_B64")
	if err != nil {
		return nil, cleanup, err
	}
	hmacKey, err := decodeGatewayHMACKey("PRISM_GATEWAY_PAYLOAD_HMAC_B64")
	if err != nil {
		clear(kek)
		return nil, cleanup, err
	}
	var keyringID uint64
	var version uint32
	err = store.DB().QueryRowContext(ctx, `SELECT k.id,k.current_version FROM crypto_keyring_state k
JOIN crypto_key_versions v ON v.keyring_id=k.id AND v.key_version=k.current_version AND v.status='current'
WHERE k.purpose='gateway-payload'`).Scan(&keyringID, &version)
	if err == nil && (keyringID == 0 || version == 0) {
		err = repository.ErrConflict
	}
	if err != nil {
		clear(kek)
		clear(hmacKey)
		return nil, cleanup, fmt.Errorf("load request log payload keyring: %w", err)
	}
	retentionUntil := time.Now().UTC().Add(config.APICallPayloadRetentionDuration())
	blob := &repository.BlobInput{
		KeyringID: keyringID, KEKVersion: version,
		Plaintext: append([]byte(nil), body[:min(len(body), maxRequestLogPayloadBytes)]...),
		KEK:       kek, HMACKey: hmacKey, RetentionUntil: &retentionUntil,
	}
	cleanup = func() {
		clear(blob.Plaintext)
		clear(blob.KEK)
		clear(blob.HMACKey)
	}
	return blob, cleanup, nil
}

func requestLogBodyHMAC(key, body []byte) string {
	digest := security.HMACSHA256(key, body)
	return fmt.Sprintf("%x", digest[:])
}
