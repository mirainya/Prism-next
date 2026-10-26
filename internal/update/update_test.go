package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCheckUsesMirrorWithoutContactingGitHub(t *testing.T) {
	asset := []byte("release archive")
	manifest := testManifest(asset, "2.0.13")
	mirrorHits, githubHits := 0, 0
	mux := http.NewServeMux()
	mux.HandleFunc("/mirainya/Prism-next/releases/latest/download/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		mirrorHits++
		_, _ = w.Write(manifest)
	})
	mux.HandleFunc("/mirainya/Prism-next/releases/download/v2.0.13/prism-v2.0.13-linux-amd64.tar.gz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(asset)
	})
	mirror := httptest.NewServer(mux)
	defer mirror.Close()
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		githubHits++
		http.NotFound(w, r)
	}))
	defer github.Close()

	client, err := newTestUpdateClient(Options{
		Repository: "mirainya/Prism-next", CurrentVersion: "2.0.12",
		Sources:  []Source{{Name: "mirror", BaseURL: mirror.URL}, {Name: "github", BaseURL: github.URL}},
		Platform: "linux", Arch: "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.LatestVersion != "2.0.13" || !result.UpdateAvailable || result.Source != "mirror" {
		t.Fatalf("unexpected check result: %+v", result)
	}
	if mirrorHits != 1 || githubHits != 0 {
		t.Fatalf("source order mirror=%d github=%d", mirrorHits, githubHits)
	}
}

func TestCheckFallsBackAfterInvalidMirrorManifest(t *testing.T) {
	manifest := testManifest([]byte("release archive"), "2.0.13")
	mux := http.NewServeMux()
	mux.HandleFunc("/mirainya/Prism-next/releases/latest/download/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	})
	mirror := httptest.NewServer(mux)
	defer mirror.Close()
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(manifest)
	}))
	defer github.Close()
	client, err := newTestUpdateClient(Options{
		Repository: "mirainya/Prism-next", CurrentVersion: "2.0.13",
		Sources:  []Source{{Name: "mirror", BaseURL: mirror.URL}, {Name: "github", BaseURL: github.URL}},
		Platform: "linux", Arch: "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "github" || result.UpdateAvailable {
		t.Fatalf("unexpected fallback result: %+v", result)
	}
}

func TestCheckHonorsSourcePriorityWithoutComparingFallbackVersions(t *testing.T) {
	oldManifest := testManifest([]byte("old"), "2.0.13")
	newManifest := testManifest([]byte("new"), "2.0.14")
	githubHits := 0
	oldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cache-Control") != "no-cache" {
			t.Error("manifest request must bypass intermediary caches")
		}
		_, _ = w.Write(oldManifest)
	}))
	defer oldServer.Close()
	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		githubHits++
		_, _ = w.Write(newManifest)
	}))
	defer newServer.Close()
	client, err := newTestUpdateClient(Options{
		Repository: "mirainya/Prism-next", CurrentVersion: "2.0.12",
		Sources:  []Source{{Name: "mirror", BaseURL: oldServer.URL}, {Name: "github", BaseURL: newServer.URL}},
		Platform: "linux", Arch: "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.LatestVersion != "2.0.13" || result.Source != "mirror" || githubHits != 0 {
		t.Fatalf("valid preferred source triggered a fallback comparison: %+v, github requests=%d", result, githubHits)
	}
}

func TestCheckIgnoresPrereleaseForStableChannel(t *testing.T) {
	preview := strings.Replace(string(testManifest([]byte("preview"), "2.0.14")), `"channel":"stable"`, `"channel":"prerelease"`, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mirainya/Prism-next/releases/latest/download/manifest.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(preview))
	}))
	defer server.Close()
	client, err := newTestUpdateClient(Options{Repository: "mirainya/Prism-next", CurrentVersion: "2.0.12", Sources: []Source{{BaseURL: server.URL}}, Platform: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Check(context.Background()); err == nil || !errors.Is(err, ErrSourcesUnavailable) {
		t.Fatalf("stable channel accepted prerelease: %v", err)
	}
}

func TestDownloadVerifiesSHA256AndIsIdempotent(t *testing.T) {
	asset := []byte("verified release archive")
	manifest := testManifest(asset, "2.0.13")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mirainya/Prism-next/releases/latest/download/manifest.json":
			_, _ = w.Write(manifest)
		case "/mirainya/Prism-next/releases/download/v2.0.13/prism-v2.0.13-linux-amd64.tar.gz":
			_, _ = w.Write(asset)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := newTestUpdateClient(Options{
		Repository: "mirainya/Prism-next", CurrentVersion: "2.0.12",
		Sources: []Source{{Name: "test", BaseURL: server.URL}}, Platform: "linux", Arch: "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	result, err := client.Download(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if result.Size != int64(len(asset)) || result.SHA256 != sha256Hex(asset) {
		t.Fatalf("unexpected download result: %+v", result)
	}
	if got, err := os.ReadFile(result.Path); err != nil || string(got) != string(asset) {
		t.Fatalf("downloaded file mismatch: err=%v content=%q", err, got)
	}
	second, err := client.Download(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if second.Path != result.Path {
		t.Fatalf("idempotent path changed: %q vs %q", second.Path, result.Path)
	}
}

func TestDownloadFallsBackToGitHubWhenMirrorAssetFails(t *testing.T) {
	asset := []byte("fallback release archive")
	manifest := testManifest(asset, "2.0.13")
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "manifest.json") {
			_, _ = w.Write(manifest)
			return
		}
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer mirror.Close()
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "manifest.json") {
			_, _ = w.Write(manifest)
			return
		}
		_, _ = w.Write(asset)
	}))
	defer github.Close()
	client, err := newTestUpdateClient(Options{
		Repository: "mirainya/Prism-next", CurrentVersion: "2.0.12",
		Sources:  []Source{{Name: "mirror", BaseURL: mirror.URL}, {Name: "github", BaseURL: github.URL}},
		Platform: "linux", Arch: "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Download(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "github" || result.SHA256 != sha256Hex(asset) {
		t.Fatalf("unexpected fallback result: %+v", result)
	}
}

func TestDownloadUsesValidMirrorWithoutContactingGitHub(t *testing.T) {
	asset := []byte("mirror release archive")
	manifest := testManifest(asset, "2.0.13")
	githubHits := 0
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "manifest.json") {
			_, _ = w.Write(manifest)
			return
		}
		_, _ = w.Write(asset)
	}))
	defer mirror.Close()
	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		githubHits++
		http.NotFound(w, r)
	}))
	defer github.Close()
	client, err := newTestUpdateClient(Options{
		Repository: "mirainya/Prism-next", CurrentVersion: "2.0.12",
		Sources:  []Source{{Name: "mirror", BaseURL: mirror.URL}, {Name: "github", BaseURL: github.URL}},
		Platform: "linux", Arch: "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Download(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "mirror" || result.SHA256 != sha256Hex(asset) || githubHits != 0 {
		t.Fatalf("valid mirror download contacted GitHub: %+v, github requests=%d", result, githubHits)
	}
}

func TestDownloadRejectsHashMismatchWithoutLeavingPartFile(t *testing.T) {
	asset := []byte("differenx")
	manifest := testManifest([]byte("different"), "2.0.13")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "manifest.json") {
			_, _ = w.Write(manifest)
			return
		}
		_, _ = w.Write(asset)
	}))
	defer server.Close()
	client, err := newTestUpdateClient(Options{
		Repository: "mirainya/Prism-next", CurrentVersion: "2.0.12",
		Sources: []Source{{Name: "test", BaseURL: server.URL}}, Platform: "linux", Arch: "amd64",
	})
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if _, err := client.Download(context.Background(), target); err == nil || !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Fatalf("download error=%v, want SHA256 mismatch", err)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("staging directory has leftovers: %v", entries)
	}
}

func TestDownloadRejectsNoNewRelease(t *testing.T) {
	manifest := testManifest([]byte("archive"), "2.0.12")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(manifest) }))
	defer server.Close()
	client, err := newTestUpdateClient(Options{CurrentVersion: "2.0.12", Sources: []Source{{BaseURL: server.URL}}, Platform: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Download(context.Background(), t.TempDir())
	if !errors.Is(err, ErrNoUpdate) {
		t.Fatalf("error=%v, want ErrNoUpdate", err)
	}
}

func TestRunCLICheckWritesJSON(t *testing.T) {
	if err := RunCLI(context.Background(), []string{"update"}, "2.0.12", nil); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("missing subcommand error=%v", err)
	}
}

func testManifest(asset []byte, version string) []byte {
	return []byte(fmt.Sprintf(`{"schema":1,"product":"Prism","channel":"stable","version":%q,"tag":"v%s","commit":"%s","build_time":"2026-10-13T00:00:00Z","assets":[{"name":"prism-v%s-linux-amd64.tar.gz","platform":"linux","arch":"amd64","size":%d,"sha256":"%s","asset_path":"mirainya/Prism-next/releases/download/v%s/prism-v%s-linux-amd64.tar.gz"}]}`,
		version, version, strings.Repeat("a", 40), version, len(asset), sha256Hex(asset), version, version))
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func newTestUpdateClient(options Options) (*Client, error) {
	options.HTTPClient = http.DefaultClient // Only fixtures may connect to httptest loopback servers.
	return NewClient(options)
}

func TestDefaultClientHasOnlyGitHubAndRejectsPrivateHosts(t *testing.T) {
	client, err := NewClient(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if client.repository != "mirainya/Prism-next" {
		t.Fatalf("default repository=%q, want new public repository", client.repository)
	}
	if len(client.sources) != 1 || client.sources[0].BaseURL != DefaultGitHubBaseURL {
		t.Fatal("default source must only be GitHub")
	}
	client, err = NewClient(Options{Sources: []Source{{BaseURL: "http://127.0.0.1:9"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Check(context.Background()); err == nil {
		t.Fatal("default updater client accepted a private host")
	}
}
