package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// ── 测试替身 ─────────────────────────────────────────────────────

// fakeHxptAuth 站内可控的 HxptAuthenticator：按预设返回值驱动登录/校验分支。
type fakeHxptAuth struct {
	captcha    *site.CaptchaChallenge
	captchaErr error
	outcome    *auth.LoginOutcome
	loginErr   error
	vip        *auth.VIPStatus
}

func (f *fakeHxptAuth) FetchCaptcha(ctx context.Context) (*site.CaptchaChallenge, error) {
	return f.captcha, f.captchaErr
}
func (f *fakeHxptAuth) Login(ctx context.Context, creds site.LoginCredentials, ch *site.CaptchaChallenge, imagestring, twoStepCode string) (*auth.LoginOutcome, error) {
	return f.outcome, f.loginErr
}
func (f *fakeHxptAuth) Verify(ctx context.Context, cookie string) *auth.VIPStatus { return f.vip }

// newAuthTestEnv 构造 AuthHandler + 真实路由（JWTAuth→Refresh 等），SQLite 内存落地。
func newAuthTestEnv(t *testing.T, fake auth.HxptAuthenticator, demoMode bool) (*gin.Engine, *gorm.DB, *security.JWT, *AuthHandler) {
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
	h := NewAuthHandler(db, j, nil, fake, security.NewCrypto("seed"), tv, auth.NewTicketStore(), demoMode, nil)
	r := gin.New()
	r.POST("/auth/login", h.HxptLogin)
	r.POST("/auth/cookie-login", h.HxptCookieLogin)
	r.POST("/auth/demo-login", h.DemoLogin)
	r.GET("/auth/captcha", h.HxptCaptcha)
	return r, db, j, h
}

func postJSON(t *testing.T, r *gin.Engine, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func getJSON(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ── HxptLogin（账号密码两步登录） ────────────────────────────────

// TestHxptLogin_Success 全链路成功：签发 token、用户落库带 VIP 快照、首用户成为管理员。
func TestHxptLogin_Success(t *testing.T) {
	fake := &fakeHxptAuth{outcome: &auth.LoginOutcome{
		HxptUID: "u-100",
		Cookie:  "c=ok",
		VIP:     &auth.VIPStatus{Verified: true, ID: "u-100", Username: "alice", Level: "APP专属VIP"},
	}}
	r, db, _, _ := newAuthTestEnv(t, fake, false)

	rec := postJSON(t, r, "/auth/login", `{"username":"alice","password":"pw"}`)
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
	var u models.User
	if err := db.Where("hxpt_uid = ?", "u-100").First(&u).Error; err != nil {
		t.Fatalf("用户应落库: %v", err)
	}
	if u.VipVerifiedAt == nil || u.HxptCookieEnc == "" {
		t.Fatal("VIP 快照与加密 cookie 应落库")
	}
	if !u.IsAdmin {
		t.Fatal("首个 VIP 用户应为管理员")
	}
}

// TestHxptLogin_WrongCredentials 凭据/验证码错误 → 401，语义化错误不泄露内部细节。
func TestHxptLogin_WrongCredentials(t *testing.T) {
	fake := &fakeHxptAuth{loginErr: site.ErrLoginFailed}
	r, _, _, _ := newAuthTestEnv(t, fake, false)

	rec := postJSON(t, r, "/auth/login", `{"username":"x","password":"y"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("凭据错误应 401，got %d", rec.Code)
	}
}

// TestHxptLogin_ChallengeMode 好学挑战响应 → 409 引导 Cookie 登录。
func TestHxptLogin_ChallengeMode(t *testing.T) {
	fake := &fakeHxptAuth{loginErr: site.ErrChallengeMode}
	r, _, _, _ := newAuthTestEnv(t, fake, false)

	rec := postJSON(t, r, "/auth/login", `{"username":"x","password":"y"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("挑战模式应 409，got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "challenge_mode") {
		t.Fatalf("应带 code=challenge_mode，body=%s", rec.Body.String())
	}
}

// TestHxptLogin_NotVIP 登录成功但非 APP 专属 VIP → 403 拒绝（准入闸门）。
func TestHxptLogin_NotVIP(t *testing.T) {
	fake := &fakeHxptAuth{outcome: &auth.LoginOutcome{
		HxptUID: "u-200",
		VIP:     &auth.VIPStatus{Verified: false, Message: "普通用户"},
	}}
	r, db, _, _ := newAuthTestEnv(t, fake, false)

	rec := postJSON(t, r, "/auth/login", `{"username":"bob","password":"pw"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("非 VIP 应 403，got %d", rec.Code)
	}
	var cnt int64
	db.Model(&models.User{}).Count(&cnt)
	if cnt != 0 {
		t.Fatalf("非 VIP 不得落库，got %d 行", cnt)
	}
}

// TestHxptLogin_BadJSON 请求体非法 → 400。
func TestHxptLogin_BadJSON(t *testing.T) {
	r, _, _, _ := newAuthTestEnv(t, &fakeHxptAuth{}, false)
	rec := postJSON(t, r, "/auth/login", `{not-json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法 JSON 应 400，got %d", rec.Code)
	}
}

// ── HxptCookieLogin（Cookie 兜底登录） ───────────────────────────

// TestHxptCookieLogin_Success cookie 验证通过 → 登录成功（2FA/挑战账号的主路径）。
func TestHxptCookieLogin_Success(t *testing.T) {
	fake := &fakeHxptAuth{vip: &auth.VIPStatus{Verified: true, ID: "u-300", Username: "carol"}}
	r, db, _, _ := newAuthTestEnv(t, fake, false)

	rec := postJSON(t, r, "/auth/cookie-login", `{"cookie":"uid=1;pass=ok"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var u models.User
	if err := db.Where("hxpt_uid = ?", "u-300").First(&u).Error; err != nil {
		t.Fatalf("用户应落库: %v", err)
	}
}

// TestHxptCookieLogin_ReusedUID_UsernameBackfill uid 解析失败回退 username 作外部身份。
func TestHxptCookieLogin_ReusedUID_UsernameBackfill(t *testing.T) {
	fake := &fakeHxptAuth{vip: &auth.VIPStatus{Verified: true, Username: "dave"}}
	r, db, _, _ := newAuthTestEnv(t, fake, false)

	rec := postJSON(t, r, "/auth/cookie-login", `{"cookie":"x=1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var u models.User
	if err := db.Where("hxpt_uid = ?", "dave").First(&u).Error; err != nil {
		t.Fatalf("uid 空时应回退 username 作 hxpt_uid: %v", err)
	}
}

// TestHxptCookieLogin_InvalidCookie cookie 无效 → 403 not_vip。
func TestHxptCookieLogin_InvalidCookie(t *testing.T) {
	fake := &fakeHxptAuth{vip: &auth.VIPStatus{Verified: false, Message: "cookie 失效"}}
	r, _, _, _ := newAuthTestEnv(t, fake, false)

	rec := postJSON(t, r, "/auth/cookie-login", `{"cookie":"bad"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("无效 cookie 应 403，got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not_vip") {
		t.Fatalf("应带 code=not_vip，body=%s", rec.Body.String())
	}
}

// TestHxptCookieLogin_EmptyCookie 缺 cookie 字段 → 400。
func TestHxptCookieLogin_EmptyCookie(t *testing.T) {
	r, _, _, _ := newAuthTestEnv(t, &fakeHxptAuth{}, false)
	rec := postJSON(t, r, "/auth/cookie-login", `{"cookie":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("空 cookie 应 400，got %d", rec.Code)
	}
}

// TestHxptCookieLogin_ServiceUnavailable 登录服务未装配 → 503。
func TestHxptCookieLogin_ServiceUnavailable(t *testing.T) {
	r, _, _, _ := newAuthTestEnv(t, nil, false)
	rec := postJSON(t, r, "/auth/cookie-login", `{"cookie":"x"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("未启用应 503，got %d", rec.Code)
	}
}

// ── DemoLogin（演示入口） ────────────────────────────────────────

// TestDemoLogin_DisabledByDefault 默认关闭 → 404（不暴露入口）。
func TestDemoLogin_DisabledByDefault(t *testing.T) {
	r, _, _, _ := newAuthTestEnv(t, nil, false)
	rec := postJSON(t, r, "/auth/demo-login", `{}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("未启用演示模式应 404，got %d", rec.Code)
	}
}

// TestDemoLogin_EnabledCreatesNonAdmin 显式开启：创建 VIP 演示用户且非管理员。
func TestDemoLogin_EnabledCreatesNonAdmin(t *testing.T) {
	r, db, _, _ := newAuthTestEnv(t, nil, true)

	rec := postJSON(t, r, "/auth/demo-login", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var u models.User
	if err := db.Where("hxpt_uid = ?", "demo").First(&u).Error; err != nil {
		t.Fatalf("演示用户应创建: %v", err)
	}
	if u.IsAdmin {
		t.Fatal("演示用户不得是管理员（防误触高破坏操作）")
	}
	if u.VipVerifiedAt == nil {
		t.Fatal("演示用户应带 VIP 快照")
	}

	// 第二次登录走既有用户更新分支（Save 路径）。
	rec2 := postJSON(t, r, "/auth/demo-login", `{}`)
	if rec2.Code != http.StatusOK {
		t.Fatalf("二次登录 code=%d body=%s", rec2.Code, rec2.Body.String())
	}
	var cnt int64
	db.Model(&models.User{}).Where("hxpt_uid = ?", "demo").Count(&cnt)
	if cnt != 1 {
		t.Fatalf("演示用户应复用不重复创建，got %d", cnt)
	}
}

// ── HxptCaptcha（Step A） ────────────────────────────────────────

// TestHxptCaptcha_ServiceUnavailable 未装配 → 503。
func TestHxptCaptcha_ServiceUnavailable(t *testing.T) {
	r, _, _, _ := newAuthTestEnv(t, nil, false)
	rec := getJSON(t, r, "/auth/captcha")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("未启用应 503，got %d", rec.Code)
	}
}

// TestHxptCaptcha_FetchFail 上游失败 → 502 带原因。
func TestHxptCaptcha_FetchFail(t *testing.T) {
	fake := &fakeHxptAuth{captchaErr: context.DeadlineExceeded}
	r, _, _, _ := newAuthTestEnv(t, fake, false)
	rec := getJSON(t, r, "/auth/captcha")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("上游失败应 502，got %d", rec.Code)
	}
}

// TestHxptCaptcha_OK 正常返回字段齐整（imagehash/image/challenge_mode）。
func TestHxptCaptcha_OK(t *testing.T) {
	fake := &fakeHxptAuth{captcha: &site.CaptchaChallenge{ImageHash: "h1", ImageB64: "b64", NeedCaptcha: true}}
	r, _, _, _ := newAuthTestEnv(t, fake, false)
	rec := getJSON(t, r, "/auth/captcha")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	for _, k := range []string{"imagehash", "image", "need_captcha", "challenge_mode"} {
		if _, ok := body[k]; !ok {
			t.Fatalf("响应缺字段 %s: %v", k, body)
		}
	}
}

// ── WsTicket（一次性 ticket 签发） ───────────────────────────────

// TestWsTicket_Issue 正常签发：ticket 非空且带有效期。
func TestWsTicket_Issue(t *testing.T) {
	_, db, _, h := newAuthTestEnv(t, nil, false)
	u := models.User{Username: "wsu"}
	db.Create(&u)

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request, _ = http.NewRequest(http.MethodGet, "/ws-ticket", nil)
	c.Set("uid", u.ID)
	h.WsTicket(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["ticket"] == "" || body["ticket"] == nil {
		t.Fatalf("应签发 ticket: %v", body)
	}
	if expiresIn, _ := body["expires_in"].(float64); expiresIn <= 0 {
		t.Fatalf("应带正的 expires_in: %v", body)
	}
}
