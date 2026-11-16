package service

import (
	"strings"
	"testing"
	"time"
)

func TestCallResultPendingRemainsVisibleWithoutPrivateReasonCode(t *testing.T) {
	for _, admin := range []bool{true, false} {
		row := callResultDeliveryRow{APICallResultDelivery: APICallResultDelivery{
			ID: 7, AttemptID: 2, State: "delivery_failed", DeliveryMode: "managed_copy",
			ReasonCode: "managed_copy_import_retry_pending", RetryStatus: "pending", RetryAttempts: 101,
		}, SourceAvailable: true}
		got := row.detail(admin, time.Now())
		if !got.TransferPending || got.CanRetry || got.Available || !strings.Contains(got.ReasonMessage, "正在处理") {
			t.Fatalf("processing result was misclassified: %+v", got)
		}
		if !admin && (got.ReasonCode != "" || got.AttemptID != 0) {
			t.Fatal("user view exposed internal identifiers")
		}
		row.RetryStatus = "dead_letter"
		if row.detail(admin, time.Now()).TransferPending {
			t.Fatal("stopped recovery still appears to be running")
		}
	}
}

func TestCallResultUncertainAndBindingMessages(t *testing.T) {
	for reason, want := range map[string]string{
		"managed_copy_import_retry_uncertain": "不会重复上传",
		"managed_copy_import_stop_uncertain":  "已停止自动查询",
		"managed_copy_import_stop_pending":    "已停止自动查询",
		"managed_copy_import_stop_binding":    "存储绑定已变化",
	} {
		if got := callDeliveryFailureMessage(reason); !strings.Contains(got, want) || strings.Contains(got, "https://") {
			t.Fatalf("reason=%s message=%s", reason, got)
		}
	}
}

func TestCallResultImportDiagnosticsHaveSpecificSafeMessages(t *testing.T) {
	for code, want := range map[string]string{
		"source_resume": "不支持断点续传", "source_dns": "域名解析失败",
		"source_incomplete": "读取不完整", "source_slow": "速度持续过低",
		"source_changed": "续传响应发生变化", "source_interrupted": "读取反复中断",
		"transfer_timeout": "总时间限制", "destination_rejected": "拒绝写入",
		"source_unavailable": "无法连接或读取", "source_rejected": "源文件读取失败",
		"worker_lost": "工作进程中断", "configuration": "配置无效",
		"transfer_aborted": "未保留更细原因", "outcome_unknown": "未返回可识别",
		"rejected": "未保留具体原因",
	} {
		row := callResultDeliveryRow{APICallResultDelivery: APICallResultDelivery{
			State: "delivery_failed", DeliveryMode: "managed_copy", RetryStatus: "dead_letter",
			ReasonCode: "managed_copy_import_stop_" + code,
		}}
		got := row.detail(false, time.Now())
		if !strings.Contains(got.ReasonMessage, want) || got.ReasonCode != "" || got.TransferPending || got.FailureRetryable {
			t.Fatalf("code=%s result=%+v", code, got)
		}
	}
}
