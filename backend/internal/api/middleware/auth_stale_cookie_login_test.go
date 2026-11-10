// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// 残留的脏会话 cookie 不许把「登录」这一步自己挡住（真机复现：删掉数据目录后浏览器
// 仍带着上一次的 httpOnly ptp_token，POST /auth/demo-login 直接 401
// "token_version_mismatch / 登录态已更新，请重新登录"——用户正在点登录，却被告知要重新登录，
// 而 httpOnly cookie 在界面上清不掉）。
// 同一条 cookie 走在受保护路由上仍必须 401（fail-closed 不变，只是登录入口放行）。
func TestJWTAuth_SkipsStaleCookieOnLoginEntries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/stale.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatal(err)
	}
	u := models.User{Username: "u", HxptUID: "u", TokenVersion: 0, VipLevel: 1}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	j := security.NewJWT("test-secret", 60)
	tv := auth.NewTokenVersionCache(db, time.Minute)
	tok, err := j.Issue(u.ID, "u", false, 7, 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// token 里 tv=7 而库里是 0 ⇒ 任何带这条 cookie 的请求都该被判定为失效会话
	skip := "/api/v1/auth/demo-login"
	mw := JWTAuth(j, "", tv, skip)

	r := gin.New()
	passed := false
	h := func(c *gin.Context) { passed = true; c.JSON(http.StatusOK, gin.H{"ok": true}) }
	r.POST(skip, mw, h)
	r.GET("/api/v1/sites", mw, h)

	req := httptest.NewRequest("POST", skip, nil)
	req.AddCookie(&http.Cookie{Name: "ptp_token", Value: tok})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !passed {
		t.Fatalf("登录入口不该被残留 cookie 抢先拒绝：code=%d passed=%v body=%s", w.Code, passed, w.Body.String())
	}

	// 反证（同一条脏 cookie）：受保护路由必须仍然 401，且不能把业务 handler 放行
	passed = false
	req2 := httptest.NewRequest("GET", "/api/v1/sites", nil)
	req2.AddCookie(&http.Cookie{Name: "ptp_token", Value: tok})
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("脏 cookie 在受保护路由上必须 401，实得 %d（放行=鉴权被绕过）", w2.Code)
	}
	if passed {
		t.Error("受保护路由的业务 handler 不该被执行")
	}
}
