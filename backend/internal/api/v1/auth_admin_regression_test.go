package v1

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
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

// TestIssueForHxpt_FirstUserAdmin 首个 HXPT 用户成为 admin（与 README 声明一致）。
func TestIssueForHxpt_FirstUserAdmin(t *testing.T) {
	db := setupAuthDB(t)
	h := &AuthHandler{db: db, crypto: security.NewCrypto("auth-test-key"), jwt: security.NewJWT("auth-test-secret", 60)}
	r := gin.New()
	r.POST("/login", func(c *gin.Context) { h.issueForHxpt(c, "hxpt-001", "alice", "cookie1") })

	rec := doReq(t, r, http.MethodPost, "/login", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var u models.User
	db.First(&u, "hxpt_uid = ?", "hxpt-001")
	if !u.IsAdmin {
		t.Fatal("首个 HXPT 用户应为 admin")
	}
}

// TestIssueForHxpt_SecondUserNotAdmin 第二个 HXPT 用户不得升权（P0-1 回归）。
func TestIssueForHxpt_SecondUserNotAdmin(t *testing.T) {
	db := setupAuthDB(t)
	h := &AuthHandler{db: db, crypto: security.NewCrypto("auth-test-key"), jwt: security.NewJWT("auth-test-secret", 60)}
	r := gin.New()
	r.POST("/login", func(c *gin.Context) { h.issueForHxpt(c, c.Query("uid"), c.Query("name"), "cookie") })

	doReq(t, r, http.MethodPost, "/login?uid=hxpt-001&name=alice", "")
	doReq(t, r, http.MethodPost, "/login?uid=hxpt-002&name=bob", "")

	var bob models.User
	db.First(&bob, "hxpt_uid = ?", "hxpt-002")
	if bob.IsAdmin {
		t.Fatal("第二个 HXPT 用户不应为 admin（P0-1 回归）")
	}
}

// TestIssueForHxpt_ReLoginNoEscalation 已有非 admin 用户重登录不得升权（P0-1 回归）。
func TestIssueForHxpt_ReLoginNoEscalation(t *testing.T) {
	db := setupAuthDB(t)
	u := models.User{Username: "carol", HxptUID: "hxpt-003", IsAdmin: false}
	db.Create(&u)

	h := &AuthHandler{db: db, crypto: security.NewCrypto("auth-test-key"), jwt: security.NewJWT("auth-test-secret", 60)}
	r := gin.New()
	r.POST("/login", func(c *gin.Context) { h.issueForHxpt(c, "hxpt-003", "carol", "new-cookie") })

	rec := doReq(t, r, http.MethodPost, "/login", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var refreshed models.User
	db.First(&refreshed, "hxpt_uid = ?", "hxpt-003")
	if refreshed.IsAdmin {
		t.Fatal("非 admin 用户重登录不得被升权（P0-1 回归）")
	}
}

// TestFirstHxptUser 验证排除逻辑：emergency/demo/oidc 前缀不计入 HXPT 首用户判定。
func TestFirstHxptUser(t *testing.T) {
	db := setupAuthDB(t)
	h := &AuthHandler{db: db}

	if !h.firstHxptUser() {
		t.Fatal("空 DB 应回报首用户")
	}
	db.Create(&models.User{Username: "oidc-alice", HxptUID: "oidc:alice", IsAdmin: true})
	if !h.firstHxptUser() {
		t.Fatal("仅有 OIDC 用户时仍应为首 HXPT 用户")
	}
	db.Create(&models.User{Username: "emergency-bob", HxptUID: "emergency:bob"})
	if !h.firstHxptUser() {
		t.Fatal("有 emergency 用户时仍应为首 HXPT 用户")
	}
	db.Create(&models.User{Username: "hxpt-user-001", HxptUID: "hxpt-001"})
	if h.firstHxptUser() {
		t.Fatal("已有 HXPT 用户后不应再报首用户")
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
