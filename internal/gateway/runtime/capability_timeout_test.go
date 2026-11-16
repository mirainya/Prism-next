package runtime

import (
	"testing"
	"time"
)

func TestCapabilityExchangeHonorsTenMinuteImageDeadline(t *testing.T) {
	for _, test := range []struct {
		name                 string
		configured, expected time.Duration
	}{
		{"default", 0, 5 * time.Minute},
		{"invalid", -time.Second, 5 * time.Minute},
		{"short_channel", 30 * time.Second, 30 * time.Second},
		{"existing_channel", 5 * time.Minute, 5 * time.Minute},
		{"long_image_channel", 10 * time.Minute, 10 * time.Minute},
		{"over_maximum", 15 * time.Minute, 10 * time.Minute},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := capabilityExchangeTimeout(test.configured); got != test.expected {
				t.Fatalf("provider timeout=%s, want %s", got, test.expected)
			}
		})
	}
}

func TestTenMinuteCapabilityHasTimeToCommitBeforeRecovery(t *testing.T) {
	exchange := capabilityExchangeTimeout(10 * time.Minute)
	policy, err := normalizeCapabilityRecoveryPolicy(CapabilityRecoveryPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if attemptOutboxLeaseDuration-exchange < time.Minute {
		t.Fatal("background worker lease can expire before the image result is committed")
	}
	if policy.Grace-exchange < time.Minute {
		t.Fatal("stale recovery can interrupt a live image generation")
	}
}
