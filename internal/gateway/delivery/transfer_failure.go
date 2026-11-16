package delivery

import (
	"fmt"
	"strconv"
	"strings"
)

// TransferFailure is persisted as a bounded, safe reason code. It contains no
// upstream text, storage location or credential, and needs no schema change.
type TransferFailure struct {
	Stage      string
	Code       string
	HTTPStatus int
	Retryable  bool
}

func (f TransferFailure) Reason() string {
	policy := "stop"
	if f.Retryable {
		policy = "retry"
	}
	code := f.Code
	if f.HTTPStatus != 0 {
		code = fmt.Sprintf("http_%d", f.HTTPStatus)
	}
	return "managed_copy_" + f.Stage + "_" + policy + "_" + code
}

func ParseTransferFailure(reason string) (TransferFailure, bool) {
	parts := strings.SplitN(strings.TrimPrefix(reason, "managed_copy_"), "_", 3)
	if !strings.HasPrefix(reason, "managed_copy_") || len(parts) != 3 ||
		(parts[0] != "upload" && parts[0] != "import" && parts[0] != "verify") ||
		(parts[1] != "retry" && parts[1] != "stop") {
		return TransferFailure{}, false
	}
	f := TransferFailure{Stage: parts[0], Code: parts[2], Retryable: parts[1] == "retry"}
	if strings.HasPrefix(f.Code, "http_") {
		status, err := strconv.Atoi(strings.TrimPrefix(f.Code, "http_"))
		if err != nil || status < 100 || status > 599 {
			return TransferFailure{}, false
		}
		f.Code, f.HTTPStatus = "http", status
		return f, true
	}
	switch f.Code {
	case "timeout", "network", "service", "permission", "capacity", "rejected", "unknown", "response", "locator", "length", "digest", "hash", "pending", "uncertain", "binding":
		return f, true
	case "configuration", "source_unavailable", "source_rejected", "worker_lost", "transfer_aborted", "outcome_unknown",
		"source_dns", "source_resume", "source_incomplete", "source_slow", "source_changed", "source_interrupted", "transfer_timeout", "destination_rejected":
		return f, f.Stage == "import"
	}
	return TransferFailure{}, false
}

func ManagedCopyWaiting(reason string) bool {
	failure, ok := ParseTransferFailure(reason)
	return ok && failure.Stage == "import" && failure.Retryable && (failure.Code == "pending" || failure.Code == "uncertain")
}

func ManagedCopyFailure(reason string) bool {
	if _, ok := ParseTransferFailure(reason); ok {
		return true
	}
	switch reason {
	case ManagedCopyUploadFailed, ManagedCopyVerificationFailed, ManagedCopySizeExceeded, ManagedCopyEmpty, ManagedCopyContentTypeInvalid:
		return true
	}
	return false
}
