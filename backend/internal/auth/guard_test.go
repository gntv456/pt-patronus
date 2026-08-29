package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// setupVIPGuard 构造挂 RequireVIP 的路由 + SQLite。身份注入中间件（inject）挂在
// 同一引擎的 RequireVIP 之前，模拟生产 JWTAuth 注入 uid/vip_verified_at/auth_via。
func setupVIPGuard(t *testing.T, graceDays int, inject gin.HandlerFunc) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/guard.db"), &gorm.Config{})
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
	r := gin.New()
	if inject != nil {
		r.Use(inject)
	}
	r.GET("/vip", RequireVIP(db, graceDays), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r, db
}

// reqVIP 走标准 ServeHTTP 管线（HandleContext/nested engine 均会重建 context 丢键）。
func reqVIP(t *testing.T, r *gin.Engine) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/vip", nil)
	r.ServeHTTP(rec, req)
	return rec
}

// TestRequireVIP_APIKeyBypass API-Key 旁路（程序化访问）必须放行——不查库不判宽限。
func TestRequireVIP_APIKeyBypass(t *testing.T) {
	r, _ := setupVIPGuard(t, 3, func(c *gin.Context) { c.Set("auth_via", "apikey") })
	rec := reqVIP(t, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("API-Key 应放行，got %d", rec.Code)
	}
}

// TestRequireVIP_ClaimsSnapshotWithinGrace JWT claims 快照在宽限期内 → 放行（不查库）。
func TestRequireVIP_ClaimsSnapshotWithinGrace(t *testing.T) {
	ts := time.Now().Add(-1 * time.Hour).Unix() // 1h 前 < 3d 宽限
	r, _ := setupVIPGuard(t, 3, func(c *gin.Context) { c.Set("vip_verified_at", ts) })
	rec := reqVIP(t, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("宽限内快照应放行，got %d", rec.Code)
	}
}

// TestRequireVIP_ClaimsSnapshotExpired claims 快照过期且无 uid → 403 vip_not_verified。
func TestRequireVIP_ClaimsSnapshotExpired(t *testing.T) {
	ts := time.Now().Add(-30 * 24 * time.Hour).Unix() // 30d 前 > 3d 宽限
	r, _ := setupVIPGuard(t, 3, func(c *gin.Context) { c.Set("vip_verified_at", ts) })
	rec := reqVIP(t, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("过期快照应 403，got %d", rec.Code)
	}
}

// TestRequireVIP_NoIdentity 匿名（无 uid 无快照）→ 403 引导登录。
func TestRequireVIP_NoIdentity(t *testing.T) {
	r, _ := setupVIPGuard(t, 3, nil)
	rec := reqVIP(t, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("匿名应 403，got %d", rec.Code)
	}
}

// TestRequireVIP_UserTableWithinGrace claims 落空、但 User 表 VipVerifiedAt 在宽限内 → 放行。
// 注意两次 setupVIPGuard 各建独立 SQLite 库——第二次的库无用户，须在同一 db 上建注入路由。
func TestRequireVIP_UserTableWithinGrace(t *testing.T) {
	vipAt := time.Now().Add(-2 * 24 * time.Hour) // 2d 前 < 3d
	u := models.User{Username: "g1", VipVerifiedAt: &vipAt}
	r, db := setupVIPGuard(t, 3, nil)
	db.Create(&u)
	// 同库追加带 uid 注入的路由（/vip2 与 /vip 同 guard 实例语义）。
	r.GET("/vip2", func(c *gin.Context) { c.Set("uid", u.ID); c.Next() }, RequireVIP(db, 3), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/vip2", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("User 表宽限内应放行，got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestRequireVIP_UserNeverVerified User 表无 VIP 记录 → 403 vip_not_verified。
func TestRequireVIP_UserNeverVerified(t *testing.T) {
	r, db := setupVIPGuard(t, 3, nil)
	u := models.User{Username: "g2"}
	db.Create(&u)
	r.GET("/vip2", func(c *gin.Context) { c.Set("uid", u.ID); c.Next() }, RequireVIP(db, 3), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/vip2", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("未验证 VIP 应 403，got %d", rec.Code)
	}
}

// TestRequireVIP_UserVIPExpired User 表 VIP 超宽限 → 403 vip_expired（需重新验证）。
func TestRequireVIP_UserVIPExpired(t *testing.T) {
	r, db := setupVIPGuard(t, 3, nil)
	vipAt := time.Now().Add(-10 * 24 * time.Hour) // 10d 前 > 3d
	u := models.User{Username: "g3", VipVerifiedAt: &vipAt}
	db.Create(&u)
	r.GET("/vip2", func(c *gin.Context) { c.Set("uid", u.ID); c.Next() }, RequireVIP(db, 3), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/vip2", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("VIP 过期应 403，got %d", rec.Code)
	}
}

// TestRequireVIP_UserMissing 用户已被删除（查库 404）→ 403 vip_unknown。
func TestRequireVIP_UserMissing(t *testing.T) {
	r, _ := setupVIPGuard(t, 3, func(c *gin.Context) { c.Set("uid", uint(9999)) })
	rec := reqVIP(t, r)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("用户缺失应 403，got %d", rec.Code)
	}
}
