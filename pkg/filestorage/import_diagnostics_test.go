package filestorage

import (
	"encoding/json"
	"testing"
)

func TestURLImportDecodesSafeCloudDiagnostics(t *testing.T) {
	for _, code := range []string{
		"import_cloud_disabled", "import_cloud_unavailable", "import_cloud_source_rejected", "import_cloud_transfer_aborted",
		"import_cloud_source_too_large", "import_cloud_source_dns_unavailable", "import_cloud_source_resume_unavailable",
		"import_cloud_source_unavailable", "import_cloud_source_incomplete", "import_cloud_source_slow",
		"import_cloud_source_changed", "import_cloud_source_range_invalid", "import_cloud_source_retries_exhausted",
		"import_cloud_transfer_deadline", "import_cloud_destination_rejected", "import_cloud_storage_unavailable",
		"import_cloud_part_checksum_mismatch", "import_cloud_configuration_invalid", "https://private.example/?key=test-secret",
	} {
		encoded, _ := json.Marshal(URLImportTask{TaskKey: managedImportTestKey, State: "failed", ErrorCode: code, Attempts: 5})
		got, err := decodeURLImportTask(encoded, managedImportTestKey)
		want := code
		if code[0] == 'h' {
			want = "import_outcome_unknown"
		}
		if err != nil || got.ErrorCode != want {
			t.Fatalf("code=%s err=%v", code, err)
		}
	}
}
