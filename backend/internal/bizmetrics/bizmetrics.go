// Package bizmetrics 业务链路 Prometheus 指标：站点数据同步 / Cookie 保活 / H&R 告警。
// 与 HTTP 层指标（middleware.Prometheus 的 ptpatronus_http_*）互补——这里回答
// 「保号自动化运转得怎么样」，供 Grafana 看板与告警规则消费：
//
//	ptpatronus_site_sync_total{result}            站点统计同步（success|fail）
//	ptpatronus_cookie_keepalive_total{result}     Cookie 保活（refreshed|logged_out|fail）
//	ptpatronus_hr_alerts_total                    H&R 临期告警发出的种子条数
//	ptpatronus_hr_check_total{result}             H&R 巡检（dangerous|clean）
//	ptpatronus_llm_requests_total{source,result} LLM 请求（source=openai|anthropic|assistant；result=ok|error）
//	ptpatronus_llm_tokens_total{direction}       LLM token 用量（direction=input|output）
package bizmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// SyncResult 站点同步结果标签值。
const (
	SyncSuccess = "success"
	SyncFail    = "fail"
)

// KeepAliveResultLabel Cookie 保活结果标签值。
const (
	KeepAliveRefreshed = "refreshed"
	KeepAliveLoggedOut = "logged_out"
	KeepAliveFail      = "fail"
)

// HRCheckResultLabel H&R 巡检结果标签值。
const (
	HRCheckDangerous = "dangerous"
	HRCheckClean     = "clean"
)

// LLMSourceLabel LLM 用量来源（协议端点 / 内置助手聊天）。
const (
	LLMSourceOpenAI    = "openai"
	LLMSourceAnthropic = "anthropic"
	LLMSourceAssistant = "assistant"
)

// LLMResultLabel LLM 请求结果标签值。
const (
	LLMResultOK    = "ok"
	LLMResultError = "error"
)

// LLMDirectionLabel token 计数方向。
const (
	LLMTokensInput  = "input"
	LLMTokensOutput = "output"
)

var (
	siteSyncTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ptpatronus_site_sync_total",
			Help: "Site stats sync attempts, labeled by result (success|fail).",
		},
		[]string{"result"},
	)
	cookieKeepaliveTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ptpatronus_cookie_keepalive_total",
			Help: "Cookie keepalive outcomes per site, labeled by result (refreshed|logged_out|fail).",
		},
		[]string{"result"},
	)
	hrAlertsTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "ptpatronus_hr_alerts_total",
			Help: "Dangerous H&R torrents that triggered an alert (cumulative torrent count).",
		},
	)
	hrCheckTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ptpatronus_hr_check_total",
			Help: "H&R check rounds, labeled by result (dangerous|clean).",
		},
		[]string{"result"},
	)
	llmRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ptpatronus_llm_requests_total",
			Help: "LLM requests (agent runs), labeled by source (openai|anthropic|assistant) and result (ok|error).",
		},
		[]string{"source", "result"},
	)
	llmTokensTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ptpatronus_llm_tokens_total",
			Help: "LLM token usage, labeled by direction (input|output); reflects partial usage on aborted runs.",
		},
		[]string{"direction"},
	)
)

func init() {
	prometheus.MustRegister(siteSyncTotal, cookieKeepaliveTotal, hrAlertsTotal, hrCheckTotal, llmRequestsTotal, llmTokensTotal)
}

// SiteSyncInc 记一次站点统计同步（Dashboard.SyncAll/SyncOne 与 snapshot 链共用）。
func SiteSyncInc(result string) { siteSyncTotal.WithLabelValues(result).Inc() }

// KeepAliveInc 记一次 Cookie 保活结果（每站每次一轮各记一条）。
func KeepAliveInc(result string) { cookieKeepaliveTotal.WithLabelValues(result).Inc() }

// HRAlertsAdd 记一轮 H&R 巡检发现的危险种子数（0 不计——干净轮次走 HRCheckInc(clean)）。
func HRAlertsAdd(n int) {
	if n > 0 {
		hrAlertsTotal.Add(float64(n))
	}
}

// HRCheckInc 记一轮 H&R 巡检结论（dangerous=有危险种子，clean=无）。
func HRCheckInc(result string) { hrCheckTotal.WithLabelValues(result).Inc() }

// LLMRequestInc 记一次 LLM 请求（source=协议端点或内置助手，result=ok|error）。
// 计数落在 agent.Run 的用量统计 defer 里——与 UsageRecord 同源，重启清零口径一致。
func LLMRequestInc(source, result string) { llmRequestsTotal.WithLabelValues(source, result).Inc() }

// LLMTokensAdd 累加一次 LLM 运行的 token 用量（in/out 分方向；负值忽略）。
func LLMTokensAdd(inTok, outTok int) {
	if inTok > 0 {
		llmTokensTotal.WithLabelValues(LLMTokensInput).Add(float64(inTok))
	}
	if outTok > 0 {
		llmTokensTotal.WithLabelValues(LLMTokensOutput).Add(float64(outTok))
	}
}
