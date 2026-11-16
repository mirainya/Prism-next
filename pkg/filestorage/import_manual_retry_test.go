package filestorage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestManualImportRetrySendsOnlyActionIdentityAndNeverFallsBack(t *testing.T) {
	for _, available := range []bool{true, false} {
		t.Run(map[bool]string{true: "supported", false: "older_server"}[available], func(t *testing.T) {
			reads := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reads++
				if r.Method != http.MethodPost || r.URL.Path != "/api/v1/url-imports/"+managedImportTestKey+"/manual-retry" || r.Header.Get("X-Api-Key") != "bound-storage-key" {
					t.Error("manual retry used a different endpoint or binding")
				}
				var body map[string]uint64
				if json.NewDecoder(r.Body).Decode(&body) != nil || len(body) != 1 || body["retryRequestId"] != 51 {
					t.Error("manual retry did not preserve its action identity")
				}
				if !available {
					w.WriteHeader(404)
					return
				}
				writeManagedImportResponse(w, URLImportTask{TaskKey: managedImportTestKey, State: "queued", Attempts: 5})
			}))
			defer server.Close()
			task, err := WithBaseURLAndAPIKey(server.URL, "bound-storage-key").RetryURLImportTaskOnce(context.Background(), managedImportTestKey, 51)
			if available && (err != nil || task.State != "queued") {
				t.Fatalf("manual retry failed: %v", err)
			}
			if !available {
				requireManagedImportError(t, err, "configuration")
			}
			if reads != 1 {
				t.Fatal("manual retry fell back to another upload endpoint")
			}
		})
	}
}
