package generic

import "testing"

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
