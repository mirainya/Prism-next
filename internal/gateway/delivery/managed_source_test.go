package delivery

import (
	"bytes"
	"testing"
)

func TestManagedSourceRetainsLegacyURLFormatAndProtectedMetadata(t *testing.T) {
	url := "https://provider.example/v1/videos/task-1/content"
	plain := EncodeManagedSource(RemoteResult{Role: "video", URL: url})
	if string(plain) != url || !ValidManagedSource(plain) {
		t.Fatal("existing source blob format changed")
	}
	saved := EncodeManagedSource(RemoteResult{Role: "video", URL: url, UpstreamAuth: &UpstreamResultAuth{Header: "Authorization", Prefix: "Bearer "}})
	source, err := DecodeManagedSource("video", saved)
	if err != nil || source.URL != url || source.UpstreamAuth == nil || source.UpstreamAuth.Header != "Authorization" || source.UpstreamAuth.Prefix != "Bearer " || source.OpenAuthenticated != nil {
		t.Fatalf("protected recovery metadata: %+v, %v", source, err)
	}
	if !ValidManagedSource(saved) || bytes.Contains(saved, []byte("api_key")) || bytes.Contains(saved, []byte("secret")) {
		t.Fatal("invalid or secret-bearing recovery envelope")
	}
	for _, invalid := range []string{`{}`, `{"managed_result_version":2,"url":"` + url + `","upstream_auth":{"header":"Authorization","prefix":"Bearer "}}`, `{"managed_result_version":1,"url":"file:///private","upstream_auth":{}}`, "file:///private"} {
		if ValidManagedSource([]byte(invalid)) {
			t.Fatalf("invalid source accepted: %s", invalid)
		}
	}
}
