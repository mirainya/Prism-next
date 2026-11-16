package domain

import "testing"

func TestSanitizeUpstreamFinancialMessage(t *testing.T) {
	for _, message := range []string{
		"积分不足，请充值后重试", "积分不足", "请充值后重试",
		"余额不足，账户剩余额度: ¥3.520000，本次需要: ¥3.900000",
		"余额不足", "Insufficient BALANCE", "required $2.00", "cost ￥3.9",
		"cost €2", "cost £2", "cost USD 2.00", "required 3.9元", "cost 2 USD",
	} {
		if got := SanitizeUpstreamFinancialMessage(message); got != "服务暂时不可用，请稍后重试" {
			t.Errorf("message %q was not replaced: %q", message, got)
		}
	}
	for _, message := range []string{"HTTP 429", "max_tokens must be <= 4096", "invalid image size 1024x1024", "invalid model", ""} {
		if got := SanitizeUpstreamFinancialMessage(message); got != message {
			t.Errorf("ordinary message %q changed to %q", message, got)
		}
	}
}
