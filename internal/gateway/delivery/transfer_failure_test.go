package delivery

import "testing"

func TestTransferFailureRoundTripAndRetryPolicy(t *testing.T) {
	for _, failure := range []TransferFailure{
		{Stage: "upload", Code: "http", HTTPStatus: 403},
		{Stage: "import", Code: "http", HTTPStatus: 503, Retryable: true},
		{Stage: "verify", Code: "timeout", Retryable: true},
		{Stage: "upload", Code: "capacity"},
		{Stage: "verify", Code: "digest"},
		{Stage: "import", Code: "pending", Retryable: true},
		{Stage: "import", Code: "uncertain", Retryable: true},
		{Stage: "import", Code: "uncertain"},
		{Stage: "import", Code: "binding"},
		{Stage: "import", Code: "source_resume"},
		{Stage: "import", Code: "transfer_aborted"},
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
	for _, reason := range []string{"managed_copy_import_stop_source_resume?secret=x", "managed_copy_import_stop_untrusted_message", "managed_copy_upload_stop_source_resume"} {
		if _, ok := ParseTransferFailure(reason); ok {
			t.Fatalf("accepted unsupported classification: %s", reason)
		}
	}
}

func TestManagedCopyWaitingExcludesFailuresAndStoppedQueries(t *testing.T) {
	for _, reason := range []string{"managed_copy_import_retry_pending", "managed_copy_import_retry_uncertain"} {
		if !ManagedCopyWaiting(reason) {
			t.Fatalf("waiting was classified as a failure: %s", reason)
		}
	}
	for _, reason := range []string{"managed_copy_import_stop_pending", "managed_copy_import_stop_uncertain", "managed_copy_upload_retry_pending", "managed_copy_import_retry_timeout", "managed_copy_import_stop_binding"} {
		if ManagedCopyWaiting(reason) {
			t.Fatalf("ordinary failure was classified as a wait: %s", reason)
		}
	}
}
