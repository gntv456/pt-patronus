package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/core/log"
)

// setupSystemServer 构造 SystemHandler 路由（update=nil：更新检查走禁用分支）。
// 版本脱敏、日志读写走真实环形缓冲。
func setupSystemServer(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := NewSystemHandler(nil, nil)
	r := gin.New()
	r.GET("/health", h.Health)
	r.GET("/info", h.Info)
	r.GET("/logs", h.Logs)
	r.POST("/logs/clear", h.ClearLogs)
	return r
}

func doSys(t *testing.T, r *gin.Engine, method, path string, authed bool) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	if authed {
		// 模拟 JWTAuth 已注入 uid（走标准管线，包一层注入中间件）
		rr := gin.New()
		rr.Use(func(c *gin.Context) { c.Set("uid", uint(1)); c.Next() })
		rr.Any("/*p", func(c *gin.Context) { r.ServeHTTP(c.Writer, c.Request) })
		rr.ServeHTTP(w, req)
		return w
	}
	r.ServeHTTP(w, req)
	return w
}

// TestSystem_Health 公共探针恒 200（Docker healthcheck 依赖）。
func TestSystem_Health(t *testing.T) {
	r := setupSystemServer(t)
	rec := doSys(t, r, http.MethodGet, "/health", false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "ok") {
		t.Fatalf("health 应 200 ok: %d %s", rec.Code, rec.Body.String())
	}
}

// TestSystem_InfoVersionGating 版本号只对已认证请求返回（未认证指纹风险）。
func TestSystem_InfoVersionGating(t *testing.T) {
	r := setupSystemServer(t)

	rec := doSys(t, r, http.MethodGet, "/info", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "version") {
		t.Fatalf("未认证不得回版本号: %s", rec.Body.String())
	}

	// 已认证（uid 注入）：带 version
	gin.SetMode(gin.TestMode)
	rr := gin.New()
	h := NewSystemHandler(nil, nil)
	rr.GET("/info", func(c *gin.Context) { c.Set("uid", uint(1)); c.Next() }, h.Info)
	rec2 := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/info", nil)
	rr.ServeHTTP(rec2, req)
	if !strings.Contains(rec2.Body.String(), "version") {
		t.Fatalf("已认证应带版本: %s", rec2.Body.String())
	}
}

// TestSystem_LogsReadWriteClear 环形缓冲读（level/n 参数）与清空。
func TestSystem_LogsReadWriteClear(t *testing.T) {
	r := setupSystemServer(t)
	log.Ring.Push(log.LogEntry{Level: "INFO", Message: "normal-event"})
	log.Ring.Push(log.LogEntry{Level: "ERROR", Message: "boom-failure"})

	// 全量
	rec := doSys(t, r, http.MethodGet, "/logs", false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "boom-failure") {
		t.Fatalf("日志应含 ERROR 条目: %d %s", rec.Code, rec.Body.String())
	}
	// level 过滤：error 级只见 ERROR
	rec = doSys(t, r, http.MethodGet, "/logs?level=error", false)
	body := rec.Body.String()
	if strings.Contains(body, "normal-event") || !strings.Contains(body, "boom-failure") {
		t.Fatalf("level=error 过滤错误: %s", body)
	}
	// n 限流：n=1 只回最近 1 条
	rec = doSys(t, r, http.MethodGet, "/logs?n=1", false)
	if strings.Count(rec.Body.String(), "\"message\"") > 1 {
		t.Fatalf("n=1 应至多 1 条: %s", rec.Body.String())
	}

	// 清空后为空数组（非 null）
	rec = doSys(t, r, http.MethodPost, "/logs/clear", false)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "true") {
		t.Fatalf("清空应回 true: %s", rec.Body.String())
	}
	rec = doSys(t, r, http.MethodGet, "/logs", false)
	if !strings.Contains(rec.Body.String(), `"entries":[]`) {
		t.Fatalf("清空后应为空数组: %s", rec.Body.String())
	}
}

// TestSystem_UpdateCheckDisabled update 服务未装配 → hasUpdate=false（不 500）。
func TestSystem_UpdateCheckDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSystemHandler(nil, nil)
	r := gin.New()
	r.GET("/update/check", h.UpdateCheck)
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/update/check?platform=docker", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"hasUpdate":false`) {
		t.Fatalf("禁用态应 hasUpdate=false: %d %s", rec.Code, rec.Body.String())
	}
}
