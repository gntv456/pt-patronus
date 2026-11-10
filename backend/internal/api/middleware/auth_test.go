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

// withAdmin 注入身份（模拟 JWTAuth 的 c.Set），isAdmin 控制 is_admin 标记。
func withAdmin(isAdmin bool, via string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("uid", uint(1))
		c.Set("username", "tester")
		c.Set("auth_via", via)
		c.Set("is_admin", isAdmin)
		c.Next()
	}
}

func TestRequireAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	okHandler := func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) }

	cases := []struct {
		name       string
		isAdmin    bool
		wantStatus int
	}{
		{"admin allowed", true, http.StatusOK},
		{"non-admin denied", false, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/", withAdmin(tc.isAdmin, "jwt"), RequireAdmin(), okHandler)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", w.Code, tc.wantStatus, w.Body.String())
			}
		})
	}

	// 未注入 is_admin（不应出现，但 RequireAdmin 必须兜底拒绝而非 panic）
	t.Run("missing identity denied", func(t *testing.T) {
		r := gin.New()
		r.GET("/", RequireAdmin(), okHandler)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403 (body=%s)", w.Code, w.Body.String())
		}
	})

	// API-Key 旁路：JWTAuth 把 is_admin 置 true，必须放行。
	t.Run("apikey bypass allowed", func(t *testing.T) {
		r := gin.New()
		r.GET("/", withAdmin(true, "apikey"), RequireAdmin(), okHandler)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
		}
	})
}

// TestJWTAuth_CookieFallback 无 Authorization 头时必须回退读 ptp_token cookie
// （Web 端 httpOnly 会话通道）；cookie 无效返回 401。
func TestJWTAuth_CookieFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/mwauth.db"), &gorm.Config{})
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
	if err := db.Create(&models.User{Username: "u", HxptUID: "u", TokenVersion: 0, VipLevel: 1}).Error; err != nil {
		t.Fatal(err)
	}
	j := security.NewJWT("test-secret", 60)
	tv := auth.NewTokenVersionCache(db, time.Minute)
	mw := JWTAuth(j, "", tv)

	// 签发一个合法 token，经 cookie 注入应认证成功
	var uid uint
	db.Raw("SELECT id FROM users ORDER BY id LIMIT 1").Scan(&uid)
	tok, err := j.Issue(uid, "u", false, 0, 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v1/x", nil)
	c.Request.AddCookie(&http.Cookie{Name: "ptp_token", Value: tok})
	mw(c)
	if _, ok := c.Get("uid"); !ok {
		t.Fatal("cookie token should authenticate")
	}
	if v, _ := c.Get("auth_via"); v != "jwt" {
		t.Fatalf("auth_via should be jwt, got %v", v)
	}

	// 无效 cookie → 401 中止（经完整 engine 验证写入状态码）
	r := gin.New()
	r.GET("/x", JWTAuth(j, "", tv), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	req := httptest.NewRequest("GET", "/x", nil)
	req.AddCookie(&http.Cookie{Name: "ptp_token", Value: "garbage"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("garbage cookie should 401, got %d", w.Code)
	}
}

// TestJWTAuth_SkipCookiePaths WS 等 ticket 鉴权路径必须跳过 cookie 通道：
// 浏览器 WebSocket 升级请求无法省略自动携带的 ptp_token cookie，也无法补
// Authorization 头。残留的过期 cookie 若在此路径上抢先判 401，会挡在路由自身
// 的 ticket 校验之前，导致实时推送永久停摆（前端只能 3s 循环重连 401）。
func TestJWTAuth_SkipCookiePaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	j := security.NewJWT("test-secret", 60)
	mw := JWTAuth(j, "", nil, "/api/v1/ws")

	req := httptest.NewRequest("GET", "/api/v1/ws?ticket=whatever", nil)
	req.AddCookie(&http.Cookie{Name: "ptp_token", Value: "stale-garbage"})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	mw(c)
	if c.IsAborted() {
		t.Fatal("stale cookie must not abort on skip-cookie path (ticket route owns auth)")
	}
	if _, ok := c.Get("uid"); ok {
		t.Fatal("garbage cookie must not authenticate either")
	}

	// 非 skip 路径保持原行为：脏 cookie 依旧 401
	r := gin.New()
	r.GET("/x", JWTAuth(j, "", nil, "/api/v1/ws"), func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	req2 := httptest.NewRequest("GET", "/x", nil)
	req2.AddCookie(&http.Cookie{Name: "ptp_token", Value: "stale-garbage"})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req2)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("stale cookie off skip-path should 401, got %d", w.Code)
	}
}
