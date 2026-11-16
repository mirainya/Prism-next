package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/mirainya/Prism/internal/migrate"
)

func TestMigrationConfirmationDigestChangesWithSQL(t *testing.T) {
	one := NewMigrationPlan([]migrate.Migration{{Version: "20261013_120000", SQL: "CREATE TABLE a(id INT)"}})
	two := NewMigrationPlan([]migrate.Migration{{Version: "20261013_120000", SQL: "CREATE TABLE b(id INT)"}})
	if one.Digest == two.Digest {
		t.Fatal("confirmation must bind exact SQL")
	}
	if NewMigrationPlan(nil).Digest != NewMigrationPlan([]migrate.Migration{}).Digest {
		t.Fatal("empty migration plans must be stable")
	}
}

func TestExtractOnlyExpectedExecutable(t *testing.T) {
	root := "prism-v2.0.14-linux-amd64/"
	for _, test := range []struct {
		name    string
		headers []*tar.Header
		valid   bool
	}{
		{"regular", []*tar.Header{{Name: root + "prism", Typeflag: tar.TypeReg, Size: 3}, {Name: root + "configs/config.example.yaml", Typeflag: tar.TypeReg, Size: 3}}, true},
		{"traversal", []*tar.Header{{Name: root + "../prism", Typeflag: tar.TypeReg, Size: 3}}, false},
		{"link", []*tar.Header{{Name: root + "prism", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}}, false},
		{"duplicate", []*tar.Header{{Name: root + "prism", Typeflag: tar.TypeReg, Size: 3}, {Name: root + "prism", Typeflag: tar.TypeReg, Size: 3}}, false},
		{"missing", []*tar.Header{{Name: root + "README.md", Typeflag: tar.TypeReg, Size: 3}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var buffer bytes.Buffer
			compressed := gzip.NewWriter(&buffer)
			archive := tar.NewWriter(compressed)
			for _, header := range test.headers {
				if err := archive.WriteHeader(header); err != nil {
					t.Fatal(err)
				}
				if header.Typeflag == tar.TypeReg {
					_, _ = archive.Write([]byte("bin"))
				}
			}
			_ = archive.Close()
			_ = compressed.Close()
			path := filepath.Join(t.TempDir(), "release.tar.gz")
			if err := os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			result := DownloadResult{Path: path, Version: "2.0.14", Asset: Asset{Platform: "linux", Arch: "amd64"}, Size: int64(buffer.Len()), SHA256: sha256Hex(buffer.Bytes())}
			destination := filepath.Join(t.TempDir(), "candidate")
			file, err := ExtractExecutable(result, destination)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%t err=%v", test.valid, err)
			}
			if test.valid {
				content, _ := os.ReadFile(file)
				if string(content) != "bin" {
					t.Fatal("unexpected executable")
				}
				if _, err := os.Stat(filepath.Join(destination, "configs")); !os.IsNotExist(err) {
					t.Fatal("archive configs must not replace installed config")
				}
			}
		})
	}
}

func TestExtractRejectsAlteredStagedArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive")
	_ = os.WriteFile(path, []byte("altered"), 0600)
	_, err := ExtractExecutable(DownloadResult{Path: path, Version: "2.0.14", Asset: Asset{Platform: "linux", Arch: "amd64"}, Size: 7, SHA256: sha256Hex([]byte("trusted"))}, t.TempDir())
	if err == nil {
		t.Fatal("altered archive must not be executed")
	}
}
