package catalogsource

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/mirainya/Prism/internal/gateway/repository"
)

func TestStandardCatalogSeedIsCanonicalDatabaseProtocolConfiguration(t *testing.T) {
	sql, err := os.ReadFile("../../../database/migrations/20261015_200000_clean_schema_baseline.sql")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile("(?m)^INSERT INTO `gw_protocol_definitions`.*'(\\{.*\\})','([0-9a-f]{64})'").FindSubmatch(sql)
	if len(match) != 3 {
		t.Fatal("missing standard protocol seed")
	}
	decoded, err := strconv.Unquote(`"` + string(match[1]) + `"`)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(decoded)
	canonical, err := repository.CanonicalProtocolJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(canonical)
	if hex.EncodeToString(digest[:]) != string(match[2]) {
		t.Fatal("SQL digest would differ from the runtime canonical digest")
	}
	definition, err := ValidateDeclarativeProtocol(canonical)
	if err != nil || definition.Kind != "catalog_discovery" || len(definition.Steps) != 1 || definition.Steps[0].Path != "/v1/models" {
		t.Fatalf("definition=%+v error=%v", definition, err)
	}
	if bytes.Contains(raw, []byte("https://")) || bytes.Contains(raw, []byte("aicost")) || bytes.Contains(raw, []byte("cangyuan")) {
		t.Fatal("generic template embeds an upstream")
	}
}
