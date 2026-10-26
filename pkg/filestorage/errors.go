package filestorage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
)

// StorageError contains only safe diagnostic identifiers. Never put response
// bodies, object URLs or credentials in this error.
type StorageError struct {
	Code       string
	HTTPStatus int
	Retryable  bool
}

func (e *StorageError) Error() string {
	if e.HTTPStatus != 0 {
		return fmt.Sprintf("file storage %s (HTTP %d)", e.Code, e.HTTPStatus)
	}
	return "file storage " + e.Code
}

func HTTPError(status int) *StorageError {
	return &StorageError{Code: "http", HTTPStatus: status,
		Retryable: status == 408 || status == 425 || status == 429 || status >= 500 && status <= 599 && status != 507}
}

// ClassifyError deliberately discards unstructured messages, which can contain
// signed URLs or upstream credentials.
func ClassifyError(err error) *StorageError {
	var storage *StorageError
	if errors.As(err, &storage) {
		return storage
	}
	var network net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &network) && network.Timeout() {
		return &StorageError{Code: "timeout", Retryable: true}
	}
	if errors.As(err, &network) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return &StorageError{Code: "network", Retryable: true}
	}
	return &StorageError{Code: "unknown"}
}

func storageAPIError(code int, message string) *StorageError {
	message = strings.ToLower(message)
	for _, word := range []string{"quota", "capacity", "disk full", "space left", "余额", "容量", "空间不足", "积分不足"} {
		if strings.Contains(message, word) {
			return &StorageError{Code: "capacity"}
		}
	}
	for _, word := range []string{"unauthorized", "forbidden", "permission", "api key", "api-key", "权限", "鉴权", "认证"} {
		if strings.Contains(message, word) {
			return &StorageError{Code: "permission"}
		}
	}
	// An application code is not an HTTP status. Only recognize standard
	// transient codes; never report it as an observed HTTP response status.
	if code == 408 || code == 425 || code == 429 || code >= 500 && code <= 599 && code != 507 {
		return &StorageError{Code: "service", Retryable: true}
	}
	return &StorageError{Code: "rejected"}
}
