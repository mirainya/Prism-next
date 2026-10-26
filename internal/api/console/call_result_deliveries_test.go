package console

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/internal/service"
	"gorm.io/gorm"
)

const testResultLocator = "https://files.example.test/private-results/original.png?Signature=stored-secret"

func setupCallDeliveryTest(t *testing.T) (*gorm.DB, uint64) {
	t.Helper()
	db := setupAPICallConsoleTestDB(t)
	callID := createAPICallConsoleTestCall(t, db, model.APICall{
		ID: "call-files", UserID: 10, TokenID: 100, Model: "model-a", Status: model.APICallStatusCompleted,
	})
	for _, item := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO gw_api_call_attempts(id,call_id,attempt_no,catalog_release_id,product_transport_id,credential_id,state) VALUES (1,?,1,1,1,31,'completed')`, []any{callID}},
		{`INSERT INTO gw_media_assets(id,user_id,token_id,purpose,state,content_type,content_length,storage_locator,retention_until) VALUES (61,10,100,'result','active','image/png',123456,?,?)`, []any{testResultLocator, time.Now().Add(time.Hour)}},
		{`INSERT INTO gw_media_asset_refs(id,media_asset_id,user_id,token_id,result_delivery_id) VALUES (71,61,10,100,41)`, nil},
		{`INSERT INTO gw_result_deliveries(id,call_id,attempt_id,result_ordinal,user_id,token_id,delivery_mode,state,media_asset_ref_id,updated_at) VALUES (41,?,1,0,10,100,'managed_copy','ready',71,?)`, []any{callID, time.Now()}},
		{`INSERT INTO gw_result_deliveries(id,call_id,attempt_id,result_ordinal,user_id,token_id,delivery_mode,state,reason_code,updated_at) VALUES (42,?,1,1,10,100,'managed_copy','delivery_failed','managed_copy_size_exceeded',?)`, []any{callID, time.Now()}},
	} {
		if err := db.Exec(item.query, item.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db, callID
}

func TestCallDetailIncludesResultDeliveryStateWithoutCredentials(t *testing.T) {
	setupCallDeliveryTest(t)
	for _, role := range []string{"user", "admin"} {
		response := requestAPICallConsole(t, 10, role, http.MethodGet, "/api/calls/call-files")
		if response.Code != http.StatusOK {
			t.Fatalf("detail: %d %s", response.Code, response.Body.String())
		}
		var envelope apiCallEnvelope
		decodeAPICallConsoleResponse(t, response, &envelope)
		var detail service.APICallDetail
		if err := json.Unmarshal(envelope.Data, &detail); err != nil {
			t.Fatal(err)
		}
		if len(detail.Deliveries) != 2 {
			t.Fatalf("missing file records: %+v", detail.Deliveries)
		}
		ready, failed := detail.Deliveries[0], detail.Deliveries[1]
		if !ready.Available || ready.Filename != "original.png" || ready.ContentLength != 123456 {
			t.Fatalf("ready result: %+v", ready)
		}
		if failed.Available || failed.State != "delivery_failed" || failed.ReasonMessage != "生成文件超过转存大小限制" {
			t.Fatalf("failed result incorrectly reported: %+v", failed)
		}
		if strings.Contains(response.Body.String(), "stored-secret") {
			t.Fatal("metadata exposed a storage signature")
		}
		if role == "admin" && (ready.StoragePath != "/private-results/original.png" || failed.ReasonCode != "managed_copy_size_exceeded") {
			t.Fatal("admin missing path or failure code")
		}
		if role == "user" && (ready.StoragePath != "" || ready.AttemptID != 0 || failed.ReasonCode != "") {
			t.Fatal("owner received internal storage metadata")
		}
	}
}

func TestCallDeliveryURLUsesSnapshotAndEnforcesOwnership(t *testing.T) {
	db, callID := setupCallDeliveryTest(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/api/v1/file/presigned-url" || r.Header.Get("X-Api-Key") != "call-snapshot-key" || r.URL.Query().Get("url") != testResultLocator || r.URL.Query().Get("expiration") != "900" {
			t.Error("incorrect storage snapshot or signing request")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"code":200,"data":"https://files.example.test/original.png?signature=temporary"}`)
	}))
	defer server.Close()
	if err := db.Exec(`UPDATE gw_api_calls SET xfs_base_url=?,xfs_api_key='call-snapshot-key' WHERE id=?`, server.URL, callID).Error; err != nil {
		t.Fatal(err)
	}
	createAPICallConsoleTestCall(t, db, model.APICall{ID: "different-call", UserID: 10, TokenID: 100, Model: "model-a"})
	for _, test := range []struct {
		actor uint
		role  string
		path  string
		want  int
	}{
		{10, "user", "/api/calls/call-files/deliveries/41/url", 200},
		{1, "admin", "/api/calls/call-files/deliveries/41/url", 200},
		{20, "user", "/api/calls/call-files/deliveries/41/url", 404},
		{0, "user", "/api/calls/call-files/deliveries/41/url", 401},
		{10, "user", "/api/calls/different-call/deliveries/41/url", 404},
		{10, "user", "/api/calls/call-files/deliveries/999/url", 404},
		{10, "user", "/api/calls/call-files/deliveries/invalid/url", 400},
		{10, "user", "/api/calls/call-files/deliveries/42/url", 409},
	} {
		response := requestAPICallConsole(t, test.actor, test.role, http.MethodGet, test.path)
		if response.Code != test.want {
			t.Fatalf("actor=%d path=%s status=%d want=%d body=%s", test.actor, test.path, response.Code, test.want, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "call-snapshot-key") {
			t.Fatal("storage API key exposed")
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("signed URL response must not be cached")
		}
		if test.want == 200 {
			var envelope apiCallEnvelope
			decodeAPICallConsoleResponse(t, response, &envelope)
			var link service.APICallDeliveryURL
			if err := json.Unmarshal(envelope.Data, &link); err != nil || !strings.HasPrefix(link.URL, "https://files.example.test/") || time.Until(link.ExpiresAt) > 15*time.Minute || time.Until(link.ExpiresAt) < 14*time.Minute {
				t.Fatalf("invalid temporary link: %+v err=%v", link, err)
			}
		}
	}
	if requests.Load() != 2 {
		t.Fatalf("unauthorized or failed result attempted signing: %d requests", requests.Load())
	}
}

func TestCallDeliveryURLRejectsExpiredOrForeignFiles(t *testing.T) {
	db, _ := setupCallDeliveryTest(t)
	for _, update := range []string{
		`UPDATE gw_media_assets SET retention_until='2000-01-01 00:00:00' WHERE id=61`,
		`UPDATE gw_media_assets SET retention_until=NULL,user_id=20 WHERE id=61`,
		`UPDATE gw_media_assets SET user_id=10,state='deleted' WHERE id=61`,
	} {
		if err := db.Exec(update).Error; err != nil {
			t.Fatal(err)
		}
		response := requestAPICallConsole(t, 10, "user", http.MethodGet, "/api/calls/call-files/deliveries/41/url")
		if response.Code != http.StatusConflict {
			t.Fatalf("unavailable file: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestCallDeliveryShowsStructuredDiagnosticsAndRecoveryProgress(t *testing.T) {
	db, _ := setupCallDeliveryTest(t)
	for _, statement := range []string{
		`UPDATE gw_result_deliveries SET source_kind='inline_response',reason_code='managed_copy_upload_retry_http_503' WHERE id=42`,
		`INSERT INTO gw_channel_request_logs(id,attempt_id,action,status,http_status,response_bytes_complete,response_payload_blob_id) VALUES (99,1,'submit','response_recorded',200,1,88)`,
		`INSERT INTO gw_async_outbox(id,result_delivery_id,state_version,action,status,attempt_count,last_error_code) VALUES (81,42,1,'reconcile_delivery','pending',2,'managed_copy_upload_retry_http_503')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	response := requestAPICallConsole(t, 10, "user", http.MethodGet, "/api/calls/call-files")
	if response.Code != 200 {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var envelope apiCallEnvelope
	decodeAPICallConsoleResponse(t, response, &envelope)
	var detail service.APICallDetail
	if err := json.Unmarshal(envelope.Data, &detail); err != nil {
		t.Fatal(err)
	}
	failure := detail.Deliveries[1]
	if failure.FailureStage != "upload" || failure.FailureHTTPStatus != 503 || !failure.FailureRetryable || failure.RetryStatus != "pending" || failure.RetryAttempts != 2 || failure.CanRetry {
		t.Fatalf("missing diagnostic or duplicate retry allowed: %+v", failure)
	}
	if err := db.Exec(`UPDATE gw_async_outbox SET status='dead_letter' WHERE id=81`).Error; err != nil {
		t.Fatal(err)
	}
	response = requestAPICallConsole(t, 10, "user", http.MethodGet, "/api/calls/call-files")
	decodeAPICallConsoleResponse(t, response, &envelope)
	if err := json.Unmarshal(envelope.Data, &detail); err != nil || !detail.Deliveries[1].CanRetry {
		t.Fatal("stopped recovery missing manual retry")
	}
}

func TestCallDeliveryRetryEnforcesOwnershipAndInput(t *testing.T) {
	setupCallDeliveryTest(t)
	for _, test := range []struct {
		actor uint
		id    string
		want  int
	}{
		{0, "42", 401}, {20, "42", 404}, {10, "999", 404}, {10, "invalid", 400},
	} {
		response := requestAPICallConsole(t, test.actor, "user", http.MethodPost, "/api/calls/call-files/deliveries/"+test.id+"/retry")
		if response.Code != test.want {
			t.Fatalf("actor=%d status=%d want=%d", test.actor, response.Code, test.want)
		}
	}
}
