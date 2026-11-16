package generic

import (
	"strings"
	"testing"
)

func TestResolveResultURL(t *testing.T) {
	for _, tt := range []struct {
		name, raw, base, want string
		fail                  bool
	}{
		{"disabled", "/api/content?sig=a%2Fb", "", "/api/content?sig=a%2Fb", false},
		{"relative", "/api/content?sig=a%2Fb&x=1", "https://provider.example", "https://provider.example/api/content?sig=a%2Fb&x=1", false},
		{"absolute", "https://cdn.example/video", "https://provider.example", "https://cdn.example/video", false},
		{"other host", "//evil.example/video", "https://provider.example", "", true},
		{"invalid base", "/video", "https://user@example.com", "", true},
		{"path base", "/video", "https://example.com/path", "", true},
		{"relative path", "video", "https://example.com", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveResultURL(tt.raw, tt.base)
			if (err != nil) != tt.fail || got != tt.want {
				t.Fatalf("got %q, error %v", got, err)
			}
		})
	}
}

func TestParseResponseRelativeResultURLs(t *testing.T) {
	config := testAdapterConfig()
	config.Response.Root = ""
	config.Response.SuccessCodePath = ""
	config.Response.VideoURLPaths = []string{"video"}
	config.Response.ThumbnailURLPaths = []string{"thumbnail"}
	config.Response.RelativeResultURLBase = "https://provider.example"
	adapter := &Adapter{config: config}
	parsed, err := adapter.parseResponse([]byte(`{"video":"/api/content?sig=a%2Fb","thumbnail":"/thumb?x=1"}`), "completed")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Result.VideoURL != "https://provider.example/api/content?sig=a%2Fb" || parsed.Result.ThumbnailURL != "https://provider.example/thumb?x=1" {
		t.Fatalf("unexpected result: %#v", parsed.Result)
	}
	config.Response.RelativeResultURLBase = ""
	adapter.config = config
	parsed, err = adapter.parseResponse([]byte(`{"video":"/api/content?sig=a%2Fb"}`), "completed")
	if err != nil || parsed.Result.VideoURL != "/api/content?sig=a%2Fb" {
		t.Fatalf("unconfigured product behavior changed: %#v, %v", parsed, err)
	}
}

func TestAppendResultURLQuery(t *testing.T) {
	for _, tt := range []struct {
		name, raw, want string
		defaults        map[string]string
	}{
		{"disabled", "https://example.com/v?sig=a%2fb+z", "https://example.com/v?sig=a%2fb+z", nil},
		{"signed", "https://example.com/v?sig=a%2fb+z&x=1#part", "https://example.com/v?sig=a%2fb+z&x=1&relay=1#part", map[string]string{"relay": "1"}},
		{"existing", "https://example.com/v?relay=0&sig=x", "https://example.com/v?relay=0&sig=x", map[string]string{"relay": "1"}},
		{"empty", "", "", map[string]string{"relay": "1"}},
		{"relative", "/content", "/content?relay=1", map[string]string{"relay": "1"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := appendResultURLQuery(tt.raw, tt.defaults)
			if err != nil || got != tt.want {
				t.Fatalf("got %q, error %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestParseResponseResultURLQuery(t *testing.T) {
	config := testAdapterConfig()
	config.Response.Root = ""
	config.Response.SuccessCodePath = ""
	config.Response.VideoURLPaths = []string{"video"}
	config.Response.ThumbnailURLPaths = []string{"thumbnail"}
	config.Response.RelativeResultURLBase = "https://provider.example"
	config.Response.ResultURLQuery = map[string]string{"relay": "1"}
	adapter := &Adapter{config: config}
	parsed, err := adapter.parseResponse([]byte(`{"video":"/content?sig=a%2fb","thumbnail":"/thumb"}`), "completed")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Result.VideoURL != "https://provider.example/content?sig=a%2fb&relay=1" || parsed.Result.ThumbnailURL != "https://provider.example/thumb" {
		t.Fatalf("unexpected result: %#v", parsed.Result)
	}
}

func TestValidateResponseResultURLQuery(t *testing.T) {
	for _, tt := range []struct {
		name     string
		defaults map[string]string
		fail     bool
	}{
		{"disabled", nil, false},
		{"empty", map[string]string{}, false},
		{"valid", map[string]string{"relay": "1"}, false},
		{"empty value", map[string]string{"download": ""}, false},
		{"empty key", map[string]string{"": "1"}, true},
		{"blank key", map[string]string{" ": "1"}, true},
		{"whitespace key", map[string]string{"\t\r\n": "1"}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := testAdapterConfig().Response
			config.ResultURLQuery = tt.defaults
			err := validateResponseConfig(config)
			if (err != nil) != tt.fail {
				t.Fatalf("validation error = %v, want failure %v", err, tt.fail)
			}
			if err != nil && !strings.Contains(err.Error(), "response.result_url_query") {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}
