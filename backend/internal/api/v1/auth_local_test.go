package v1

// 本地密码登录测试（好学站点关闭后的主登录路径，#<issue>）：
// 首登建号即 admin、单实例拒绝第二账号、存量未设密账号 409、
// 弱密码 400、改密端点旧密码校验。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/middleware"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// localEnv 复用 newAuthTestEnv（本地登录路由已挂）。
func localEnv(t *testing.T) (*gin.Engine, *gorm.DB, *AuthHandler) {
	r, db, _, h := newAuthTestEnv(t, false)
	return r, db, h
}

func TestLocalLogin_FirstUserBecomesAdmin(t *testing.T) {
	r, db, _ := localEnv(t)
	rec := postJSON(t, r, "/auth/local/login", `{"username":"owner","password":"abc12345"}`)
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
		t.Fatal("首位本地登录者应为 admin")
	}
	var u models.User
	if err := db.Where("username = ?", "owner").First(&u).Error; err != nil {
		t.Fatal("用户应落库")
	}
	if u.PasswordHash == "" {
		t.Fatal("密码哈希应落库")
	}
}

func TestLocalLogin_SecondUserRejected(t *testing.T) {
	r, _, _ := localEnv(t)
	postJSON(t, r, "/auth/local/login", `{"username":"owner","password":"abc12345"}`)
	rec := postJSON(t, r, "/auth/local/login", `{"username":"intruder","password":"xyz98765"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("单实例应拒绝第二账号，code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLocalLogin_BadPassword(t *testing.T) {
	r, _, _ := localEnv(t)
	postJSON(t, r, "/auth/local/login", `{"username":"owner","password":"abc12345"}`)
	rec := postJSON(t, r, "/auth/local/login", `{"username":"owner","password":"wrong999"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("错密码应 401，code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLocalLogin_LegacyUserWithoutPassword(t *testing.T) {
	// 存量好学账号（无 PasswordHash）不能凭任意口令登录。
	r, db, _ := localEnv(t)
	legacy := models.User{Username: "hxpt-user", HxptUID: "u-999", VipLevel: 1}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	rec := postJSON(t, r, "/auth/local/login", `{"username":"hxpt-user","password":"abc12345"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("未设密应 409，code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLocalLogin_WeakPasswordRejectedOnSignup(t *testing.T) {
	r, _, _ := localEnv(t)
	rec := postJSON(t, r, "/auth/local/login", `{"username":"owner","password":"short"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("弱密码建号应 400，code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestChangePassword_RequiresOldPassword(t *testing.T) {
	r, _, j, h := newAuthTestEnv(t, false)
	// 挂真实中间件链：JWTAuth（注入 uid）→ ChangePassword，对齐 guarded 语义。
	// （/auth/local/login 已由 newAuthTestEnv 挂载，这里只补 guarded 的改密路由。）
	tv := h.tvCache
	authed := r.Group("/auth/local", middleware.JWTAuth(j, "", tv))
	authed.POST("/password", h.ChangePassword)
	rec := postJSON(t, r, "/auth/local/login", `{"username":"owner","password":"abc12345"}`)
	var resp struct {
		Token string `json:"token"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)

	// 错旧密码 → 401；对旧密码 → 200；新密码可登录。
	req1 := postJSONAuth(t, r, "/auth/local/password", resp.Token,
		`{"old_password":"bad00000","new_password":"newpass99"}`)
	if req1.Code != http.StatusUnauthorized {
		t.Fatalf("错旧密码应 401，code=%d body=%s", req1.Code, req1.Body.String())
	}
	req2 := postJSONAuth(t, r, "/auth/local/password", resp.Token,
		`{"old_password":"abc12345","new_password":"newpass99"}`)
	if req2.Code != http.StatusOK {
		t.Fatalf("改密应 200，code=%d body=%s", req2.Code, req2.Body.String())
	}
	rec2 := postJSON(t, r, "/auth/local/login", `{"username":"owner","password":"newpass99"}`)
	if rec2.Code != http.StatusOK {
		t.Fatalf("新密码应可登录，code=%d body=%s", rec2.Code, rec2.Body.String())
	}
}

// 存量升级账号（本地密码为空）在「已登录」状态下可首次设密，无需旧密码、无需终端 CLI。
// 零终端升级的入口：guarded + 有效 JWT 已证明是实例主人，此时放宽旧密码校验是安全的。
func TestChangePassword_LegacyUserCanSetInitialPassword(t *testing.T) {
	r, database, j, h := newAuthTestEnv(t, false)
	authed := r.Group("/auth/local", middleware.JWTAuth(j, "", h.tvCache))
	authed.POST("/password", h.ChangePassword)

	rec := postJSON(t, r, "/auth/local/login", `{"username":"owner","password":"abc12345"}`)
	var resp struct {
		Token string `json:"token"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Token == "" {
		t.Fatal("拿不到登录 token")
	}
	// 模拟升级后的存量账号：密码为空（好学 VIP 时代遗留），但旧会话 JWT 仍有效。
	if err := database.Model(&models.User{}).Where("username = ?", "owner").Update("password_hash", "").Error; err != nil {
		t.Fatal(err)
	}
	setup := postJSONAuth(t, r, "/auth/local/password", resp.Token,
		`{"old_password":"","new_password":"Upgrade12345"}`)
	if setup.Code != http.StatusOK {
		t.Fatalf("存量账号首次设密应 200，code=%d body=%s", setup.Code, setup.Body.String())
	}
	login := postJSON(t, r, "/auth/local/login", `{"username":"owner","password":"Upgrade12345"}`)
	if login.Code != http.StatusOK {
		t.Fatalf("新密码应可登录，code=%d body=%s", login.Code, login.Body.String())
	}
}

// postJSONAuth 带 Bearer 头的 JSON POST（改密端点按 guarded 语义测试）。
func postJSONAuth(t *testing.T, r *gin.Engine, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
