// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// 三种结局不许压成同一句「登录态已更新，请重新登录」：
//  1. 版本号确实不符 → 该重新登录（旧行为，保留）；
//  2. 账号已不在库里 → 也该重新登录，但要留痕；
//  3. 读不出来（表没了、库繁忙）→ 会话本身没被判错，这句错话会把用户推去
//     白登一次并怀疑账号坏了，且服务端一条日志都没有。
func TestJWTAuth_SeparatesUnreadableTokenVersionFromMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	j := security.NewJWT("test-secret", 60)

	// —— 3) 读不出来 → 503，且不许出现「已更新」那句 ——
	bad, err := gorm.Open(sqlite.Open(t.TempDir()+"/notables.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := bad.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	tok, err := j.Issue(1, "u", false, 0, 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/guarded", JWTAuth(j, "", auth.NewTokenVersionCache(bad, time.Minute)),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	req := httptest.NewRequest("GET", "/guarded", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("读不出来该是 503（会话本身没被判错），实得 %d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "已更新") {
		t.Errorf("把读库失败说成「登录态已更新」：%s", w.Body.String())
	}

	// —— 2) 账号不在库里 → 仍是 401 请重新登录 ——
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/gone.db"), &gorm.Config{})
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
	u := models.User{Username: "u2", HxptUID: "u2", TokenVersion: 0, VipLevel: 1}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	tok2, err := j.Issue(u.ID, "u2", false, 0, 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&models.User{}, u.ID).Error; err != nil {
		t.Fatal(err)
	}
	r2 := gin.New()
	r2.GET("/guarded", JWTAuth(j, "", auth.NewTokenVersionCache(db, time.Minute)),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	req2 := httptest.NewRequest("GET", "/guarded", nil)
	req2.Header.Set("Authorization", "Bearer "+tok2)
	w2 := httptest.NewRecorder()
	r2.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized || !strings.Contains(w2.Body.String(), "token_version_mismatch") {
		t.Fatalf("账号已不存在应判「请重新登录」，实得 %d body=%s", w2.Code, w2.Body.String())
	}

	// —— 正控：哨兵确实分得开两种 err ——
	if _, e := auth.NewTokenVersionCache(db, time.Minute).Get(u.ID); e == nil {
		t.Fatal("删掉的账号应返回错误（否则上面两条断言都可能恒真）")
	} else if !strings.Contains(e.Error(), "user row missing") {
		t.Errorf("缺账号应包 ErrUserMissing 哨兵，实得 %v", e)
	}
}

// 判据留痕不许只到「回了什么」：拒绝理由必须有一条 Warn 落在日志语句里，
// 否则运维只看界面永远不知道那次拒绝其实是库读不出。
func TestJWTAuth_RejectionsAreLoggedWithReason(t *testing.T) {
	srcBytes, err := os.ReadFile("auth.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(srcBytes)
	for _, want := range []string{
		`slog.Warn("auth rejected: token version unreadable`,
		`slog.Warn("auth rejected: token points at a missing user`,
		"auth.ErrUserMissing",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("auth.go 里缺少 %s（拒绝不能只留一句错话给界面）", want)
		}
	}
	// 正控：同族旧形状（一个 if 吞两种错）必须被这条判据拒掉
	oldShape := "cur, err := tvCache.Get(claims.UserID)\nif err != nil || cur != claims.TokenVersion {"
	if strings.Contains(oldShape, `slog.Warn("auth rejected: token version unreadable`) {
		t.Error("判据本身失效（旧形状也被算作已留痕）")
	}
}
