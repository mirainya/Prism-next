package repository

import "testing"

func TestUploadSlotsRequireStartedAttempt(t *testing.T) {
	for _, action := range []string{"upload_prepare", "upload_wait", "upload_part", "upload_complete", "upload_abort"} {
		if !slotAttemptAllows("started", action) {
			t.Fatalf("upload rejected: %s", action)
		}
		for _, state := range []string{"completed", "failed", "terminated_unknown", "recovery_pending"} {
			if slotAttemptAllows(state, action) {
				t.Fatalf("upload allowed on %s", state)
			}
		}
	}
}
