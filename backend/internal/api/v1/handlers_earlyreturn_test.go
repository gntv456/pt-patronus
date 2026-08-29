package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// handler 早返回路径：构造 nil/最小依赖 handler，命中 400/守卫分支，触达 chain/DB 前即返回。
// 覆盖 SystemHandler(nil update)、Subscribe 手动必填校验、CommandRun 4 道早返回门、
// Media Recommend nil 优雅降级、Auth hxpt 未启用 503。

func init() { gin.SetMode(gin.TestMode) }

func doReq(t *testing.T, r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rd *strings.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	var req *http.Request
	if rd != nil {
		req = httptest.NewRequest(method, path, rd)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func bodyJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON body %q: %v", rec.Body.String(), err)
	}
	return m
}

func TestSystemHandlerNilUpdate(t *testing.T) {
	h := NewSystemHandler(nil, nil)
	r := gin.New()
	r.GET("/health", h.Health)
	r.GET("/info", h.Info)
	r.GET("/update", h.UpdateCheck)
	r.POST("/manifest", h.SetManifest)

	t.Run("health always ok", func(t *testing.T) {
		rec := doReq(t, r, http.MethodGet, "/health", "")
		if rec.Code != http.StatusOK {
			t.Errorf("code = %d, want 200", rec.Code)
		}
		if m := bodyJSON(t, rec); m["status"] != "ok" {
			t.Errorf("status = %v", m["status"])
		}
	})

	t.Run("info unauthed omits version", func(t *testing.T) {
		rec := doReq(t, r, http.MethodGet, "/info", "")
		if rec.Code != http.StatusOK {
			t.Errorf("code = %d, want 200", rec.Code)
		}
		m := bodyJSON(t, rec)
		if m["name"] != "PTPatronus" {
			t.Errorf("name = %v", m["name"])
		}
		if _, has := m["version"]; has {
			t.Error("unauthed info must not expose version")
		}
	})

	t.Run("update check nil returns hasUpdate false", func(t *testing.T) {
		rec := doReq(t, r, http.MethodGet, "/update", "")
		if rec.Code != http.StatusOK {
			t.Errorf("code = %d, want 200", rec.Code)
		}
		if m := bodyJSON(t, rec); m["hasUpdate"] != false {
			t.Errorf("hasUpdate = %v, want false", m["hasUpdate"])
		}
	})

	t.Run("set manifest nil returns 503", func(t *testing.T) {
		rec := doReq(t, r, http.MethodPost, "/manifest", `{"latest_version":"1.0"}`)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("code = %d, want 503", rec.Code)
		}
	})
}

func TestSubscribeHandlerCreateValidation(t *testing.T) {
	h := NewSubscribeHandler(nil) // subCh=nil：手动必填校验在触达 chain 前
	r := gin.New()
	r.POST("/sub", h.Create)

	t.Run("missing title/source/source_id rejected", func(t *testing.T) {
		rec := doReq(t, r, http.MethodPost, "/sub", `{}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d, want 400", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "required") {
			t.Errorf("body = %q, want required hint", rec.Body.String())
		}
	})

	t.Run("malformed json rejected", func(t *testing.T) {
		rec := doReq(t, r, http.MethodPost, "/sub", `{not json`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d, want 400", rec.Code)
		}
	})
}

func TestWorkflowHandlerCreateBadJSON(t *testing.T) {
	h := NewWorkflowHandler(nil)
	r := gin.New()
	r.POST("/wf", h.Create)
	rec := doReq(t, r, http.MethodPost, "/wf", `{bad`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

func TestWebhookHandlerCreateBadJSON(t *testing.T) {
	h := NewWebhookHandler(nil, nil, nil, nil)
	r := gin.New()
	r.POST("/wh", h.Create)
	rec := doReq(t, r, http.MethodPost, "/wh", `{bad`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", rec.Code)
	}
}

// TestCommandRunHandlerCreateGates 4 道早返回门（均在触达 runs/targets/svc 前）。
func TestCommandRunHandlerCreateGates(t *testing.T) {
	h := NewCommandRunHandler(nil, nil, nil)
	r := gin.New()
	r.POST("/run", h.Create)

	t.Run("malformed json", func(t *testing.T) {
		rec := doReq(t, r, http.MethodPost, "/run", `{bad`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d, want 400", rec.Code)
		}
	})

	t.Run("empty command", func(t *testing.T) {
		rec := doReq(t, r, http.MethodPost, "/run", `{"target_ids":[1],"command":""}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d, want 400", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "field_required") {
			t.Errorf("body = %q", rec.Body.String())
		}
	})

	t.Run("empty target_ids", func(t *testing.T) {
		rec := doReq(t, r, http.MethodPost, "/run", `{"target_ids":[],"command":"ls"}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d, want 400", rec.Code)
		}
	})

	t.Run("zero target filtered to empty", func(t *testing.T) {
		// target_ids=[0] 经 uniqueTargetIDs 去零 → 空 → 400
		rec := doReq(t, r, http.MethodPost, "/run", `{"target_ids":[0,0],"command":"ls"}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d, want 400", rec.Code)
		}
	})

	t.Run("too many targets", func(t *testing.T) {
		ids := make([]string, 0, 51)
		for i := 1; i <= 51; i++ {
			ids = append(ids, strconv.Itoa(i))
		}
		body := `{"target_ids":[` + strings.Join(ids, ",") + `],"command":"ls"}`
		rec := doReq(t, r, http.MethodPost, "/run", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d, want 400", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "invalid_request") {
			t.Errorf("body = %q", rec.Body.String())
		}
	})

	t.Run("high risk without confirmation", func(t *testing.T) {
		rec := doReq(t, r, http.MethodPost, "/run", `{"target_ids":[1],"command":"rm -rf /","confirm_risk":false}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("code = %d, want 400", rec.Code)
		}
		m := bodyJSON(t, rec)
		if m["code"] != "risk_confirmation_required" {
			t.Errorf("code = %v, want risk_confirmation_required", m["code"])
		}
		if m["risk_level"] != "high" {
			t.Errorf("risk_level = %v, want high", m["risk_level"])
		}
	})
}

func TestMediaHandlerRecommendNilGraceful(t *testing.T) {
	h := NewMediaHandler(nil, nil) // recommend=nil → 优雅返空
	r := gin.New()
	r.GET("/rec", h.Recommend)
	rec := doReq(t, r, http.MethodGet, "/rec", "")
	if rec.Code != http.StatusOK {
		t.Errorf("code = %d, want 200", rec.Code)
	}
	m := bodyJSON(t, rec)
	res, ok := m["results"].([]any)
	if !ok || len(res) != 0 {
		t.Errorf("results = %v, want empty array", m["results"])
	}
}

func TestAuthHandlerHxptDisabled(t *testing.T) {
	h := &AuthHandler{} // hxptAuth=nil → 登录服务未启用
	r := gin.New()
	r.GET("/captcha", h.HxptCaptcha)
	rec := doReq(t, r, http.MethodGet, "/captcha", "")
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("code = %d, want 503", rec.Code)
	}
}
