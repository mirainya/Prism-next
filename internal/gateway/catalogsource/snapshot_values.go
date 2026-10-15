// Package catalogsource stages model and pricing facts using database-owned
// HTTP and JSON mapping definitions. Publication remains a separate operation.
package catalogsource

import (
	"bytes"
	"encoding/json"
	"github.com/mirainya/Prism/internal/gateway/billing"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

var endpointPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

func normalizeProviderText(value string) string {
	if !utf8.ValidString(value) {
		return value
	}
	return strings.Join(strings.Fields(value), " ")
}

func optionalDecimal(number json.Number) (*string, error) {
	if number == "" {
		return nil, nil
	}
	amount, err := billing.ParseAmount(number.String(), 18, true)
	if err != nil {
		return nil, ErrInvalidSnapshot
	}
	value := amount.String()
	return &value, nil
}

func canonicalScalar(value json.RawMessage) (string, error) {
	if len(value) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return "", nil
	}
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var scalar any
	if err := decoder.Decode(&scalar); err != nil {
		return "", ErrInvalidSnapshot
	}
	switch scalar.(type) {
	case string, json.Number:
	default:
		return "", ErrInvalidSnapshot
	}
	encoded, err := json.Marshal(scalar)
	if err != nil || len(encoded) > 128 {
		return "", ErrInvalidSnapshot
	}
	return string(encoded), nil
}

func normalizeStrings(values []string, maxLength int, pattern *regexp.Regexp) ([]string, error) {
	if len(values) > 256 {
		return nil, ErrInvalidSnapshot
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if !validText(value, maxLength) || pattern != nil && !pattern.MatchString(value) {
			return nil, ErrInvalidSnapshot
		}
		if _, exists := seen[value]; exists {
			return nil, ErrInvalidSnapshot
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func validText(value string, maxRunes int) bool {
	return value != "" && validOptionalText(value, maxRunes)
}

func validModelCode(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	runes := []rune(value)
	if len(runes) == 0 || len(runes) > 128 || !unicode.IsLetter(runes[0]) && !unicode.IsDigit(runes[0]) {
		return false
	}
	for _, current := range runes[1:] {
		if unicode.IsLetter(current) || unicode.IsDigit(current) || strings.ContainsRune("._:/-%", current) {
			continue
		}
		return false
	}
	return true
}

func validOptionalText(value string, maxRunes int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= maxRunes &&
		!strings.ContainsAny(value, "\x00\r\n\t")
}

func boundedDecimal(number json.Number, upper string) (string, error) {
	if number == "" {
		return "", ErrInvalidSnapshot
	}
	amount, err := billing.ParseAmount(number.String(), 18, true)
	if err != nil || amount.Sign() < 0 {
		return "", ErrInvalidSnapshot
	}
	bound, err := billing.ParseAmount(upper, 18, true)
	if err != nil || amount.Cmp(bound) > 0 {
		return "", ErrInvalidSnapshot
	}
	return amount.String(), nil
}
func worseChannelStatus(left, right string) string {
	if right == "" {
		return left
	}
	if left == "" || channelStatusRank(right) > channelStatusRank(left) {
		return right
	}
	return left
}

func channelStatusRank(value string) int {
	status := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(status, "不可用"), strings.Contains(status, "异常"), strings.Contains(status, "unavailable"), strings.Contains(status, "failed"), strings.Contains(status, "error"), status == "down":
		return 3
	case strings.Contains(status, "波动"), strings.Contains(status, "不稳定"), strings.Contains(status, "degrad"), strings.Contains(status, "fluctuat"), strings.Contains(status, "unstable"):
		return 2
	case strings.Contains(status, "正常"), strings.Contains(status, "normal"), strings.Contains(status, "healthy"), strings.Contains(status, "available"):
		return 1
	default:
		return 1
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
