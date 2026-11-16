package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/mirainya/Prism/internal/gateway/billing"
	"github.com/mirainya/Prism/internal/gateway/delivery"
	"github.com/mirainya/Prism/internal/gateway/repository"
	"github.com/mirainya/Prism/pkg/filestorage"
)

type authenticatedResultTransport func(*http.Request) (*http.Response, error)

func (f authenticatedResultTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestAuthenticatedResultBindingRejectsUntrustedOrigins(t *testing.T) {
	d := &AsyncDispatcher{}
	fixed := repository.AsyncDispatch{BaseURL: "https://provider.example", DeliveryMode: "managed_copy"}
	for _, target := range []string{"https://other.example/content", "https://provider.example.evil/content", "http://provider.example/content", "https://provider.example:444/content", "https://user:password@provider.example/content"} {
		source := delivery.RemoteResult{Role: "video", URL: target, UpstreamAuth: &delivery.UpstreamResultAuth{Header: "Authorization", Prefix: "Bearer "}}
		if err := d.bindAuthenticatedResult(context.Background(), fixed, &source); err == nil || source.OpenAuthenticated != nil {
			t.Fatalf("credential attached to an untrusted origin: %s", target)
		}
	}
	for _, mode := range []string{"managed_copy", "reference"} {
		fixed.DeliveryMode = mode
		source := delivery.RemoteResult{Role: "video", URL: fixed.BaseURL + "/v1/videos/task-1/content", UpstreamAuth: &delivery.UpstreamResultAuth{Header: "Authorization", Prefix: "Bearer "}}
		err := d.bindAuthenticatedResult(context.Background(), fixed, &source)
		if (err == nil) != (mode == "managed_copy") {
			t.Fatalf("delivery mode %s: %v", mode, err)
		}
	}
	plain := delivery.RemoteResult{Role: "video", URL: "https://legacy-cdn.example/result.mp4"}
	if err := d.bindAuthenticatedResult(context.Background(), repository.AsyncDispatch{}, &plain); err != nil || plain.OpenAuthenticated != nil {
		t.Fatal("legacy public result acquired an authentication requirement")
	}
}

func TestAuthenticatedResultPublicCDNRequiresExactTransportPermission(t *testing.T) {
	for _, sample := range []struct {
		name, target, host, port string
		allowed                  int
		lookupError              bool
	}{
		{"approved CDN", "https://cdn.provider.example/result.mp4", "cdn.provider.example", "443", 1, false},
		{"approved port", "https://cdn.provider.example:8443/result.mp4", "cdn.provider.example", "8443", 1, false},
		{"unapproved CDN", "https://other.example/result.mp4", "other.example", "443", 0, false},
		{"policy unavailable", "https://cdn.provider.example/result.mp4", "cdn.provider.example", "443", 0, true},
	} {
		t.Run(sample.name, func(t *testing.T) {
			d, mock := testDispatcher(t, nil)
			fixed := repository.AsyncDispatch{BaseURL: "https://provider.example", DeliveryMode: "managed_copy", ReleaseID: 7, ChannelTransportID: 9}
			query := mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM gw_transport_allowed_hosts WHERE release_id=").
				WithArgs(fixed.ReleaseID, fixed.ChannelTransportID, "https", sample.host, sample.port)
			if sample.lookupError {
				query.WillReturnError(errors.New("policy lookup failed"))
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(sample.allowed))
			}
			source := delivery.RemoteResult{Role: "video", URL: sample.target, UpstreamAuth: &delivery.UpstreamResultAuth{Header: "Authorization", Prefix: "Bearer "}}
			err := d.bindAuthenticatedResult(context.Background(), fixed, &source)
			wantOK := sample.allowed > 0 && !sample.lookupError
			if (err == nil) != wantOK {
				t.Fatalf("binding error=%v, want success=%v", err, wantOK)
			}
			if source.OpenAuthenticated != nil || wantOK && source.UpstreamAuth != nil || source.URL != sample.target {
				t.Fatal("public CDN must retain its URL without receiving upstream credentials")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAuthenticatedResultRedirectNeverForwardsCredentialsToAnotherOrigin(t *testing.T) {
	for _, header := range []string{"Authorization", "X-Provider-Key"} {
		t.Run(header, func(t *testing.T) {
			client := authenticatedResultClient(header)
			defer client.CloseIdleConnections()
			calls := 0
			client.Transport = authenticatedResultTransport(func(request *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					if request.Header.Get(header) != "Bearer test-secret" {
						t.Fatal("initial download lacks its credential")
					}
					return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://1.1.1.1/signed/result.mp4"}}, Body: io.NopCloser(strings.NewReader("")), Request: request}, nil
				}
				if request.URL.Host != "1.1.1.1" || request.Header.Get(header) != "" || request.Header.Get("Authorization") != "" || request.Header.Get("Cookie") != "" {
					t.Fatal("credential or cookie forwarded to a result CDN")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(testMP4)), Request: request}, nil
			})
			request, _ := http.NewRequest(http.MethodGet, "https://8.8.8.8/v1/videos/task-1/content", nil)
			request.Header.Set(header, "Bearer test-secret")
			request.Header.Set("Cookie", "session=private")
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if calls != 2 {
				t.Fatalf("download requests = %d", calls)
			}
		})
	}
}

func TestAuthenticatedResultStreamsIntoBoundStorageAndChecksDigest(t *testing.T) {
	previousUpload, previousDelete := uploadAuthenticatedResult, deleteManagedResult
	t.Cleanup(func() { uploadAuthenticatedResult, deleteManagedResult = previousUpload, previousDelete })
	for _, sample := range []struct {
		name, contentType string
		status            int
		max               int64
		badHash           bool
		wantOK            bool
	}{
		{"success", "video/mp4", 200, 1024, false, true},
		{"auth rejected", "application/json", 401, 1024, false, false},
		{"upstream transient", "application/json", 503, 1024, false, false},
		{"invalid type", "application/json", 200, 1024, false, false},
		{"over limit", "video/mp4", 200, 5, false, false},
		{"hash mismatch", "video/mp4", 200, 1024, true, false},
	} {
		t.Run(sample.name, func(t *testing.T) {
			uploadCalls, deleteCalls := 0, 0
			uploadAuthenticatedResult = func(_ context.Context, storage repository.TokenFileStorage, reader io.Reader, contentType, path, filename string) (filestorage.UploadResult, error) {
				uploadCalls++
				if storage.APIKey != testStorageAPIKey || storage.XFSBaseURL != testStorageBaseURL || contentType != "video/mp4" || path != "gateway-results/2/0/" || filename != "result.mp4" {
					t.Fatal("bound storage or transfer metadata changed")
				}
				data, err := io.ReadAll(reader)
				if err != nil {
					return filestorage.UploadResult{}, err
				}
				digest := sha256.Sum256(data)
				if sample.badHash {
					digest[0] ^= 1
				}
				return filestorage.UploadResult{RawURL: "private/result.mp4", Size: int64(len(data)), ContentType: contentType, HashInfo: filestorage.UploadHashInfo{"SHA-256": hex.EncodeToString(digest[:])}}, nil
			}
			deleteManagedResult = func(_ context.Context, _ repository.TokenFileStorage, locator string) error {
				deleteCalls++
				if locator != "private/result.mp4" {
					t.Fatal("unexpected cleanup locator")
				}
				return nil
			}
			source := delivery.RemoteResult{Role: "video", URL: "https://provider.example/v1/videos/task-1/content", UpstreamAuth: &delivery.UpstreamResultAuth{Header: "Authorization", Prefix: "Bearer "},
				OpenAuthenticated: func(context.Context) (*http.Response, error) {
					return &http.Response{StatusCode: sample.status, ContentLength: int64(len(testMP4)), Header: http.Header{"Content-Type": []string{sample.contentType}}, Body: io.NopCloser(bytes.NewReader(testMP4))}, nil
				},
			}
			result, err := importAuthenticatedManagedResult(context.Background(), repository.TokenFileStorage{XFSBaseURL: testStorageBaseURL, APIKey: testStorageAPIKey}, source, "gateway-results/2/0/", "result.mp4", sample.max)
			if (err == nil) != sample.wantOK {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if sample.wantOK && (uploadCalls != 1 || result.StorageLocator() != "private/result.mp4" || deleteCalls != 0) {
				t.Fatal("successful stream not retained")
			}
			if sample.badHash && deleteCalls != 1 {
				t.Fatal("corrupt upload not removed")
			}
			if sample.status == 503 && !filestorage.ClassifyError(err).Retryable {
				t.Fatal("transient content download cannot be retried")
			}
		})
	}
}

func TestProtectedVideoRejectsMissingStorageBeforeCallCreation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store, _ := repository.New(db)
	service := &Service{Store: store}
	in := SubmitInput{Call: repository.CreateCallInput{UserID: 11, TokenID: 13}, ResourceKind: "video_task", RequireManagedCopy: true}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT COALESCE\\(xfs_base_url,''\\),COALESCE\\(xfs_api_key,''\\) FROM tokens").WithArgs(uint64(13), uint64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"xfs_base_url", "xfs_api_key"}).AddRow("", ""))
	mock.ExpectRollback()
	err = store.WithTx(context.Background(), func(tx *sql.Tx) error { return service.configureMediaDelivery(context.Background(), tx, &in) })
	if !errors.Is(err, repository.ErrInvalidInput) {
		t.Fatalf("missing storage must fail before a call/reservation exists: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProviderBilledSecondsEstimateUsesUpperBound(t *testing.T) {
	schedule := billing.RateSchedule{
		Currency:   billing.Currency{Code: "CREDIT", Version: 1, FractionDigits: 8, RoundingMode: "half_even", MaxAmount: "1000000000"},
		Components: []billing.RateComponent{{Code: "billable_seconds", Unit: "second", Source: billing.QuantityBilledSeconds, Event: billing.ChargeSucceeded, UnitPrice: "2", QuantityStep: "0", MaxQuantity: "100"}},
	}
	charge, upperBound, err := EstimateVideoCharge(schedule, []byte(`{"duration":4}`))
	if err != nil || !upperBound || charge.Amount.String() != "200" {
		t.Fatalf("estimate must not substitute requested seconds for provider billing: %+v, %t, %v", charge, upperBound, err)
	}
}
