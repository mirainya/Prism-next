package domain

import "github.com/mirainya/Prism/pkg/upstreamerror"

// SanitizeUpstreamFinancialMessage hides provider balances and monetary amounts.
// Call only for upstream errors, not local user billing errors.
func SanitizeUpstreamFinancialMessage(message string) string {
	return upstreamerror.Sanitize(message)
}
