package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

func TestMissingOpenBillingAccountRejectsBeforeCreatingCall(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := repository.New(db)
	service, _ := New(store)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT k.id FROM crypto_keyring_state").
		WithArgs(uint64(1), uint32(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
	mock.ExpectQuery("SELECT id FROM billing_accounts").
		WithArgs(uint64(1), "CREDIT", uint32(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()
	_, err = service.Submit(context.Background(), SubmitInput{
		Call:        repository.CreateCallInput{UserID: 1, TokenID: 44, Currency: "CREDIT", CurrencyVersion: 1},
		Reservation: repository.ReservationInput{TokenID: 44, Currency: "CREDIT", CurrencyVersion: 1},
		RequestPayload: repository.BlobInput{
			Plaintext: []byte(`{"model":"image"}`), KEK: make([]byte, 32), HMACKey: make([]byte, 32), KeyringID: 1, KEKVersion: 1,
		},
	})
	if !errors.Is(err, ErrSubmissionRejected) || !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected explicit pre-creation rejection preserving its cause, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
