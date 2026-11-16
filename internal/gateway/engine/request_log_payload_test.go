package engine

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/internal/gateway/security"
)

func requestLogTestKeys(t *testing.T) []byte {
	t.Helper()
	key := []byte("01234567890123456789012345678901")
	encoded := base64.StdEncoding.EncodeToString(key)
	t.Setenv("PRISM_GATEWAY_PAYLOAD_KEK_B64", encoded)
	t.Setenv("PRISM_GATEWAY_PAYLOAD_HMAC_B64", encoded)
	return key
}

func TestRequestLogPayloadPreservesExactBodyAndClearsCopies(t *testing.T) {
	requestLogTestKeys(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := repository.New(db)
	mock.ExpectQuery("SELECT k.id,k.current_version").WillReturnRows(sqlmock.NewRows([]string{"id", "current_version"}).AddRow(3, 2))
	body := []byte(" {\"model\":\"vendor\", \"messages\":[{\"role\":\"user\",\"content\":\"你好\"}]}\n")
	blob, cleanup, err := requestLogPayload(t.Context(), store, body)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if blob.KeyringID != 3 || blob.KEKVersion != 2 || !bytes.Equal(blob.Plaintext, body) || len(blob.KEK) != security.KeySize || blob.RetentionUntil == nil || !blob.RetentionUntil.After(time.Now()) {
		t.Fatal("raw evidence or keyring metadata was changed")
	}
	cleanup()
	if !bytes.Equal(blob.Plaintext, make([]byte, len(blob.Plaintext))) || !bytes.Equal(blob.KEK, make([]byte, security.KeySize)) || !bytes.Equal(blob.HMACKey, make([]byte, security.KeySize)) || body[0] != ' ' {
		t.Fatal("cleanup did not clear copies independently of the outgoing body")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestLogPayloadBoundsEvidenceWithoutChangingRequest(t *testing.T) {
	requestLogTestKeys(t)
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store, _ := repository.New(db)
	mock.ExpectQuery("SELECT k.id,k.current_version").WillReturnRows(sqlmock.NewRows([]string{"id", "current_version"}).AddRow(3, 2))
	body := bytes.Repeat([]byte("x"), maxRequestLogPayloadBytes+8)
	blob, cleanup, err := requestLogPayload(t.Context(), store, body)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if len(blob.Plaintext) != maxRequestLogPayloadBytes || len(body) != maxRequestLogPayloadBytes+8 || !bytes.Equal(blob.Plaintext, body[:maxRequestLogPayloadBytes]) {
		t.Fatal("capture limit modified the executable request")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestLogPayloadFailsWhenEvidenceKeysAreMissing(t *testing.T) {
	t.Setenv("PRISM_GATEWAY_PAYLOAD_KEK_B64", "")
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store, _ := repository.New(db)
	if _, cleanup, err := requestLogPayload(t.Context(), store, []byte(`{"model":"vendor"}`)); err == nil {
		cleanup()
		t.Fatal("missing evidence key silently skipped capture")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestLogPayloadReturnsKeyringLookupFailure(t *testing.T) {
	requestLogTestKeys(t)
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store, _ := repository.New(db)
	want := errors.New("keyring unavailable")
	mock.ExpectQuery("SELECT k.id,k.current_version").WillReturnError(want)
	if _, cleanup, err := requestLogPayload(t.Context(), store, []byte("gateway failure")); !errors.Is(err, want) {
		cleanup()
		t.Fatalf("keyring error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
