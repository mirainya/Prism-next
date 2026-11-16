package repository

import "testing"

func TestReferenceUploadLogActions(t *testing.T) {
	for _, action := range []string{"upload_prepare", "upload_wait", "upload_part", "upload_complete", "upload_abort"} {
		if !validRequestAction(action) {
			t.Fatalf("upload action cannot be logged: %s", action)
		}
	}
	if validRequestAction("upload_unknown") {
		t.Fatal("unknown upload action accepted")
	}
}
