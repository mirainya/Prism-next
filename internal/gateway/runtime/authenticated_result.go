package runtime

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mirainya/Prism/internal/gateway/delivery"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/pkg/filestorage"
	"github.com/mirainya/Prism/pkg/safeurl"
)

// ManagedCopyAsyncCodec opts a model into requiring a durable storage binding
// before a call or balance reservation is created.
type ManagedCopyAsyncCodec interface {
	RequiresManagedCopy(repository.AsyncDispatch) (bool, error)
}

func resultForManagedSource(blob repository.BlobInput, source delivery.RemoteResult) repository.BlobInput {
	blob.Plaintext = delivery.EncodeManagedSource(source)
	return blob
}

func decodeManagedSource(role string, plaintext []byte) (delivery.RemoteResult, error) {
	source, err := delivery.DecodeManagedSource(role, plaintext)
	if err != nil {
		return delivery.RemoteResult{}, &PermanentDispatchError{Code: "managed_copy_source_unavailable"}
	}
	return source, nil
}

func (d *AsyncDispatcher) bindAuthenticatedResult(ctx context.Context, fixed repository.AsyncDispatch, source *delivery.RemoteResult) error {
	if source.UpstreamAuth == nil {
		return nil
	}
	base, baseErr := url.Parse(fixed.BaseURL)
	target, targetErr := url.Parse(source.URL)
	if baseErr != nil || targetErr != nil || base.Scheme != "https" || target.Scheme != base.Scheme ||
		target.User != nil || target.Fragment != "" ||
		fixed.DeliveryMode != "managed_copy" || !delivery.ValidSource(*source) {
		return &PermanentDispatchError{Code: "invalid_authenticated_result_origin"}
	}
	if !strings.EqualFold(base.Host, target.Host) {
		// A provider may return an explicitly approved public CDN instead of
		// its authenticated content endpoint. Never send upstream credentials
		// there; the permission is scoped to this exact catalog transport.
		if d.service == nil || d.service.Store == nil || fixed.ReleaseID == 0 || fixed.ChannelTransportID == 0 {
			return &PermanentDispatchError{Code: "invalid_authenticated_result_origin"}
		}
		port := target.Port()
		if port == "" {
			port = "443"
		}
		host := strings.ToLower(strings.TrimSuffix(target.Hostname(), "."))
		var allowed int
		if err := d.service.Store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM gw_transport_allowed_hosts WHERE release_id=? AND channel_transport_id=? AND protocol=? AND host_pattern=? AND port=?`, fixed.ReleaseID, fixed.ChannelTransportID, target.Scheme, host, port).Scan(&allowed); err != nil {
			return err
		}
		if allowed == 0 {
			return &PermanentDispatchError{Code: "invalid_authenticated_result_origin"}
		}
		source.UpstreamAuth = nil
		source.OpenAuthenticated = nil
		return nil
	}
	path := target.RequestURI()
	if prefix := strings.TrimRight(base.EscapedPath(), "/"); prefix != "" {
		if !strings.HasPrefix(target.EscapedPath(), prefix+"/") {
			return &PermanentDispatchError{Code: "invalid_authenticated_result_origin"}
		}
		path = strings.TrimPrefix(path, prefix)
	}
	// Keep the original credential binding; no secret is copied into the URL,
	// source metadata, storage request, public response or persisted source.
	prepared := AsyncRequest{Method: http.MethodGet, Path: path,
		CredentialHeader: source.UpstreamAuth.Header, CredentialPrefix: source.UpstreamAuth.Prefix}
	if !validCredentialHeader(prepared.CredentialHeader) || len(prepared.CredentialPrefix) > 256 || strings.ContainsAny(prepared.CredentialPrefix, "\r\n") {
		return &PermanentDispatchError{Code: "invalid_async_auth_mapping"}
	}
	source.OpenAuthenticated = func(ctx context.Context) (*http.Response, error) {
		secret, err := d.openCredential(ctx, fixed.CredentialID, fixed.CredentialSecret, fixed.CredentialBlobID)
		if err != nil {
			return nil, err
		}
		defer clear(secret)
		request, err := d.prepareHTTP(ctx, fixed, prepared, secret)
		if err != nil {
			return nil, err
		}
		defer request.Header.Del(prepared.CredentialHeader)
		client := authenticatedResultClient(prepared.CredentialHeader)
		defer client.CloseIdleConnections()
		return client.Do(request)
	}
	return nil
}

func authenticatedResultClient(credentialHeader string) *http.Client {
	client := safeurl.NewClient(5 * time.Minute)
	validateRedirect := client.CheckRedirect
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) == 0 || request.URL.Scheme != "https" {
			return errors.New("unsafe authenticated result redirect")
		}
		original := via[0].URL
		if !strings.EqualFold(request.URL.Host, original.Host) || request.URL.Scheme != original.Scheme {
			request.Header.Del(credentialHeader)
			request.Header.Del("Authorization")
			request.Header.Del("Cookie")
		}
		return validateRedirect(request, via)
	}
	return client
}

var uploadAuthenticatedResult = func(ctx context.Context, storage repository.TokenFileStorage, data io.Reader, contentType, storagePath, filename string) (filestorage.UploadResult, error) {
	return filestorage.WithBaseURLAndAPIKey(storage.XFSBaseURL, storage.APIKey).UploadReaderAtPathWithFilename(ctx, data, contentType, storagePath, filename)
}

func importAuthenticatedManagedResult(ctx context.Context, storage repository.TokenFileStorage, source delivery.RemoteResult, storagePath, filename string, maxBytes int64) (filestorage.UploadResult, error) {
	if source.UpstreamAuth == nil || source.OpenAuthenticated == nil || maxBytes <= 0 {
		return filestorage.UploadResult{}, &filestorage.StorageError{Code: "rejected"}
	}
	ctx, cancel := managedTransferContext(ctx)
	defer cancel()
	response, err := source.OpenAuthenticated(ctx)
	if err != nil {
		return filestorage.UploadResult{}, err
	}
	if response == nil || response.Body == nil {
		return filestorage.UploadResult{}, &filestorage.StorageError{Code: "response"}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return filestorage.UploadResult{}, filestorage.HTTPError(response.StatusCode)
	}
	if response.ContentLength > maxBytes {
		return filestorage.UploadResult{}, &filestorage.StorageError{Code: "size"}
	}
	reader := bufio.NewReader(response.Body)
	prefix, readErr := reader.Peek(512)
	if readErr != nil && readErr != io.EOF {
		return filestorage.UploadResult{}, readErr
	}
	if len(prefix) == 0 {
		return filestorage.UploadResult{}, &filestorage.StorageError{Code: "empty"}
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	detected := http.DetectContentType(prefix)
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = detected
	}
	if !validManagedResultMIME(source.Role, contentType, detected) {
		return filestorage.UploadResult{}, &filestorage.StorageError{Code: "content_type"}
	}
	limited := &io.LimitedReader{R: reader, N: maxBytes + 1}
	hash := sha256.New()
	uploaded, err := uploadAuthenticatedResult(ctx, storage, io.TeeReader(limited, hash), contentType, storagePath, filename)
	if err != nil {
		return filestorage.UploadResult{}, err
	}
	// A partial or over-limit upload must never become a ready delivery.
	count := maxBytes + 1 - limited.N
	var extra [1]byte
	n, eof := limited.Read(extra[:])
	digest := hex.EncodeToString(hash.Sum(nil))
	storedDigest := uploaded.SHA256()
	invalid := ""
	if count > maxBytes || n > 0 {
		invalid = "size"
	} else if eof != io.EOF || count <= 0 || uploaded.Size != count || (response.ContentLength >= 0 && response.ContentLength != count) {
		invalid = "response"
	} else if storedDigest != "" && storedDigest != digest {
		invalid = "hash"
	}
	if invalid != "" {
		deleteManagedResultBestEffort(ctx, storage, uploaded.StorageLocator())
		return filestorage.UploadResult{}, &filestorage.StorageError{Code: invalid}
	}
	if storedDigest == "" {
		// Some XFS platforms return no SHA-256 metadata. Verify the stored
		// bytes against the source stream before retaining its local digest.
		if err := verifyManagedResult(ctx, storage, uploaded.StorageLocator(), count, digest); err != nil {
			deleteManagedResultBestEffort(ctx, storage, uploaded.StorageLocator())
			return filestorage.UploadResult{}, err
		}
		uploaded.HashInfo = filestorage.UploadHashInfo{"SHA-256": digest}
	}
	return uploaded, nil
}
