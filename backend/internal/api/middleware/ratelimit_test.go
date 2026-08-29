package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRateLimiterBurst(t *testing.T) {
	l := NewRateLimiter(1, 2) // 突发 2
	if !l.Allow("1.2.3.4") {
		t.Error("first request should pass")
	}
	if !l.Allow("1.2.3.4") {
		t.Error("second request (within burst) should pass")
	}
	if l.Allow("1.2.3.4") {
		t.Error("third request should be rate-limited")
	}
}

func TestRateLimiterPerKey(t *testing.T) {
	l := NewRateLimiter(1, 1)
	if !l.Allow("a") {
		t.Error("a should pass")
	}
	if l.Allow("a") {
		t.Error("a second should be limited")
	}
	// 不同 key 独立
	if !l.Allow("b") {
		t.Error("b should pass independently")
	}
}

// ── Middleware() HTTP 集成 ────────────────────────────────────────

// newRateTestServer 构造挂限流中间件的引擎（模拟生产 router 形态：/api/* 受限流，
// 静态路径直通）。remoteAddr 经 c.ClientIP() 取值，gin 默认取 RemoteAddr。
func newRateTestServer(rps float64, burst int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	l := NewRateLimiter(rps, burst)
	r.Use(l.Middleware())
	r.GET("/api/v1/ping", func(c *gin.Context) { c.String(200, "pong") })
	r.GET("/assets/app.js", func(c *gin.Context) { c.String(200, "js") })
	return r
}

func doReq(r *gin.Engine, path, remoteAddr string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = remoteAddr
	r.ServeHTTP(w, req)
	return w
}

// TestRateLimiterMiddleware_ApiLimited burst 打满后 /api/* 返回 429（防暴力破解路径）。
func TestRateLimiterMiddleware_ApiLimited(t *testing.T) {
	r := newRateTestServer(1, 2)
	for i := 0; i < 2; i++ {
		if rec := doReq(r, "/api/v1/ping", "10.0.0.1:1234"); rec.Code != 200 {
			t.Fatalf("burst 内第 %d 个请求应 200，got %d", i+1, rec.Code)
		}
	}
	if rec := doReq(r, "/api/v1/ping", "10.0.0.1:1234"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("超 burst 应 429，got %d", rec.Code)
	}
	if !strings.Contains(rec429Body(t, r), "rate limit") {
		t.Fatal("429 响应应带语义化错误")
	}
}

func rec429Body(t *testing.T, r *gin.Engine) string {
	t.Helper()
	return doReq(r, "/api/v1/ping", "10.0.0.1:1234").Body.String()
}

// TestRateLimiterMiddleware_StaticAssetsExempt 静态资源（前端 chunk）不计入令牌桶：
// 刷新页面并发拉 ~30 个 chunk 不应 429（SPA 白板回归防线）。
func TestRateLimiterMiddleware_StaticAssetsExempt(t *testing.T) {
	r := newRateTestServer(1, 1) // 极小桶：API 第二发就 429
	for i := 0; i < 30; i++ {
		if rec := doReq(r, "/assets/app.js", "10.0.0.2:1234"); rec.Code != 200 {
			t.Fatalf("静态资源第 %d 个请求应 200（不受限流），got %d", i+1, rec.Code)
		}
	}
	// 同 IP 的 API 仍受限流约束（豁免只对非 /api/ 路径）
	if rec := doReq(r, "/api/v1/ping", "10.0.0.2:1234"); rec.Code != 200 {
		t.Fatalf("首个 API 请求应 200，got %d", rec.Code)
	}
	if rec := doReq(r, "/api/v1/ping", "10.0.0.2:1234"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("第二个 API 请求应 429（桶=1），got %d", rec.Code)
	}
}

// TestRateLimiterMiddleware_PerIP 同一限流器对不同 IP 独立计数。
func TestRateLimiterMiddleware_PerIP(t *testing.T) {
	r := newRateTestServer(1, 1)
	if rec := doReq(r, "/api/v1/ping", "10.0.0.3:1"); rec.Code != 200 {
		t.Fatalf("A 首请求应 200，got %d", rec.Code)
	}
	if rec := doReq(r, "/api/v1/ping", "10.0.0.4:1"); rec.Code != 200 {
		t.Fatalf("B 首请求应独立 200，got %d", rec.Code)
	}
}

// TestRateLimiterMiddleware_InvalidArgs 非法 rps/burst 归一为 1（不 panic）。
func TestRateLimiterMiddleware_InvalidArgs(t *testing.T) {
	r := newRateTestServer(0, 0)
	if rec := doReq(r, "/api/v1/ping", "10.0.0.5:1"); rec.Code != 200 {
		t.Fatalf("归一后首请求应 200，got %d", rec.Code)
	}
}
