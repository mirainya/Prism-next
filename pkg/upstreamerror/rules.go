package upstreamerror

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"unicode/utf8"
)

var ErrInvalidRules = errors.New("invalid upstream error rules")

type Rules struct {
	Patterns []string `json:"patterns"`
	Message  string   `json:"message"`
}

type compiledRules struct {
	rules    Rules
	patterns []*regexp.Regexp
}

func DefaultRules() Rules {
	return Rules{
		Message: "服务暂时不可用，请稍后重试",
		Patterns: []string{
			`(?i)余额|\bbalance\b`,
			`(?i)[¥￥$€£]\s*[0-9]+(?:[.,][0-9]+)*|\b(?:USD|CNY|RMB|EUR|GBP)\s*[0-9]+(?:[.,][0-9]+)*|[0-9]+(?:[.,][0-9]+)*\s*(?:元|美元|人民币|\b(?:USD|CNY|RMB|EUR|GBP)\b)`,
		},
	}
}

func compile(rules Rules) (*compiledRules, error) {
	if strings.TrimSpace(rules.Message) == "" || !utf8.ValidString(rules.Message) || utf8.RuneCountInString(rules.Message) > 500 || len(rules.Patterns) > 32 {
		return nil, ErrInvalidRules
	}
	compiled := &compiledRules{rules: Rules{Message: rules.Message, Patterns: append([]string{}, rules.Patterns...)}}
	for i, pattern := range rules.Patterns {
		if strings.TrimSpace(pattern) == "" || len(pattern) > 2048 {
			return nil, fmt.Errorf("%w: patterns[%d] must contain 1–2048 bytes", ErrInvalidRules, i)
		}
		value, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("%w: patterns[%d]: %v", ErrInvalidRules, i, err)
		}
		compiled.patterns = append(compiled.patterns, value)
	}
	return compiled, nil
}

var activeRules = func() *atomic.Pointer[compiledRules] {
	rules, err := compile(DefaultRules())
	if err != nil {
		panic(err)
	}
	active := new(atomic.Pointer[compiledRules])
	active.Store(rules)
	return active
}()

func Sanitize(message string) string {
	rules := activeRules.Load()
	for _, pattern := range rules.patterns {
		if pattern.MatchString(message) {
			return rules.rules.Message
		}
	}
	return message
}
