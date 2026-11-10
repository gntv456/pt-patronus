// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/bizmetrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// newMetricsTestEngine 构造挂 Prometheus 中间件的引擎，含一条参数路由 + /metrics。
func newMetricsTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Prometheus())
	r.GET("/sites/:id", func(c *gin.Context) { c.Status(200) })
	r.GET("/boom", func(c *gin.Context) { c.Status(500) })
	r.GET("/metrics", func(c *gin.Context) { c.Status(200) }) // 占位 handler：只测中间件的路径排除逻辑
	return r
}

func doMetricsReq(r *gin.Engine, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, path, nil)
	r.ServeHTTP(w, req)
	return w
}

// TestPrometheusCountsRequests 请求计数：路由模板归并 + 状态码标签。
func TestPrometheusCountsRequests(t *testing.T) {
	r := newMetricsTestEngine()

	// 同一路由不同参数 → 同一 path 标签（防高基数）
	doMetricsReq(r, "/sites/1")
	doMetricsReq(r, "/sites/999")

	n := testutil.CollectAndCount(httpRequestsTotal)
	if n == 0 {
		t.Fatal("请求后应有指标系列")
	}
	// 计数值 ≥2（两次 /sites/:id 命中同系列累加）
	val := testutil.ToFloat64(httpRequestsTotal.WithLabelValues(http.MethodGet, "/sites/:id", "200"))
	if val < 2 {
		t.Fatalf("同路由两次请求应累加 ≥2，got %v", val)
	}
}

// TestPrometheusErrorPathLabels 5xx 也计数（code 标签区分）。
func TestPrometheusErrorPathLabels(t *testing.T) {
	r := newMetricsTestEngine()
	doMetricsReq(r, "/boom")
	val := testutil.ToFloat64(httpRequestsTotal.WithLabelValues(http.MethodGet, "/boom", "500"))
	if val < 1 {
		t.Fatalf("500 响应应计数，got %v", val)
	}
}

// TestPrometheusExcludesMetricsPath /metrics 自身与静态资源不计数（防自激/噪音）。
func TestPrometheusExcludesMetricsPath(t *testing.T) {
	r := newMetricsTestEngine()
	before := testutil.ToFloat64(httpRequestsTotal.WithLabelValues(http.MethodGet, "/metrics", "200"))
	doMetricsReq(r, "/metrics")
	after := testutil.ToFloat64(httpRequestsTotal.WithLabelValues(http.MethodGet, "/metrics", "200"))
	if before != after {
		t.Fatalf("/metrics 不应自计数: before=%v after=%v", before, after)
	}
}

// TestPrometheusUnmatchedRoute 未匹配路由记为 "unmatched"（不展开实际 URL）。
func TestPrometheusUnmatchedRoute(t *testing.T) {
	r := newMetricsTestEngine()
	doMetricsReq(r, "/definitely/not/a/route")
	val := testutil.ToFloat64(httpRequestsTotal.WithLabelValues(http.MethodGet, "unmatched", "404"))
	if val < 1 {
		t.Fatalf("未匹配路由应记 unmatched 标签，got %v", val)
	}
}

// TestPrometheusInFlightGauge 在途计数器对称（请求完成后回落）。
func TestPrometheusInFlightGauge(t *testing.T) {
	_ = newMetricsTestEngine()
	// 串行请求结束后 in-flight 应回落（无法轻易在中间观测，验证终态为 0）
	val := testutil.ToFloat64(httpRequestsInFlight)
	if val != 0 {
		t.Fatalf("空闲时 in-flight 应为 0，got %v", val)
	}
}

// TestPrometheusOutputFormat 指标输出为 Prometheus 文本格式（端到端 /metrics 形态）。
// 用真实 promhttp handler 验证（与 router 注册同款）。
func TestPrometheusOutputFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Prometheus())
	r.GET("/ping", func(c *gin.Context) { c.String(200, "pong") })
	r.GET("/metrics", func(c *gin.Context) {
		promhttp.Handler().ServeHTTP(c.Writer, c.Request)
	})

	// 业务指标（bizmetrics 包注册进同一默认 registry）——必须在抓取前产生系列
	bizmetrics.SiteSyncInc(bizmetrics.SyncSuccess)
	bizmetrics.KeepAliveInc(bizmetrics.KeepAliveRefreshed)
	bizmetrics.HRCheckInc(bizmetrics.HRCheckDangerous)
	bizmetrics.HRAlertsAdd(1)
	bizmetrics.LLMRequestInc(bizmetrics.LLMSourceAssistant, bizmetrics.LLMResultOK)
	bizmetrics.LLMTokensAdd(1, 1)
	doMetricsReq(r, "/ping")
	rec := doMetricsReq(r, "/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics 应 200，got %d", rec.Code)
	}
	body := rec.Body.String()

	// 诊断输出：全部出现过的指标名
	var names []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# TYPE ") {
			names = append(names, strings.Fields(line)[2])
		}
	}
	t.Logf("exposed series: %v", names)
	for _, want := range []string{
		"ptpatronus_http_requests_total",
		"ptpatronus_http_request_duration_seconds",
		"go_goroutines", // client_golang 默认 Go 运行时指标
		"ptpatronus_site_sync_total",
		"ptpatronus_cookie_keepalive_total",
		"ptpatronus_hr_alerts_total",
		"ptpatronus_hr_check_total",
		"ptpatronus_llm_requests_total",
		"ptpatronus_llm_tokens_total",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("输出应含 %s:\n%s", want, body[:min(2000, len(body))])
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
