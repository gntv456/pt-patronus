// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// setupAuthDB 构造内存 SQLite + User 表，供 AuthHandler 回归测试。
func setupAuthDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/auth_admin.db"), &gorm.Config{})
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
	return db
}

// TestIssueForOIDC_FirstUserAdmin 首个 OIDC 用户成为 admin（与 README 声明一致；
// 原 hxpt 账号登录同款语义，改版下线后由 OIDC 路径承接回归）。
func TestIssueForOIDC_FirstUserAdmin(t *testing.T) {
	db := setupAuthDB(t)
	h := &AuthHandler{db: db, jwt: security.NewJWT("auth-test-secret", 60)}

	if _, err := h.issueForOIDC("oidc-001", "alice"); err != nil {
		t.Fatalf("issueForOIDC: %v", err)
	}
	var u models.User
	db.First(&u, "hxpt_uid = ?", "oidc-001")
	if !u.IsAdmin {
		t.Fatal("首个 OIDC 用户应为 admin")
	}
}

// TestIssueForOIDC_SecondUserRejected 第二个真实身份不得自助建号（单用户硬限制回归）：
// 返回 auth_user_rejected，不落库（HTTP 403 映射在 OIDC handler 层）。
func TestIssueForOIDC_SecondUserRejected(t *testing.T) {
	db := setupAuthDB(t)
	h := &AuthHandler{db: db, jwt: security.NewJWT("auth-test-secret", 60)}

	if _, err := h.issueForOIDC("oidc-001", "alice"); err != nil {
		t.Fatalf("首个用户: %v", err)
	}
	_, err := h.issueForOIDC("oidc-002", "bob")
	if !errors.Is(err, errcode.AuthUserRejected) {
		t.Fatalf("第二个身份应被单用户限制拒绝（auth_user_rejected），got %v", err)
	}
	var n int64
	db.Model(&models.User{}).Where("hxpt_uid = ?", "oidc-002").Count(&n)
	if n != 0 {
		t.Fatal("被拒的第二用户不得落库")
	}
}

// TestIssueForOIDC_ReLoginNoEscalation 已有非 admin 用户重登录不得升权（P0-1 回归）。
func TestIssueForOIDC_ReLoginNoEscalation(t *testing.T) {
	db := setupAuthDB(t)
	u := models.User{Username: "carol", HxptUID: "oidc-003", IsAdmin: false}
	db.Create(&u)

	h := &AuthHandler{db: db, jwt: security.NewJWT("auth-test-secret", 60)}

	if _, err := h.issueForOIDC("oidc-003", "carol"); err != nil {
		t.Fatalf("issueForOIDC: %v", err)
	}
	var refreshed models.User
	db.First(&refreshed, "hxpt_uid = ?", "oidc-003")
	if refreshed.IsAdmin {
		t.Fatal("非 admin 用户重登录不得被升权（P0-1 回归）")
	}
}

// TestExistingRealUserUIDExcludesSpecial demo/emergency/oidc 不占用「实例主人」名额：
// 单用户硬限制依据 existingRealUserUID 的排除逻辑判定。
func TestExistingRealUserUIDExcludesSpecial(t *testing.T) {
	db := setupAuthDB(t)
	h := &AuthHandler{db: db}

	if uid, err := h.existingRealUserUID(); err != nil || uid != "" {
		t.Fatalf("空库应无主人，got uid=%q err=%v", uid, err)
	}
	db.Create(&models.User{Username: "demo", HxptUID: "demo", IsAdmin: false})
	db.Create(&models.User{Username: "oidc-alice", HxptUID: "oidc:alice", IsAdmin: true})
	db.Create(&models.User{Username: "emergency-ops", HxptUID: "emergency:ops", IsAdmin: true})
	if uid, err := h.existingRealUserUID(); err != nil || uid != "" {
		t.Fatalf("demo/emergency/oidc 在库时仍应视为无主人，got uid=%q err=%v", uid, err)
	}
	db.Create(&models.User{Username: "real", HxptUID: "local:real", IsAdmin: true})
	if uid, err := h.existingRealUserUID(); err != nil || uid != "local:real" {
		t.Fatalf("真实用户存在时应返回其 uid，got uid=%q err=%v", uid, err)
	}
}

// TestFirstUserGatesFailClosedOnQueryError 「查不出来」绝不能被当成「还没有主人」：
// 那会同时打开单用户硬限制并把这次登录者提成 admin（DB 抖动即可提权）。
func TestFirstUserGatesFailClosedOnQueryError(t *testing.T) {
	db := setupAuthDB(t)
	db.Create(&models.User{Username: "owner", HxptUID: "local:owner", IsAdmin: true})
	if err := db.Migrator().DropTable(&models.User{}); err != nil {
		t.Fatalf("drop users: %v", err)
	}
	h := &AuthHandler{db: db}

	if _, err := h.existingRealUserUID(); err == nil {
		t.Fatal("表已不存在：existingRealUserUID 必须报错")
	}
	if first, err := h.firstOIDCUser(); err == nil || first {
		t.Fatalf("firstOIDCUser 必须 fail-closed，got first=%v err=%v", first, err)
	}
}

// TestExistingRealUserUIDNoOwnerIsNotAnError 正控：表在但一个真实用户都没有 → ("", nil)，
// 首位登录者仍可成为 admin（否则新实例装不起来）。
func TestExistingRealUserUIDNoOwnerIsNotAnError(t *testing.T) {
	db := setupAuthDB(t)
	h := &AuthHandler{db: db}

	uid, err := h.existingRealUserUID()
	if err != nil {
		t.Fatalf("确实没有主人不应报错: %v", err)
	}
	if uid != "" {
		t.Fatalf("空库应返回空 UID，got %q", uid)
	}
}

// ---- 用户偏好（跨浏览器同步）回归 ----

// setupPrefEnv 构造带偏好路由的测试服务器 + 登录上下文。
func setupPrefEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := setupAuthDB(t)
	if err := db.AutoMigrate(&models.UserPreference{}); err != nil {
		t.Fatal(err)
	}
	db.Create(&models.User{Username: "u1", HxptUID: "hxpt-001"})
	h := &AuthHandler{db: db}
	r := gin.New()
	r.GET("/auth/preferences/:key", func(c *gin.Context) {
		c.Set("uid", uint(1))
		h.GetPreferences(c)
	})
	r.PUT("/auth/preferences/:key", func(c *gin.Context) {
		c.Set("uid", uint(1))
		h.SetPreferences(c)
	})
	return r, db
}

func TestUserPreferencesRoundTrip(t *testing.T) {
	r, db := setupPrefEnv(t)

	// 未保存过：返回空对象
	rec := doReq(t, r, http.MethodGet, "/auth/preferences/ui", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"value":"{}"`) {
		t.Fatalf("empty get: code=%d body=%s", rec.Code, rec.Body.String())
	}

	// 保存（合法 JSON 域内覆盖）
	rec = doReq(t, r, http.MethodPut, "/auth/preferences/ui", `{"value":"{\"accent\":\"azure\"}"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put: code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doReq(t, r, http.MethodGet, "/auth/preferences/ui", "")
	if !strings.Contains(rec.Body.String(), "azure") {
		t.Fatalf("round trip lost value: %s", rec.Body.String())
	}

	// 再保存覆盖为 rose
	doReq(t, r, http.MethodPut, "/auth/preferences/ui", `{"value":"{\"accent\":\"rose\"}"}`)
	rec = doReq(t, r, http.MethodGet, "/auth/preferences/ui", "")
	if !strings.Contains(rec.Body.String(), "rose") || strings.Contains(rec.Body.String(), "azure") {
		t.Fatalf("overwrite failed: %s", rec.Body.String())
	}
	_ = db
}

func TestUserPreferencesRejectsInvalid(t *testing.T) {
	r, _ := setupPrefEnv(t)
	// 非 JSON 值拒绝
	if rec := doReq(t, r, http.MethodPut, "/auth/preferences/ui", `{"value":"not-json"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid json should 400, got %d", rec.Code)
	}
	// 超 64KB 拒绝
	big := strings.Repeat("a", 70*1024)
	if rec := doReq(t, r, http.MethodPut, "/auth/preferences/ui", `{"value":"`+big+`"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized value should 400, got %d", rec.Code)
	}
}
