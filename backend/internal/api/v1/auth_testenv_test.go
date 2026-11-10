// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

// 鉴权域测试公共骨架：AuthHandler + 生产形态公开鉴权路由 + JSON 请求 helper。
// （原随好学登录测试；站点改版下线旧登录链路后保留通用骨架，供本地登录/
// demo/续期/信封回归共用。）

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/api/middleware"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// newAuthTestEnv 构造 AuthHandler + 公开鉴权路由（对齐生产路由形态：
// 建会话入口跳过 ptp_token cookie 通道，JWTAuth 注入身份供 refresh 复用）。
func newAuthTestEnv(t *testing.T, demoMode bool) (
	*gin.Engine, *gorm.DB, *security.JWT, *AuthHandler,
) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/auth.db"), &gorm.Config{})
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
	j := security.NewJWT("auth-login-test-secret", 60)
	tv := auth.NewTokenVersionCache(db, time.Minute)
	h := &AuthHandler{db: db, jwt: j, tvCache: tv, demoMode: demoMode}
	r := gin.New()
	r.Use(middleware.JWTAuth(j, "", tv, "/auth/local/login", "/auth/demo-login"))
	r.POST("/auth/setup", h.Setup)
	r.POST("/auth/login", h.Login)
	r.POST("/auth/local/login", h.LocalLogin)
	r.POST("/auth/demo-login", h.DemoLogin)
	r.POST("/auth/refresh", h.Refresh)
	return r, db, j, h
}

// postJSON 发 JSON POST（无鉴权头）。
func postJSON(
	t *testing.T, r *gin.Engine, path, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// getJSON 发 GET（无鉴权头）。
