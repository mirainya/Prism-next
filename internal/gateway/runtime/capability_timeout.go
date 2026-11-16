package runtime

import "time"

const (
	maximumCapabilityExchangeTimeout = 10 * time.Minute
	// Keep time for result persistence after the longest provider exchange.
	attemptOutboxLeaseDuration = maximumCapabilityExchangeTimeout + time.Minute
)

func capabilityExchangeTimeout(configured time.Duration) time.Duration {
	if configured <= 0 {
		return 5 * time.Minute
	}
	return min(configured, maximumCapabilityExchangeTimeout)
}
