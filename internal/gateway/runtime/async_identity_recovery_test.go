package runtime

import (
	"bytes"
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/execution"
	"github.com/mirainya/Prism/internal/gateway/repository"
)

type identityEvidenceCodec struct{ testAsyncCodec }

func (identityEvidenceCodec) DecodeWithDispatch(action string, fixed repository.AsyncDispatch, request, response []byte) (AsyncObservation, error) {
	if action != "submit" || fixed.PublicID != "gateway-uuid" || string(request) != "request" || string(response) != "submission-evidence" {
		return AsyncObservation{}, repository.ErrInvalidInput
	}
	return AsyncObservation{State: execution.AsyncAccepted, TaskID: "test-task-123"}, nil
}

func TestMissingIdentityRecoveredFromSubmissionUsingOriginalCodec(t *testing.T) {
	dispatcher, mock := testDispatcher(t, nil)
	dispatcher.codecs["test@1"] = identityEvidenceCodec{}
	fixed := repository.AsyncDispatch{CallID: 11, RequestBlobID: 5, PublicID: "gateway-uuid", AdapterCode: "test", AdapterVersion: 1}
	mock.ExpectQuery(`SELECT id,response_payload_blob_id FROM gw_channel_request_logs`).WithArgs(uint64(9)).WillReturnRows(sqlmock.NewRows([]string{"id", "blob"}).AddRow(31, 6))
	key := bytes.Repeat([]byte{13}, 32)
	expectCallbackWorkerBlob(t, mock, 6, "gateway-upstream-response", []byte("request-log:31:response"), []byte("submission-evidence"), key)
	expectCallbackWorkerBlob(t, mock, 5, "gateway-payload", []byte("call:11:request"), []byte("request"), key)
	id, err := dispatcher.identityFromSubmission(context.Background(), 9, fixed)
	if err != nil || id != "test-task-123" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMissingSubmissionEvidenceRequestsManualIdentity(t *testing.T) {
	dispatcher, mock := testDispatcher(t, nil)
	mock.ExpectQuery(`SELECT id,response_payload_blob_id FROM gw_channel_request_logs`).WithArgs(uint64(9)).WillReturnError(sql.ErrNoRows)
	_, err := dispatcher.identityFromSubmission(context.Background(), 9, repository.AsyncDispatch{})
	if err != ErrProviderTaskIdentityRequired {
		t.Fatalf("err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
