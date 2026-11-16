package generic

import (
	"errors"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

// ResultAuthentication is opt-in. Existing declarations produce no result
// credential requirement, even when submit/query use Authorization headers.
func (c *RuntimeCodec) ResultAuthentication() (header, prefix string, enabled bool) {
	if c.adapter.config.Response.ResultAuth != "upstream" {
		return "", "", false
	}
	return c.adapter.config.AuthKey, valueOrEmpty(c.adapter.config.AuthPrefix), true
}

func (c *RuntimeCodec) exactBillingSeconds(body []byte, completed bool) (string, error) {
	paths := c.adapter.config.Response.BillingSecondsPaths
	if !completed || len(paths) == 0 {
		return "", nil
	}
	payload, err := c.adapter.responsePayload(body)
	if err != nil {
		return "", err
	}
	value := firstResult(payload, paths)
	if !value.Exists() || (value.Type != gjson.Number && value.Type != gjson.String) {
		return "", errors.New("generic provider billing seconds are missing or invalid")
	}
	raw := strings.TrimSpace(value.Raw)
	if value.Type == gjson.String {
		raw = strings.TrimSpace(value.String())
	}
	// Match the integer meter and SQL DECIMAL integer capacity before any
	// expansion; provider-controlled exponent notation must not allocate
	// arbitrarily large decimal strings.
	if len(raw) == 0 || len(raw) > 20 || raw[0] == '0' {
		return "", errors.New("generic provider billing seconds must be a positive integer")
	}
	for _, digit := range raw {
		if digit < '0' || digit > '9' {
			return "", errors.New("generic provider billing seconds must be a positive integer")
		}
	}
	if maximum := c.adapter.config.Response.BillingSecondsMax; maximum > 0 {
		bound := strconv.Itoa(maximum)
		if len(raw) > len(bound) || (len(raw) == len(bound) && raw > bound) {
			return "", errors.New("generic provider billing seconds exceed the configured bound")
		}
	}
	return raw, nil
}
