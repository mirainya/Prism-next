package filestorage

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

func TestStorageErrorsClassifyRetryAndDiscardSensitiveMessages(t *testing.T) {
	for _, status := range []int{401, 403, 413, 507, 429, 502, 503, 504} {
		err := HTTPError(status)
		want := status == 429 || status == 502 || status == 503 || status == 504
		if err.HTTPStatus != status || err.Retryable != want {
			t.Fatalf("status=%d error=%+v", status, err)
		}
	}
	for _, item := range []struct{ message, code string }{
		{"quota exceeded https://private.example?key=secret", "capacity"},
		{"permission denied Bearer private-secret", "permission"},
		{"private upstream failure sk-private-secret", "service"},
	} {
		err := storageAPIError(500, item.message)
		if err.Code != item.code || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private") || err.HTTPStatus != 0 {
			t.Fatalf("unsafe storage error: %+v", err)
		}
	}
	timeout := ClassifyError(fmt.Errorf("upload: %w", &url.Error{Op: "Post", URL: "https://private?key=secret", Err: context.DeadlineExceeded}))
	if !timeout.Retryable || timeout.Code != "timeout" || strings.Contains(timeout.Error(), "secret") {
		t.Fatal("timeout lost its classification or leaked URL")
	}
}
