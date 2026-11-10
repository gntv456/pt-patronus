// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// setupRefreshServer 构造带 JWTAuth 链的 /refresh 测试服务器（对齐生产路由形态：
// 公开组挂 refresh，JWTAuth 先验签并注入身份）。
func setupRefreshServer(t *testing.T) (*gin.Engine, *gorm.DB, *security.JWT, *auth.TokenVersionCache) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/refresh.db"), &gorm.Config{})
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
	j := security.NewJWT("refresh-test-secret", 60)
	tv := auth.NewTokenVersionCache(db, time.Minute)
	h := &AuthHandler{db: db, jwt: j, tvCache: tv}
	r := gin.New()
	r.Use(middleware.JWTAuth(j, "", tv))
	r.POST("/refresh", h.Refresh)
	return r, db, j, tv
}

// TestRefresh_SlidingRenewal 有效 JWT 续期成功：新 token 可用且 claims 与 User 表同步。
func TestRefresh_SlidingRenewal(t *testing.T) {
	r, db, j, _ := setupRefreshServer(t)
	vipAt := time.Now().Add(-time.Hour)
	u := models.User{Username: "alice", HxptUID: "hxpt-001", IsAdmin: true, VipLevel: 1, VipVerifiedAt: &vipAt}
	db.Create(&u)
	tok, err := j.Issue(u.ID, u.Username, u.IsAdmin, u.TokenVersion, u.VipLevel, vipAt)
	if err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := doRefreshReq(t, r, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Token string      `json:"token"`
		User  models.User `json:"user"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Token == "" {
		t.Fatal("应签发 token")
	}
	if !resp.User.IsAdmin {
		t.Fatal("admin 快照应随续期保留")
	}
	claims, err := j.Parse(resp.Token)
	if err != nil || claims.UserID != u.ID {
		t.Fatalf("新 token 解析失败: %v", err)
	}
	// 注：不比较新旧 token 字符串——IssuedAt 秒级粒度下同秒重签字符串相同属正常。
}

// TestRefresh_AdminRevokedSync 收回 admin 后续期不得沿用旧快照（以 User 表真值重发）。
func TestRefresh_AdminRevokedSync(t *testing.T) {
	r, db, j, _ := setupRefreshServer(t)
	u := models.User{Username: "bob", HxptUID: "hxpt-002", IsAdmin: true}
	db.Create(&u)
	tok, _ := j.Issue(u.ID, u.Username, true, u.TokenVersion, 0, time.Time{})
	db.Model(&models.User{}).Where("id = ?", u.ID).UpdateColumn("is_admin", false)

	req, _ := http.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := doRefreshReq(t, r, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	claims, err := j.Parse(resp.Token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.IsAdmin {
		t.Fatal("admin 已收回，续期 token 不得保留 is_admin")
	}
}

// TestRefresh_RejectedAfterLogout 登出（TokenVersion bump）后旧 token 不得续期。
func TestRefresh_RejectedAfterLogout(t *testing.T) {
	r, db, j, tv := setupRefreshServer(t)
	u := models.User{Username: "carol", HxptUID: "hxpt-003"}
	db.Create(&u)
	tok, _ := j.Issue(u.ID, u.Username, false, u.TokenVersion, 0, time.Time{})
	db.Model(&models.User{}).Where("id = ?", u.ID).UpdateColumn("token_version", gorm.Expr("token_version + 1"))
	tv.Invalidate(u.ID)

	req, _ := http.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := doRefreshReq(t, r, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("登出后旧 token 续期应 401，got %d", rec.Code)
	}
}

// TestRefresh_ExpiredTokenRejected 过期 token 在 JWTAuth 即被拒（到不了 handler）。
// 用独立极短 TTL（0.0001 分钟 = 6ms）签发后等待过期。
func TestRefresh_ExpiredTokenRejected(t *testing.T) {
	r, db, _, _ := setupRefreshServer(t)
	u := models.User{Username: "dave", HxptUID: "hxpt-004"}
	db.Create(&u)
	short := security.NewJWT("refresh-test-secret", 0)
	// NewJWT(min=0) → expires=0；golang-jwt 对 ExpiresAt=now 的 token 判过期需过时钟，
	// 这里手动验证：签发即已到期的 token 必须被 401。
	tok, err := short.Issue(u.ID, u.Username, false, u.TokenVersion, 0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	req, _ := http.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := doRefreshReq(t, r, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("过期 token 续期应 401，got %d", rec.Code)
	}
}

// TestRefresh_NoIdentityRejected 无 Bearer（匿名）请求不得续期（JWTAuth 放行但 uid 缺失）。
func TestRefresh_NoIdentityRejected(t *testing.T) {
	r, _, _, _ := setupRefreshServer(t)
	rec := doRefreshReq(t, r, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("匿名续期应 400，got %d", rec.Code)
	}
}

func doRefreshReq(t *testing.T, r *gin.Engine, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	if req == nil {
		req, _ = http.NewRequest(http.MethodPost, "/refresh", nil)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
