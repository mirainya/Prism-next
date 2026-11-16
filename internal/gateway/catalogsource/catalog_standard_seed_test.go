package catalogsource

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/mirainya/Prism/internal/gateway/repository"
)

func TestStandardCatalogSeedIsCanonicalDatabaseProtocolConfiguration(t *testing.T) {
	sql, err := os.ReadFile("../../../database/migrations/20261026_120000_clean_schema_baseline.sql")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile("(?m)^INSERT INTO `gw_protocol_definitions`[^\\n]+,'(\\{[^\\n]+\\})','([a-f0-9]{64})'").FindSubmatch(sql)
	if len(match) != 3 {
		t.Fatal("missing standard protocol seed")
	}
	raw := []byte(strings.ReplaceAll(string(match[1]), `\"`, `"`))
	canonical, err := repository.CanonicalProtocolJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonical)
	if hex.EncodeToString(digest[:]) != string(match[2]) {
		t.Fatal("SQL digest differs from the canonical runtime digest")
	}
	definition, err := ValidateDeclarativeProtocol(canonical)
	if err != nil || definition.Kind != "catalog_discovery" || len(definition.Steps) != 1 || definition.Steps[0].Path != "/v1/models" {
		t.Fatalf("definition=%+v error=%v", definition, err)
	}
	if bytes.Contains(raw, []byte("https://")) || bytes.Contains(raw, []byte("http://")) {
		t.Fatal("generic template embeds an upstream")
	}
}
