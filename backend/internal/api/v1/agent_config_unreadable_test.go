package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/aiagent"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/core/security"
)

// spyKV 既能模拟「读不出来」，也能证明「读不出来时一次都没写」。
// 真库的 SystemConfigOper.Get 把「键不存在」归一成 ("", nil)，所以这里的 err 只代表真故障。
type spyKV struct {
	val     string
	readErr error
	writes  int
}

func (k *spyKV) Get(string) (string, error) {
	if k.readErr != nil {
		return "", k.readErr
	}
	return k.val, nil
}
func (k *spyKV) Set(_, value string) error {
	k.writes++
	k.val = value
	return nil
}

func newAgentCfgRouter(kv aiagent.KVStore, cr aiagent.Crypto) *gin.Engine {
	h := NewAgentChatHandler(nil, nil, nil, nil, nil, nil, nil, kv, cr)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/agent/config", h.GetConfig)
	r.PUT("/agent/config", h.SetConfig)
	return r
}

func storedConfigJSON(t *testing.T, enabled bool, keyEnc string) string {
	t.Helper()
	raw, err := json.Marshal(aiagent.Config{
		Enabled: enabled, Provider: "openai", BaseURL: "https://api.openai.com/v1",
		Model: "gpt-4o-mini", Temperature: 0.3, MaxTokens: 2048, MaxIterations: 12,
		SystemPrompt: "你是助手", APIKeyEnc: keyEnc,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

const validSetBody = `{"enabled":true,"provider":"openai","base_url":"https://api.openai.com/v1",` +
	`"model":"gpt-4o-mini","temperature":0.3,"max_tokens":2048,"enable_thinking":false,` +
	`"max_iterations":12,"system_prompt":"你是助手","enabled_tools":[]}`

// TestAgentConfigOutletsRefuseUnreadable #271 的出口层红证：
// 从前 GET 回 200 + 默认值 + api_key_configured:false（面板显示「还没配过」），
// PUT 更坏——合并基准也是这份默认值，用户不填 key 点一次保存就把真 key 抹掉。
func TestAgentConfigOutletsRefuseUnreadable(t *testing.T) {
	cr := security.NewCrypto("master")

	t.Run("GET 读不出来不能说成没配过", func(t *testing.T) {
		kv := &spyKV{val: storedConfigJSON(t, true, "enc:sk-real"), readErr: errors.New("database is locked")}
		rec := httptest.NewRecorder()
		newAgentCfgRouter(kv, cr).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/agent/config", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d want 503，body=%q", rec.Code, rec.Body.String())
		}
		var env struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("响应不是 JSON：%q", rec.Body.String())
		}
		if env.Code != errcode.AgentConfigUnreadable.Code {
			t.Errorf("code=%q want %q", env.Code, errcode.AgentConfigUnreadable.Code)
		}
		if !strings.Contains(env.Message, "读不出来") || !strings.Contains(env.Message, "覆盖") {
			t.Errorf("文案要同时说清处境与「先别保存」：%q", env.Message)
		}
	})

	t.Run("PUT 读不出来时绝不写入", func(t *testing.T) {
		original := storedConfigJSON(t, true, "enc:sk-real")
		kv := &spyKV{val: original, readErr: errors.New("database is locked")}
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/agent/config", strings.NewReader(validSetBody))
		req.Header.Set("Content-Type", "application/json")
		newAgentCfgRouter(kv, cr).ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d want 503，body=%q", rec.Code, rec.Body.String())
		}
		if kv.writes != 0 {
			t.Errorf("读不出来时一次都不该写，writes=%d", kv.writes)
		}
		if kv.val != original || !strings.Contains(kv.val, "enc:sk-real") {
			t.Errorf("库里的配置被改动了：原=%q 现=%q", original, kv.val)
		}
	})

	// 正控：读得出来时，GET 如实回 configured=true；PUT 不填 key 也保留原 key。
	t.Run("正控：健康配置照常可读写", func(t *testing.T) {
		kv := &spyKV{val: storedConfigJSON(t, true, "enc:sk-real")}
		rec := httptest.NewRecorder()
		newAgentCfgRouter(kv, cr).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/agent/config", nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"api_key_configured":true`) {
			t.Fatalf("GET 应回 200 且 configured=true：%d %q", rec.Code, rec.Body.String())
		}

		rec = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/agent/config", strings.NewReader(validSetBody))
		req.Header.Set("Content-Type", "application/json")
		newAgentCfgRouter(kv, cr).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT=%d body=%q", rec.Code, rec.Body.String())
		}
		if !strings.Contains(kv.val, "enc:sk-real") {
			t.Errorf("不填 key 的保存应保留原 key，现值=%q", kv.val)
		}
	})
}
