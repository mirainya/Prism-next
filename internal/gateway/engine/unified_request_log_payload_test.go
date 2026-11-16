package engine

import (
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/canonical"
	"github.com/mirainya/Prism/internal/gateway/repository"
	gatewayruntime "github.com/mirainya/Prism/internal/gateway/runtime"
	"github.com/mirainya/Prism/internal/gateway/transport/openai"
)

func TestUnifiedRequestLogRetainsWrappedUpstreamErrorBody(t *testing.T) {
	for _, body := range []string{`{"error":{"message":"no upstream available"}}`, "<html>Bad Gateway</html>"} {
		t.Run(body, func(t *testing.T) {
			key := requestLogTestKeys(t)
			db, mock, _ := sqlmock.New()
			defer db.Close()
			store, _ := repository.New(db)
			runtime, _ := gatewayruntime.New(store)
			log := &unifiedRequestLog{service: runtime, id: 8, requestComplete: true}
			digest := requestLogBodyHMAC(key, []byte(body))
			mock.ExpectQuery("SELECT k.id,k.current_version").WillReturnRows(sqlmock.NewRows([]string{"id", "current_version"}).AddRow(3, 2))
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT status FROM gw_channel_request_logs").WithArgs(uint64(8)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("dispatching"))
			for _, state := range []string{"dispatching", "sent"} {
				mock.ExpectQuery("SELECT status FROM gw_channel_request_logs").WithArgs(uint64(8)).WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(state))
				if state == "dispatching" {
					mock.ExpectQuery("SELECT response_payload_blob_id FROM gw_channel_request_logs").WithArgs(uint64(8)).WillReturnRows(sqlmock.NewRows([]string{"response_payload_blob_id"}).AddRow(nil))
					mock.ExpectExec("INSERT INTO encrypted_blobs").WillReturnResult(sqlmock.NewResult(101, 1))
					mock.ExpectExec("UPDATE encrypted_blobs").WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectExec("INSERT INTO encrypted_blob_key_wraps").WillReturnResult(sqlmock.NewResult(0, 1))
					mock.ExpectExec("UPDATE gw_channel_request_logs SET response_payload_blob_id").WithArgs(uint64(101), uint64(8)).WillReturnResult(sqlmock.NewResult(0, 1))
				} else {
					mock.ExpectQuery("SELECT response_payload_blob_id FROM gw_channel_request_logs").WithArgs(uint64(8)).WillReturnRows(sqlmock.NewRows([]string{"response_payload_blob_id"}).AddRow(101))
					mock.ExpectQuery("SELECT purpose,content_hmac,content_length FROM encrypted_blobs").WithArgs(int64(101)).WillReturnRows(sqlmock.NewRows([]string{"purpose", "content_hmac", "content_length"}).AddRow("gateway-upstream-response", digest, len(body)))
				}
				status := "sent"
				if state == "sent" {
					status = "response_recorded"
				}
				mock.ExpectExec("UPDATE gw_channel_request_logs SET status=").WithArgs(status, digest, true, true, uint16(502), uint64(10), "exchange_error", sqlmock.AnyArg(), uint64(8)).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			expectRequestLogSlotRelease(mock)
			mock.ExpectCommit()
			upstreamErr := &openai.HTTPError{Status: 502, Body: []byte(body), Details: &canonical.Error{Status: 502, Raw: []byte(body)}}
			if err := log.finish(false, 0, errors.Join(upstreamErr, errors.New("related failure")), 10*time.Millisecond); err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUnifiedRequestLogPayloadFailureStillReleasesSlot(t *testing.T) {
	t.Setenv("PRISM_GATEWAY_PAYLOAD_KEK_B64", "")
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store, _ := repository.New(db)
	runtime, _ := gatewayruntime.New(store)
	log := &unifiedRequestLog{service: runtime, id: 8, requestComplete: true}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT status FROM gw_channel_request_logs").WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("dispatching"))
	for _, state := range []string{"dispatching", "sent"} {
		mock.ExpectQuery("SELECT status FROM gw_channel_request_logs").WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow(state))
		status := "sent"
		if state == "sent" {
			status = "response_recorded"
		}
		mock.ExpectExec("UPDATE gw_channel_request_logs SET status=").WithArgs(status, nil, true, false, uint16(502), uint64(10), "exchange_error", sqlmock.AnyArg(), uint64(8)).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	expectRequestLogSlotRelease(mock)
	mock.ExpectCommit()
	upstreamErr := &openai.HTTPError{Status: 502, Details: &canonical.Error{Status: 502, Raw: []byte("gateway error")}}
	if err := log.finish(false, 0, upstreamErr, 10*time.Millisecond); err == nil {
		t.Fatal("capture failure was not reported")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func expectRequestLogSlotRelease(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT id,state FROM gw_credential_slots WHERE request_log_id").WithArgs(uint64(8)).WillReturnRows(sqlmock.NewRows([]string{"id", "state"}).AddRow(9, "active"))
	mock.ExpectQuery("SELECT state,scope FROM gw_credential_slots").WithArgs(uint64(9)).WillReturnRows(sqlmock.NewRows([]string{"state", "scope"}).AddRow("active", "request"))
	mock.ExpectExec("UPDATE gw_credential_slots").WithArgs("released", sqlmock.AnyArg(), uint64(9)).WillReturnResult(sqlmock.NewResult(0, 1))
}
