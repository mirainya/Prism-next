package middleware

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mirainya/Prism/internal/model"
	"github.com/mirainya/Prism/pkg/cache"
	"github.com/mirainya/Prism/pkg/config"
)

// rateLimitScript 原子递增计数并在首次创建时设置过期时间(秒),返回当前计数
// 用 Lua 保证 INCR 与 EXPIRE 的原子性,避免两条命令间 Redis 故障导致 key 无过期时间
const rateLimitScript = `
local c = redis.call('INCR', KEYS[1])
if c == 1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return c
`

// RateLimit 基于 Redis 的固定窗口限流中间件
// keyFunc 用于从请求中提取限流 key（如 token ID 或 IP）
func RateLimit(keyFunc func(c *gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.Get()
		if cfg == nil || !cfg.RateLimit.Enabled || cache.Client == nil {
			c.Next()
			return
		}

		if enforceRateLimit(c, keyFunc(c), effectiveTokenRateLimit(nil, cfg.RateLimit.RequestsPerMin), incrementRateLimit) {
			c.Next()
		}
	}
}

func effectiveTokenRateLimit(token *model.Token, globalLimit int) int {
	if globalLimit <= 0 {
		globalLimit = 60
	}
	if token != nil && token.RateLimit > 0 {
		return min(token.RateLimit, globalLimit)
	}
	return globalLimit
}

func incrementRateLimit(ctx context.Context, key string) (int64, error) {
	return cache.Client.Eval(ctx, rateLimitScript, []string{key}, 60).Int64()
}

func enforceRateLimit(c *gin.Context, key string, limit int, increment func(context.Context, string) (int64, error)) bool {
	count, err := increment(c.Request.Context(), "rate_limit:"+key)
	// Preserve the existing availability policy when Redis is unavailable.
	if err != nil {
		return true
	}
	c.Header("X-RateLimit-Limit", fmt.Sprint(limit))
	c.Header("X-RateLimit-Remaining", fmt.Sprint(max(0, int64(limit)-count)))
	if count <= int64(limit) {
		return true
	}
	c.Header("Retry-After", "60")
	writeGatewayProtocolError(c, http.StatusTooManyRequests, "rate limit exceeded, please try again later", "rate_limit_error", "rate_limit_exceeded")
	c.Abort()
	return false
}

// EnforceTokenRateLimit also protects the JWT-owned Playground entrance.
// Both entrances use the same counter and enforce the global ceiling.
func EnforceTokenRateLimit(c *gin.Context) bool {
	if cache.Client == nil {
		return true
	}
	return enforceTokenRateLimit(c, incrementRateLimit)
}

func enforceTokenRateLimit(c *gin.Context, increment func(context.Context, string) (int64, error)) bool {
	if c.GetBool("token_rate_limit_checked") {
		return !c.IsAborted()
	}
	c.Set("token_rate_limit_checked", true)
	cfg := config.Get()
	if cfg == nil || !cfg.RateLimit.Enabled {
		return true
	}
	key := "ip:" + c.ClientIP()
	if id := GetTokenID(c); id != 0 {
		key = fmt.Sprintf("token:%d", id)
	}
	return enforceRateLimit(c, key, effectiveTokenRateLimit(GetToken(c), cfg.RateLimit.RequestsPerMin), increment)
}

// RateLimitByToken 按 Token ID 限流（用于 v1 API）
func RateLimitByToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		if EnforceTokenRateLimit(c) {
			c.Next()
		}
	}
}

// RateLimitByIP 按 IP 限流（用于 auth API）
func RateLimitByIP() gin.HandlerFunc {
	return RateLimit(func(c *gin.Context) string {
		return "ip:" + c.ClientIP()
	})
}
