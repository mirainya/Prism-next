package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"testing"

	"github.com/mirainya/Prism/internal/gateway/delivery"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/pkg/filestorage"
)

func TestAuthenticatedResultMissingStorageHashVerifiesStoredObject(t *testing.T) {
	previousUpload, previousVerify, previousDelete := uploadAuthenticatedResult, verifyManagedResult, deleteManagedResult
	t.Cleanup(func() {
		uploadAuthenticatedResult, verifyManagedResult, deleteManagedResult = previousUpload, previousVerify, previousDelete
	})
	digest := sha256.Sum256(testMP4)
	expectedDigest := hex.EncodeToString(digest[:])
	for _, sample := range []struct {
		name      string
		hashInfo  filestorage.UploadHashInfo
		verifyErr error
	}{
		{name: "missing hash metadata"},
		{name: "empty hash metadata", hashInfo: filestorage.UploadHashInfo{}},
		{name: "other hash algorithm", hashInfo: filestorage.UploadHashInfo{"MD5": "unrelated"}},
		{name: "unusable hash metadata", hashInfo: filestorage.UploadHashInfo{"sha256": "invalid"}},
		{name: "stored content mismatch", verifyErr: &filestorage.StorageError{Code: "digest"}},
		{name: "storage temporarily unavailable", verifyErr: filestorage.HTTPError(http.StatusServiceUnavailable)},
	} {
		t.Run(sample.name, func(t *testing.T) {
			verifyCalls, deleteCalls := 0, 0
			uploadAuthenticatedResult = func(_ context.Context, _ repository.TokenFileStorage, reader io.Reader, contentType, _, _ string) (filestorage.UploadResult, error) {
				data, err := io.ReadAll(reader)
				if err != nil {
					return filestorage.UploadResult{}, err
				}
				return filestorage.UploadResult{
					RawURL: "private/result.mp4", Size: int64(len(data)), ContentType: contentType, HashInfo: sample.hashInfo,
				}, nil
			}
			verifyManagedResult = func(_ context.Context, storage repository.TokenFileStorage, locator string, size int64, sha256 string) error {
				verifyCalls++
				if storage.XFSBaseURL != testStorageBaseURL || storage.APIKey != testStorageAPIKey ||
					locator != "private/result.mp4" || size != int64(len(testMP4)) || sha256 != expectedDigest {
					t.Fatal("stored object verification lost its storage binding or source digest")
				}
				return sample.verifyErr
			}
			deleteManagedResult = func(_ context.Context, _ repository.TokenFileStorage, locator string) error {
				deleteCalls++
				if locator != "private/result.mp4" {
					t.Fatal("unexpected cleanup locator")
				}
				return nil
			}
			source := delivery.RemoteResult{
				Role: "video", URL: "https://provider.example/v1/videos/task-1/content",
				UpstreamAuth: &delivery.UpstreamResultAuth{Header: "Authorization", Prefix: "Bearer "},
				OpenAuthenticated: func(context.Context) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK, ContentLength: int64(len(testMP4)),
						Header: http.Header{"Content-Type": []string{"video/mp4"}},
						Body:   io.NopCloser(bytes.NewReader(testMP4)),
					}, nil
				},
			}
			storage := repository.TokenFileStorage{XFSBaseURL: testStorageBaseURL, APIKey: testStorageAPIKey}
			result, err := importAuthenticatedManagedResult(context.Background(), storage, source, "gateway-results/2/0/", "result.mp4", 1024)
			if verifyCalls != 1 {
				t.Fatalf("stored object verification calls = %d, want 1", verifyCalls)
			}
			if sample.verifyErr != nil {
				if err != sample.verifyErr || deleteCalls != 1 || result.StorageLocator() != "" {
					t.Fatalf("verification failure was not preserved: result=%+v err=%v cleanup=%d", result, err, deleteCalls)
				}
				return
			}
			if err != nil || deleteCalls != 0 || result.SHA256() != expectedDigest || result.StorageLocator() != "private/result.mp4" {
				t.Fatalf("verified upload not retained: result=%+v err=%v cleanup=%d", result, err, deleteCalls)
			}
		})
	}
}
