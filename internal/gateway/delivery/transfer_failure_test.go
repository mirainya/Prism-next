package delivery

import "testing"

func TestTransferFailureRoundTripAndRetryPolicy(t *testing.T) {
	for _, failure := range []TransferFailure{
		{Stage: "upload", Code: "http", HTTPStatus: 403},
		{Stage: "import", Code: "http", HTTPStatus: 503, Retryable: true},
		{Stage: "verify", Code: "timeout", Retryable: true},
		{Stage: "upload", Code: "capacity"},
		{Stage: "verify", Code: "digest"},
	} {
		reason := failure.Reason()
		got, ok := ParseTransferFailure(reason)
		if !ok || got != failure || len(reason) > 64 || RetryableManagedCopyFailure(reason) != failure.Retryable {
			t.Fatalf("reason=%q decoded=%+v", reason, got)
		}
	}
	if _, ok := ParseTransferFailure("managed_copy_upload_retry_http_503?key=secret"); ok {
		t.Fatal("accepted an unstructured diagnostic")
	}
}
