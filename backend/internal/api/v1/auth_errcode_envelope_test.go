// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

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
	"testing"

	"github.com/gin-gonic/gin"
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

// TestEnvelope_AuthBadCredentials 密码错 → 401 + auth_bad_credentials 信封
// （原 hxpt 账号登录用例的等价迁移：本地登录错误口径一致）。
func TestEnvelope_AuthBadCredentials(t *testing.T) {
	r, db, _, _ := newAuthTestEnv(t, false)
	hash, err := security.HashPassword("rightpass123")
	if err != nil {
		t.Fatal(err)
	}
	db.Create(&models.User{Username: "alice", PasswordHash: hash, HxptUID: "local:alice"})

	rec := postJSON(t, r, "/auth/local/login", `{"username":"alice","password":"wrongpass123"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("应 401，got %d", rec.Code)
	}
	assertEnvelope(t, rec, "auth_bad_credentials")
}

// TestEnvelope_AuthCaptchaFailed / AuthCookieRequired / AuthServiceUnavailable
// 已随旧站点登录链路（验证码/cookie 登录）下线删除。

// TestEnvelope_DemoDisabled 演示登录未启用 → 404 + demo_disabled。
func TestEnvelope_DemoDisabled(t *testing.T) {
	r, _, _, _ := newAuthTestEnv(t, false)

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
