package middleware

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/pkg/config"
)

func TestTokenRateLimitSharesPublicAndPlaygroundCounter(t *testing.T) {
	previous := config.C
	config.C = &config.Config{RateLimit: config.RateLimitConfig{Enabled: true, RequestsPerMin: 10}}
	t.Cleanup(func() { config.C = previous })
	counts := map[string]int64{}
	increment := func(_ context.Context, key string) (int64, error) { counts[key]++; return counts[key], nil }
	request := func(path string, tokenID uint, rate int) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", path, nil)
		c.Set(ContextKeyTokenID, tokenID)
		c.Set(ContextKeyToken, &model.Token{RateLimit: rate})
		if enforceTokenRateLimit(c, increment) {
			// A request can validate its Key more than once; count it once.
			if !enforceTokenRateLimit(c, increment) {
				t.Fatal("request was counted twice")
			}
			c.Status(204)
			c.Writer.WriteHeaderNow()
		}
		return w
	}
	if w := request("/v1/images/generations", 7, 2); w.Code != 204 || w.Header().Get("X-RateLimit-Limit") != "2" || w.Header().Get("X-RateLimit-Remaining") != "1" {
		t.Fatalf("first response=%d headers=%v", w.Code, w.Header())
	}
	if w := request("/api/playground/7/images/generations", 7, 2); w.Code != 204 || w.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("second response=%d headers=%v", w.Code, w.Header())
	}
	if w := request("/v1/responses", 7, 2); w.Code != 429 || w.Header().Get("Retry-After") == "" || !strings.Contains(w.Body.String(), "rate_limit_exceeded") {
		t.Fatalf("over limit response=%d body=%s", w.Code, w.Body)
	}
	if counts["rate_limit:token:7"] != 3 {
		t.Fatalf("unexpected counters=%v", counts)
	}
	if w := request("/v1/responses", 8, 2); w.Code != 204 {
		t.Fatal("another key shared the quota")
	}
	// A changed policy applies to the existing window, not a fresh counter.
	if w := request("/v1/responses", 7, 1); w.Code != 429 {
		t.Fatal("lowering the limit reset its counter")
	}
}

func TestTokenRateLimitUsesGlobalCeilingAndSwitch(t *testing.T) {
	for _, item := range []struct{ token, global, want int }{{2, 10, 2}, {100, 10, 10}, {0, 10, 10}, {-1, 10, 10}, {100, 0, 60}, {0, -1, 60}} {
		if actual := effectiveTokenRateLimit(&model.Token{RateLimit: item.token}, item.global); actual != item.want {
			t.Fatalf("token=%d global=%d limit=%d want %d", item.token, item.global, actual, item.want)
		}
	}
	previous := config.C
	t.Cleanup(func() { config.C = previous })
	for _, cfg := range []*config.Config{nil, {RateLimit: config.RateLimitConfig{Enabled: false, RequestsPerMin: 1}}} {
		config.C = cfg
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
		if !enforceTokenRateLimit(c, func(context.Context, string) (int64, error) {
			t.Fatal("disabled limiter accessed Redis")
			return 0, nil
		}) {
			t.Fatal("disabled limiter denied request")
		}
	}
	config.C = &config.Config{RateLimit: config.RateLimitConfig{Enabled: true, RequestsPerMin: 1}}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	if !enforceTokenRateLimit(c, func(context.Context, string) (int64, error) { return 0, errors.New("synthetic Redis outage") }) || c.IsAborted() {
		t.Fatal("Redis failure changed existing availability behavior")
	}
}
