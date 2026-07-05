package v1

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/middleware"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// audit 记录关键安全事件的审计日志（动作/结果/IP/请求ID）。绝不记录密码。
func audit(c *gin.Context, action, result string, extra ...any) {
	args := []any{
		slog.String("action", action),
		slog.String("result", result),
		slog.String("ip", c.ClientIP()),
		slog.String("req_id", middleware.RequestIDOf(c)),
	}
	args = append(args, extra...)
	slog.Info("audit", args...)
}

// dbFail 记录 DB 错误详情到日志，对客户端只回通用提示——
// 避免把 gorm 错误（表名/约束/SQL 片段）经 err.Error() 泄露给调用方。
func dbFail(c *gin.Context, action string, err error) {
	slog.Error("auth db error", "action", action, "err", err, "ip", c.ClientIP())
	c.JSON(http.StatusInternalServerError, gin.H{"error": "服务暂不可用，请稍后重试"})
}

// AuthHandler 处理好学账号/Cookie 登录、登出与（legacy）激活码。
type AuthHandler struct {
	db       *gorm.DB
	jwt      *security.JWT
	act      *auth.Activation        // legacy 激活码（不再作准入 gate，保留 Activate 端点）
	hxptAuth auth.HxptAuthenticator  // 好学登录 + VIP 校验
	crypto   *security.Crypto        // 加密 HxptCookieEnc
	tvCache  *auth.TokenVersionCache // 登出 invalidate
	tickets  *auth.TicketStore       // WS/RTSP 一次性 ticket（替代 query JWT）
	demoMode bool                    // 显式开启演示登录（默认关闭）
}

// NewAuthHandler 构造。
func NewAuthHandler(db *gorm.DB, jwt *security.JWT, act *auth.Activation,
	hxptAuth auth.HxptAuthenticator, crypto *security.Crypto, tvCache *auth.TokenVersionCache,
	tickets *auth.TicketStore, demoMode bool) *AuthHandler {
	return &AuthHandler{db: db, jwt: jwt, act: act, hxptAuth: hxptAuth, crypto: crypto, tvCache: tvCache, tickets: tickets, demoMode: demoMode}
}

// WsTicket 签发一次性 WS/RTSP ticket（guarded 内：header JWT 已校验，含 TokenVersion 失效校验）。
// 客户端用返回的 ticket 以 ?ticket= 连 WS/RTSP，避免 JWT 落 URL。
func (h *AuthHandler) WsTicket(c *gin.Context) {
	tk, exp, err := h.tickets.Issue(c.GetUint("uid"))
	if err != nil {
		audit(c, "ws_ticket", "fail", slog.String("err", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "签发 ticket 失败"})
		return
	}
	audit(c, "ws_ticket", "ok")
	c.JSON(http.StatusOK, gin.H{
		"ticket":     tk,
		"expires_in": int(time.Until(exp).Seconds()),
	})
}

// ===== 好学账号登录（验证码两步） =====

// HxptCaptcha 账号登录 Step A：取登录页验证码 / 探测模式。
func (h *AuthHandler) HxptCaptcha(c *gin.Context) {
	if h.hxptAuth == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "登录服务未启用"})
		return
	}
	ch, err := h.hxptAuth.FetchCaptcha(c.Request.Context())
	if err != nil {
		audit(c, "hxpt_captcha", "fail", slog.String("err", err.Error()))
		c.JSON(http.StatusBadGateway, gin.H{"error": "取验证码失败：" + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"imagehash":      ch.ImageHash,
		"image":          ch.ImageB64,
		"need_captcha":   ch.NeedCaptcha,
		"challenge_mode": ch.ChallengeMode,
		"extra_form":     ch.ExtraForm,
	})
}

type hxptLoginReq struct {
	Username      string            `json:"username"`
	Password      string            `json:"password"`
	ImageHash     string            `json:"imagehash"`
	ImageString   string            `json:"imagestring"`
	TwoStepCode   string            `json:"two_step_code"`
	ExtraForm     map[string]string `json:"extra_form"`
	ChallengeMode bool              `json:"challenge_mode"` // 由 captcha 步探得，回传以驱动挑战响应分支
}

// HxptLogin 账号登录 Step B：POST takelogin.php + VIP 校验。
func (h *AuthHandler) HxptLogin(c *gin.Context) {
	if h.hxptAuth == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "登录服务未启用"})
		return
	}
	var req hxptLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 应急兜底：好学 POST 链路坏掉时，运维设 HXPT_EMERGENCY_ADMIN=<username> 临时放行
	// （以 env 为信用的引导登录，不恢复本地密码语义；仅应急、可审计）。
	if emerg := os.Getenv("HXPT_EMERGENCY_ADMIN"); emerg != "" && emerg == req.Username {
		audit(c, "hxpt_login", "ok", slog.String("via", "emergency"), slog.String("user", req.Username))
		h.issueForHxpt(c, "emergency:"+req.Username, req.Username, "")
		return
	}
	// 重组 challenge：imagehash + 前端回带的 extra_form（secret 等）+ challenge_mode
	// （challenge_mode 必须回传，否则 PostLogin 挑战响应分支不触发，退回普通密码路径被好学拒）
	ch := &site.CaptchaChallenge{
		ImageHash:     req.ImageHash,
		ExtraForm:     req.ExtraForm,
		NeedCaptcha:   req.ImageHash != "",
		ChallengeMode: req.ChallengeMode,
	}
	out, err := h.hxptAuth.Login(c.Request.Context(),
		site.LoginCredentials{Username: req.Username, Password: req.Password}, ch, req.ImageString, req.TwoStepCode)
	if err != nil {
		switch {
		case errors.Is(err, site.ErrChallengeMode):
			c.JSON(http.StatusConflict, gin.H{"code": "challenge_mode", "message": "好学已开启挑战响应认证，请改用 Cookie 登录"})
		case errors.Is(err, site.ErrLoginFailed):
			audit(c, "hxpt_login", "fail", slog.String("user", req.Username), slog.String("reason", "creds/captcha/2fa"))
			c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名/密码/验证码错误"})
		default:
			audit(c, "hxpt_login", "fail", slog.String("err", err.Error()))
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		}
		return
	}
	if !out.VIP.Verified {
		audit(c, "hxpt_login", "fail", slog.String("reason", "not_vip"))
		c.JSON(http.StatusForbidden, gin.H{"code": "not_vip", "message": "非好学 APP 专属 VIP", "vip": out.VIP})
		return
	}
	h.issueForHxpt(c, out.HxptUID, out.VIP.Username, out.Cookie)
}

// ===== 好学 Cookie 登录（2FA / 挑战响应账号的兜底） =====

type hxptCookieLoginReq struct {
	Cookie string `json:"cookie"`
}

// HxptCookieLogin 直接用好学 cookie 校验 VIP 并登录（最稳路径，覆盖 2FA/挑战响应账号）。
func (h *AuthHandler) HxptCookieLogin(c *gin.Context) {
	if h.hxptAuth == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "登录服务未启用"})
		return
	}
	var req hxptCookieLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Cookie == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cookie 必填"})
		return
	}
	status := h.hxptAuth.Verify(c.Request.Context(), req.Cookie)
	audit(c, "hxpt_cookie_login", boolStr(status.Verified), slog.String("level", status.Level))
	if !status.Verified {
		c.JSON(http.StatusForbidden, gin.H{"code": "not_vip", "message": status.Message, "vip": status})
		return
	}
	uid := status.ID
	if uid == "" {
		uid = status.Username // 资料页未解析出 ID 时回退用户名
	}
	h.issueForHxpt(c, uid, status.Username, req.Cookie)
}

// DemoLogin 为评审/演示实例提供低门槛入口。默认关闭，必须显式设置 DEMO_MODE=true。
// 演示用户是普通 VIP 用户而非管理员，避免误触高破坏操作；生产实例不应开启。
func (h *AuthHandler) DemoLogin(c *gin.Context) {
	if !h.demoMode {
		c.JSON(http.StatusNotFound, gin.H{"error": "演示登录未启用"})
		return
	}
	now := time.Now()
	var u models.User
	err := h.db.Where("hxpt_uid = ?", "demo").First(&u).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		u = models.User{
			Username: "demo", HxptUID: "demo", HxptUsername: "演示用户",
			VipLevel: 1, VipVerifiedAt: &now, IsAdmin: false,
		}
		if err := h.db.Create(&u).Error; err != nil {
			dbFail(c, "demo_user_create", err)
			return
		}
	case err != nil:
		dbFail(c, "demo_user_lookup", err)
		return
	default:
		u.VipLevel = 1
		u.VipVerifiedAt = &now
		u.HxptUsername = "演示用户"
		u.IsAdmin = false
		if err := h.db.Save(&u).Error; err != nil {
			dbFail(c, "demo_user_save", err)
			return
		}
	}
	token, err := h.jwt.Issue(u.ID, u.Username, u.IsAdmin, u.TokenVersion, u.VipLevel, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token failed"})
		return
	}
	audit(c, "demo_login", "ok")
	c.JSON(http.StatusOK, gin.H{"token": token, "user": u})
}

// issueForHxpt 按 HxptUID upsert User 并签发带 TokenVersion + VIP 快照的 JWT。
// 仅在 VIP 校验通过后调用：VipLevel=1，VipVerifiedAt=now，cookie 加密落库（供定期重验）。
func (h *AuthHandler) issueForHxpt(c *gin.Context, hxptUID, username, cookie string) {
	now := time.Now()
	cookieEnc, _ := h.crypto.Encrypt(cookie)
	var u models.User
	err := h.db.Where("hxpt_uid = ?", hxptUID).First(&u).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		u = models.User{
			Username: username, HxptUID: hxptUID, HxptUsername: username,
			VipLevel: 1, VipVerifiedAt: &now, HxptCookieEnc: cookieEnc,
			IsAdmin: h.firstHxptUser(),
		}
		if err := h.db.Create(&u).Error; err != nil {
			dbFail(c, "hxpt_user_create", err)
			return
		}
	case err != nil:
		dbFail(c, "hxpt_user_lookup", err)
		return
	default:
		u.VipLevel = 1
		u.VipVerifiedAt = &now
		u.HxptCookieEnc = cookieEnc
		if u.Username == "" {
			u.Username = username
		}
		if u.HxptUsername == "" {
			u.HxptUsername = username
		}
		if err := h.db.Save(&u).Error; err != nil {
			dbFail(c, "hxpt_user_save", err)
			return
		}
	}
	token, err := h.jwt.Issue(u.ID, u.Username, u.IsAdmin, u.TokenVersion, u.VipLevel, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token failed"})
		return
	}
	audit(c, "hxpt_login", "ok", slog.String("hxpt_uid", hxptUID))
	c.JSON(http.StatusOK, gin.H{"token": token, "user": u})
}

// firstHxptUser 报告是否尚无好学绑定用户（首个好学 VIP 用户成为 admin）。
func (h *AuthHandler) firstHxptUser() bool {
	var n int64
	h.db.Model(&models.User{}).Where("hxpt_uid <> ''").Count(&n)
	return n == 0
}

// ===== 登出（TokenVersion 失效） =====

// Logout bump 当前用户 TokenVersion，使其所有已签发 JWT 立即失效。
func (h *AuthHandler) Logout(c *gin.Context) {
	uid, _ := c.Get("uid")
	id, _ := uid.(uint)
	if id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API-Key 旁路无需登出"})
		return
	}
	if err := h.db.Model(&models.User{}).Where("id = ?", id).
		UpdateColumn("token_version", gorm.Expr("token_version + 1")).Error; err != nil {
		dbFail(c, "logout_token_bump", err)
		return
	}
	if h.tvCache != nil {
		h.tvCache.Invalidate(id)
	}
	audit(c, "logout", "ok")
	c.JSON(http.StatusOK, gin.H{"revoked": true})
}

// ===== Legacy / 兼容端点 =====

// Setup 本地初始化已停用（改好学登录），保留路由返回 410 引导。
func (h *AuthHandler) Setup(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"code": "deprecated", "message": "本地初始化已停用，请使用好学账号/Cookie 登录"})
}

// Login 本地登录已停用（改好学登录），保留路由返回 410 引导。
func (h *AuthHandler) Login(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"code": "deprecated", "message": "本地登录已停用，请使用好学账号/Cookie 登录"})
}

type activateReq struct {
	Code     string `json:"code" binding:"required"`
	DeviceID string `json:"device_id"`
}

// Activate（legacy）消费好学 APP 专属 VIP 激活码。不再作实例级准入 gate，保留兼容。
func (h *AuthHandler) Activate(c *gin.Context) {
	var req activateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.DeviceID == "" {
		req.DeviceID = "default"
	}
	level, err := h.act.Activate(req.Code, req.DeviceID)
	if err != nil {
		audit(c, "activate", "fail", slog.String("reason", err.Error()))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	audit(c, "activate", "ok", slog.Int("vip_level", level))
	c.JSON(http.StatusOK, gin.H{"activated": true, "vip_level": level})
}

// Status 上报鉴权与 VIP 状态（供前端 UI 外壳）。authed 时附带当前 user，供前端刷新后
// 恢复顶栏用户名 / VIP 状态——user 只在登录时下发、不随 token 持久化，刷新即丢。
func (h *AuthHandler) Status(c *gin.Context) {
	uid, authed := c.Get("uid")
	verified := false
	var user *models.User
	if authed {
		if raw, ok := c.Get("vip_verified_at"); ok {
			if ts, ok := raw.(int64); ok && ts > 0 {
				verified = true
			}
		}
		if id, ok := uid.(uint); ok && id > 0 {
			var u models.User
			if err := h.db.First(&u, id).Error; err == nil {
				user = &u
			}
		}
	}
	resp := gin.H{
		"activated":     h.act != nil && h.act.AnyActivated(),
		"authenticated": authed,
		"auth_site_on":  h.hxptAuth != nil,
		"vip_verified":  verified,
		"demo_mode":     h.demoMode,
	}
	if user != nil {
		resp["user"] = user
	}
	c.JSON(http.StatusOK, resp)
}

type verifyVIPReq struct {
	Cookie string `json:"cookie"`
}

// VerifyVIP 用好学 cookie 校验 APP 专属 VIP 身份（复用 HxptAuthenticator.Verify）。
func (h *AuthHandler) VerifyVIP(c *gin.Context) {
	if h.hxptAuth == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "VIP 校验未启用"})
		return
	}
	var req verifyVIPReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status := h.hxptAuth.Verify(c.Request.Context(), req.Cookie)
	audit(c, "verify_vip", boolStr(status.Verified), slog.String("level", status.Level))
	c.JSON(http.StatusOK, status)
}

func boolStr(b bool) string {
	if b {
		return "ok"
	}
	return "fail"
}
