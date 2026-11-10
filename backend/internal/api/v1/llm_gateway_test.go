// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/llmgateway"
)

// memKV 内存 KV（实现 llmgateway.KVStore），隔离 SystemConfig 依赖。
type memKV struct{ m map[string]string }

func (k *memKV) Get(key string) (string, error) { return k.m[key], nil }
func (k *memKV) Set(key, v string) error        { k.m[key] = v; return nil }

// setupLLMServer 构造 LLM 网关路由（agent=nil：授权通过后由 agentReady 拦 503，
// 授权/配置路径全走真实逻辑——常量时间比较、脱敏回显、令牌加密落库）。
func setupLLMServer(t *testing.T, enabled bool, token string) (*gin.Engine, *memKV, *security.Crypto) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	kv := &memKV{m: map[string]string{}}
	cr := security.NewCrypto("llm-test-seed")
	cfg := llmgateway.DefaultConfig()
	cfg.Enabled = enabled
	if token != "" {
		enc, _ := cr.Encrypt(token)
		cfg.AccessTokenEnc = enc
	}
	if err := llmgateway.Save(kv, cfg); err != nil {
		t.Fatal(err)
	}
	h := NewLLMGatewayHandler(nil, nil, kv, cr)
	r := gin.New()
	r.POST("/openai/v1/chat/completions", h.OpenAIChat)
	r.POST("/anthropic/v1/messages", h.AnthropicMessages)
	r.GET("/llm-gateway/config", h.GetConfig)
	r.PUT("/llm-gateway/config", h.SetConfig)
	return r, kv, cr
}

func doLLM(t *testing.T, r *gin.Engine, method, path, body, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req, _ = http.NewRequest(method, path, nil)
	} else {
		req, _ = http.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestLLMGateway_Disabled 未启用 → 503（OpenAI 与 Anthropic 同语义）。
func TestLLMGateway_Disabled(t *testing.T) {
	r, _, _ := setupLLMServer(t, false, "")
	rec := doLLM(t, r, http.MethodPost, "/openai/v1/chat/completions", `{}`, "tok")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("未启用应 503，got %d", rec.Code)
	}
	rec = doLLM(t, r, http.MethodPost, "/anthropic/v1/messages", `{}`, "tok")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("anthropic 未启用应 503，got %d", rec.Code)
	}
}

// TestLLMGateway_BadToken 令牌错误/缺失 → 401（常量时间比较路径）。
func TestLLMGateway_BadToken(t *testing.T) {
	r, _, _ := setupLLMServer(t, true, "correct-token")
	if rec := doLLM(t, r, http.MethodPost, "/openai/v1/chat/completions", `{}`, "wrong"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("错 token 应 401，got %d", rec.Code)
	}
	if rec := doLLM(t, r, http.MethodPost, "/openai/v1/chat/completions", `{}`, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("缺 token 应 401，got %d", rec.Code)
	}
}

// TestLLMGateway_AuthorizedButNoAgent 授权通过 + agent 未配 → 503（不进入流式）。
func TestLLMGateway_AuthorizedButNoAgent(t *testing.T) {
	r, _, _ := setupLLMServer(t, true, "correct-token")
	rec := doLLM(t, r, http.MethodPost, "/openai/v1/chat/completions", `{}`, "correct-token")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("agent 未配应 503，got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "AI agent") {
		t.Fatalf("错误信息应指向 agent 未启用: %s", rec.Body.String())
	}
}

// TestLLMGateway_ConfigRW 配置读写：令牌只写不读（脱敏）、令牌空=保留。
func TestLLMGateway_ConfigRW(t *testing.T) {
	r, kv, _ := setupLLMServer(t, false, "")

	// 默认读回脱敏（无 access_token 明文）
	rec := doLLM(t, r, http.MethodGet, "/llm-gateway/config", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "access_token") && strings.Contains(rec.Body.String(), "plain") {
		t.Fatalf("配置读取不得带明文 token: %s", rec.Body.String())
	}

	// 写入：enable + token
	rec = doLLM(t, r, http.MethodPut, "/llm-gateway/config",
		`{"enabled":true,"allowed_models":["gpt-4o"],"max_concurrency":2,"access_token":"secret-tok"}`, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("写入失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	cfg := llmgateway.Load(kv)
	if !cfg.Enabled || cfg.MaxConcurrency != 2 {
		t.Fatalf("配置应已生效: %+v", cfg)
	}
	if cfg.AccessTokenEnc == "" || cfg.AccessTokenEnc == "secret-tok" {
		t.Fatalf("token 应加密落库: %q", cfg.AccessTokenEnc)
	}

	// 非法 JSON → 400
	if rec = doLLM(t, r, http.MethodPut, "/llm-gateway/config", `{bad`, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 JSON 应 400，got %d", rec.Code)
	}
}

// TestLLMGateway_TokenFrom 令牌提取：Bearer 优先、x-api-key 兜底。
func TestLLMGateway_TokenFrom(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &LLMGatewayHandler{}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/x", nil)
	c.Request.Header.Set("Authorization", "Bearer abc")
	if got := h.tokenFrom(c); got != "abc" {
		t.Fatalf("Bearer 提取错误: %q", got)
	}

	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest(http.MethodPost, "/x", nil)
	c2.Request.Header.Set("x-api-key", "sk-xyz")
	if got := h.tokenFrom(c2); got != "sk-xyz" {
		t.Fatalf("x-api-key 提取错误: %q", got)
	}
}

// TestLLMGateway_SemFor 并发信号量按容量重建（非法值归一）。
func TestLLMGateway_SemFor(t *testing.T) {
	h := &LLMGatewayHandler{}
	if ch := h.semFor(0); cap(ch) != 4 {
		t.Fatalf("n=0 应归一 4，got %d", cap(ch))
	}
	if ch := h.semFor(8); cap(ch) != 8 {
		t.Fatalf("应建容量 8，got %d", cap(ch))
	}
	// 同容量复用（不重建）
	ch1 := h.semFor(8)
	ch2 := h.semFor(8)
	if ch1 != ch2 {
		t.Fatal("同容量应复用同一信号量")
	}
}
