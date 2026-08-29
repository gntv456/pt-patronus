package v1

import (
	"encoding/json"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/site"
)

// PR-2 信封断言：认证域用户路径错误必须带 {code, message, error} 三字段，
// error 与 message 同值（过渡兼容老客户端只读 error 的行为）。
// 覆盖 auth.go / auth_oidc.go / webhook.go 迁移到 errcode 的全部响应形态。

// assertEnvelope 解析响应体并断言三字段齐全且 error==message。
func assertEnvelope(t *testing.T, rec *httptest.ResponseRecorder, wantCode string) string {
	t.Helper()
	var body struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("信封非 JSON: %v\n%s", err, rec.Body.String())
	}
	if body.Code != wantCode {
		t.Fatalf("code=%q, want %q\n%s", body.Code, wantCode, rec.Body.String())
	}
	if body.Message == "" {
		t.Fatalf("message 不应为空:\n%s", rec.Body.String())
	}
	if body.Error != body.Message {
		t.Fatalf("error(%q) 应与 message(%q) 同值", body.Error, body.Message)
	}
	return body.Message
}

// TestEnvelope_AuthBadCredentials 凭据错误 → 401 + auth_bad_credentials 信封。
func TestEnvelope_AuthBadCredentials(t *testing.T) {
	fake := &fakeHxptAuth{loginErr: site.ErrLoginFailed}
	r, _, _, _ := newAuthTestEnv(t, fake, false)

	rec := postJSON(t, r, "/auth/login", `{"username":"x","password":"y"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("应 401，got %d", rec.Code)
	}
	assertEnvelope(t, rec, "auth_bad_credentials")
}

// TestEnvelope_AuthCaptchaFailed 取验证码失败 → 502 + auth_captcha_failed，
// 底层细节不进响应体（WithErr 只留日志链）。
func TestEnvelope_AuthCaptchaFailed(t *testing.T) {
	fake := &fakeHxptAuth{captchaErr: site.ErrLoginFailed}
	r, _, _, _ := newAuthTestEnv(t, fake, false)

	rec := getJSON(t, r, "/auth/captcha")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("应 502，got %d", rec.Code)
	}
	msg := assertEnvelope(t, rec, "auth_captcha_failed")
	if strings.Contains(msg, site.ErrLoginFailed.Error()) {
		t.Fatalf("底层错误细节不应进 message: %q", msg)
	}
}

// TestEnvelope_AuthCookieRequired cookie 登录缺参 → 400 + auth_cookie_required。
func TestEnvelope_AuthCookieRequired(t *testing.T) {
	r, _, _, _ := newAuthTestEnv(t, &fakeHxptAuth{}, false)

	rec := postJSON(t, r, "/auth/cookie-login", `{"cookie":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("应 400，got %d", rec.Code)
	}
	assertEnvelope(t, rec, "auth_cookie_required")
}

// TestEnvelope_AuthServiceUnavailable 登录服务未装配（hxptAuth=nil）→ 503 + auth_service_unavailable。
func TestEnvelope_AuthServiceUnavailable(t *testing.T) {
	r, _, _, _ := newAuthTestEnv(t, nil, false)

	rec := postJSON(t, r, "/auth/login", `{"username":"x","password":"y"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("应 503，got %d", rec.Code)
	}
	assertEnvelope(t, rec, "auth_service_unavailable")
}

// TestEnvelope_DemoDisabled 演示登录未启用 → 404 + demo_disabled。
func TestEnvelope_DemoDisabled(t *testing.T) {
	r, _, _, _ := newAuthTestEnv(t, &fakeHxptAuth{}, false)

	rec := postJSON(t, r, "/auth/demo-login", `{}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("应 404，got %d", rec.Code)
	}
	assertEnvelope(t, rec, "demo_disabled")
}

// TestEnvelope_WebhookInvalid 无效 webhook → 404 + webhook_invalid。
// Receive 为公开端点（无 JWT）；用真实 SQLite oper 打 token 未命中分支（nil oper 会 panic）。
func TestEnvelope_WebhookInvalid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/webhook.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Webhook{}); err != nil {
		t.Fatal(err)
	}
	h := NewWebhookHandler(oper.NewWebhookOper(db, security.NewCrypto("wh-test-seed")), nil, nil, nil)
	r := gin.New()
	r.POST("/webhooks/:token", h.Receive)
	rec := postJSON(t, r, "/webhooks/nonexistent-token", `{}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("应 404，got %d", rec.Code)
	}
	assertEnvelope(t, rec, "webhook_invalid")
}
