package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
