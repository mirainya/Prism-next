package runtime

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/delivery"
	"github.com/mirainya/Prism/internal/gateway/execution"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/pkg/filestorage"
)

type savedImageCodec struct{ testAsyncCodec }

func (savedImageCodec) Prepare(context.Context, string, repository.AsyncDispatch, []byte, string) (AsyncRequest, error) {
	panic("a transfer recovery must never prepare a generation request")
}

func (savedImageCodec) Decode(action string, request, response []byte) (AsyncObservation, error) {
	if action != "submit" || string(request) != "original-request" || string(response) != "original-response" {
		return AsyncObservation{}, repository.ErrConflict
	}
	return AsyncObservation{State: execution.AsyncSucceeded, Sources: []delivery.RemoteResult{
		{Role: "image", InlineData: []byte("first"), ContentType: "image/png"},
		{Role: "image", InlineData: []byte("second"), ContentType: "image/png"},
	}}, nil
}

func TestManagedCopyRecoveryDecodesOriginalResponseWithoutUpstreamCall(t *testing.T) {
	d, mock := testDispatcher(t, nil)
	d.codecs["test@1"] = savedImageCodec{}
	mock.ExpectQuery("SELECT c.id,a.id,a.credential_id").WithArgs(uint64(9)).WillReturnRows(sqlmock.NewRows([]string{
		"call", "attempt", "credential", "secret", "credential_blob", "request_blob", "identity_blob", "callback_blob", "transport", "release", "public", "protocol", "base", "method", "path", "auth", "model", "mode", "policy", "config", "timeout", "adapter", "version",
	}).AddRow(1, 2, 3, nil, 4, 5, nil, nil, 6, 7, "call", "http", "https://upstream.invalid", "POST", "/images", "bearer", "image", "managed_copy", "fixed", []byte(`{}`), 600000, "test", 1))
	mock.ExpectQuery("SELECT l.id,l.response_payload_blob_id,l.response_bytes_hmac,l.action").
		WithArgs(uint64(2), uint64(9)).WillReturnRows(sqlmock.NewRows([]string{"request", "blob", "digest", "action"}).
		AddRow(31, 6, d.digest([]byte("original-response")), "submit"))
	key := bytes.Repeat([]byte{13}, 32)
	expectCallbackWorkerBlob(t, mock, 6, "gateway-upstream-response", []byte("request-log:31:response"), []byte("original-response"), key)
	expectCallbackWorkerBlob(t, mock, 5, "gateway-payload", []byte("call:1:request"), []byte("original-request"), key)
	source, err := d.savedManagedCopySource(context.Background(), repository.DeliveryRecoveryTarget{AttemptID: 2, AsyncExecutionID: 9, Ordinal: 1, ResourceKind: delivery.ResourceCapabilityTask})
	if err != nil || string(source.InlineData) != "second" {
		t.Fatalf("recovered wrong image ordinal: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInlineManagedCopyRetryUsesOriginalStorageAndOrdinal(t *testing.T) {
	d, mock := testDispatcher(t, nil)
	expectManagedOwner(mock, 2, 10, 20)
	expectManagedAllocation(mock, 2, 1, 10, 20, "gateway-result:2:3", 31)
	expectManagedUploadRecord(mock, 31)
	restore := stubManagedStorage(t)
	defer restore()
	uploadManagedResult = func(_ context.Context, storage repository.TokenFileStorage, data []byte, mime, _, _ string) (filestorage.UploadResult, error) {
		if storage.APIKey != testStorageAPIKey || storage.XFSBaseURL != testStorageBaseURL || !bytes.Equal(data, testPNG) || mime != "image/png" {
			t.Fatal("recovery changed the original image or storage snapshot")
		}
		return filestorage.UploadResult{RawURL: "original/result.png"}, nil
	}
	copy, err := d.service.prepareManagedResultCopyAt(context.Background(), 2, 3, delivery.RemoteResult{Role: "image", InlineData: append([]byte(nil), testPNG...), ContentType: "image/png"})
	if err != nil || copy.MediaAssetID != 31 || copy.ObjectKey != "gateway-result:2:3" {
		t.Fatalf("copy=%+v err=%v", copy, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestManagedTransferDiagnosticNeverPersistsRawError(t *testing.T) {
	if reason := managedTransferFailure("upload", errors.New("POST https://private.example?signature=secret Authorization: Bearer secret")); reason != "managed_copy_upload_stop_unknown" || strings.Contains(reason, "secret") {
		t.Fatalf("reason=%s", reason)
	}
}

func TestManagedTransferLeavesTimeToCommitSuccessfulGeneration(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	parent, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	transfer, finish := managedTransferContext(parent)
	defer finish()
	actual, ok := transfer.Deadline()
	if !ok || !actual.Equal(deadline.Add(-30*time.Second)) {
		t.Fatal("transfer consumed the generation result commit budget")
	}
}
