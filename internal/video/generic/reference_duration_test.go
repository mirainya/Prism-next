package generic

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mirainya/Prism/internal/video"
)

func TestReferenceDurationDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name     string
		duration float64
		message  string
	}{
		{"missing", 0, "missing or zero"},
		{"short", 1.7, "at least 1.8 seconds"},
		{"boundary", 1.8, ""},
		{"long", 30.1, "at most 30 seconds"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRequestRule(&video.GenerateRequest{Content: []video.ContentItem{{Type: "audio_url", DurationSeconds: tc.duration}}}, validationRule{MediaDurationMin: 1.8, MediaDurationMax: 30})
			if tc.message == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.message) || !strings.Contains(err.Error(), "content[0].duration_seconds") {
				t.Fatalf("unexpected error: %v", err)
			}
			var detail *ReferenceDurationError
			if !errors.As(fmt.Errorf("invalid input: %w", err), &detail) || detail.Actual != tc.duration {
				t.Fatalf("lost diagnostic: %v", err)
			}
		})
	}
}
