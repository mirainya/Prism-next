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
	parts := strings.Split(strings.TrimPrefix(reason, "managed_copy_"), "_")
	if !strings.HasPrefix(reason, "managed_copy_") || len(parts) < 3 || len(parts) > 4 ||
		(parts[0] != "upload" && parts[0] != "import" && parts[0] != "verify") ||
		(parts[1] != "retry" && parts[1] != "stop") {
		return TransferFailure{}, false
	}
	f := TransferFailure{Stage: parts[0], Code: parts[2], Retryable: parts[1] == "retry"}
	if f.Code == "http" && len(parts) == 4 {
		status, err := strconv.Atoi(parts[3])
		if err != nil || status < 100 || status > 599 {
			return TransferFailure{}, false
		}
		f.HTTPStatus = status
		return f, true
	}
	if len(parts) != 3 {
		return TransferFailure{}, false
	}
	switch f.Code {
	case "timeout", "network", "service", "permission", "capacity", "rejected", "unknown", "response", "locator", "length", "digest", "hash":
		return f, true
	}
	return TransferFailure{}, false
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
