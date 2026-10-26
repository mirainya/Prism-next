package catalogsource

import (
	"bytes"
	"os"
	"regexp"
	"testing"

	"github.com/mirainya/Prism/internal/gateway/repository"
)

func TestStandardCatalogSeedIsCanonicalDatabaseProtocolConfiguration(t *testing.T) {
	sql, err := os.ReadFile("../../../database/migrations/20261010_191000_standard_model_catalog_protocol.sql")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`SET @prism_standard_model_protocol = '([^']+)';`).FindSubmatch(sql)
	if len(match) != 2 {
		t.Fatal("missing standard protocol seed")
	}
	raw := match[1]
	canonical, err := repository.CanonicalProtocolJSON(raw)
	if err != nil || !bytes.Equal(canonical, raw) {
		t.Fatal("SQL digest would differ from the runtime canonical digest")
	}
	definition, err := ValidateDeclarativeProtocol(raw)
	if err != nil || definition.Kind != "catalog_discovery" || len(definition.Steps) != 1 || definition.Steps[0].Path != "/v1/models" {
		t.Fatalf("definition=%+v error=%v", definition, err)
	}
	if bytes.Contains(raw, []byte("https://")) || bytes.Contains(raw, []byte("http://")) {
		t.Fatal("generic template embeds an upstream")
	}
}
