package bizmetrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestCountersIncrement 各 API 递增对应标签系列（间接验证注册成功——未注册会 panic）。
func TestCountersIncrement(t *testing.T) {
	SiteSyncInc(SyncSuccess)
	SiteSyncInc(SyncSuccess)
	SiteSyncInc(SyncFail)

	// siteSync 是 Vec，取具体系列值需要 Collector；用 CollectAndCount 验证系列存在
	n := testutil.CollectAndCount(siteSyncTotal)
	if n < 2 {
		t.Fatalf("success+fail 两个系列应存在，got %d", n)
	}

	KeepAliveInc(KeepAliveRefreshed)
	KeepAliveInc(KeepAliveLoggedOut)
	KeepAliveInc(KeepAliveFail)
	if n := testutil.CollectAndCount(cookieKeepaliveTotal); n < 3 {
		t.Fatalf("refreshed+logged_out+fail 三个系列应存在，got %d", n)
	}

	HRCheckInc(HRCheckDangerous)
	HRCheckInc(HRCheckClean)
	if n := testutil.CollectAndCount(hrCheckTotal); n < 2 {
		t.Fatalf("dangerous+clean 两个系列应存在，got %d", n)
	}
}

// TestLLMCounters LLM 网关用量计数：请求按 source×result 分系列、token 按方向累加、负值忽略。
func TestLLMCounters(t *testing.T) {
	LLMRequestInc(LLMSourceOpenAI, LLMResultOK)
	LLMRequestInc(LLMSourceOpenAI, LLMResultOK)
	LLMRequestInc(LLMSourceOpenAI, LLMResultError)
	LLMRequestInc(LLMSourceAssistant, LLMResultOK)
	if n := testutil.CollectAndCount(llmRequestsTotal); n < 3 {
		t.Fatalf("openai{ok,error}+assistant{ok} 三个系列应存在，got %d", n)
	}

	beforeIn := testutil.ToFloat64(llmTokensTotal.WithLabelValues(LLMTokensInput))
	beforeOut := testutil.ToFloat64(llmTokensTotal.WithLabelValues(LLMTokensOutput))
	LLMTokensAdd(1200, 340)
	if v := testutil.ToFloat64(llmTokensTotal.WithLabelValues(LLMTokensInput)); v-beforeIn != 1200 {
		t.Fatalf("input 应 +1200，got %v→%v", beforeIn, v)
	}
	if v := testutil.ToFloat64(llmTokensTotal.WithLabelValues(LLMTokensOutput)); v-beforeOut != 340 {
		t.Fatalf("output 应 +340，got %v→%v", beforeOut, v)
	}
	// 零/负值不污染
	LLMTokensAdd(0, 0)
	LLMTokensAdd(-5, -7)
	if v := testutil.ToFloat64(llmTokensTotal.WithLabelValues(LLMTokensInput)); v != beforeIn+1200 {
		t.Fatalf("零/负值不应变化: %v", v)
	}
}

// TestHRAlertsAdd H&R 告警计数：正值累加、零/负值忽略（干净轮次不污染）。
func TestHRAlertsAdd(t *testing.T) {
	before := hrAlertsTotalValue(t)
	HRAlertsAdd(5)
	after := hrAlertsTotalValue(t)
	if after-before != 5 {
		t.Fatalf("应 +5，got %v→%v", before, after)
	}
	HRAlertsAdd(0)
	HRAlertsAdd(-3)
	if v := hrAlertsTotalValue(t); v != after {
		t.Fatalf("零/负值不应变化: %v != %v", v, after)
	}
}

// hrAlertsTotalValue 读裸 counter 当前值（测试内可见的包级变量）。
func hrAlertsTotalValue(t *testing.T) float64 {
	t.Helper()
	return testutil.ToFloat64(hrAlertsTotal)
}

// TestMetricsExposedViaPrometheus 全套业务指标能经标准 registry 输出
// （端到端：/metrics 端点用的默认 registry 必须包含这些系列）。
func TestMetricsExposedViaPrometheus(t *testing.T) {
	SiteSyncInc(SyncSuccess)
	KeepAliveInc(KeepAliveRefreshed)
	HRCheckInc(HRCheckDangerous)
	HRAlertsAdd(2)

	// 用默认 registry gather（promhttp.Handler() 同源）
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	for _, want := range []string{
		"ptpatronus_site_sync_total",
		"ptpatronus_cookie_keepalive_total",
		"ptpatronus_hr_alerts_total",
		"ptpatronus_hr_check_total",
	} {
		if !names[want] {
			t.Fatalf("默认 registry 缺业务指标 %s，实际有: %v", want, keys(names))
		}
	}
}

func keys(m map[string]bool) string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return strings.Join(out, ",")
}
