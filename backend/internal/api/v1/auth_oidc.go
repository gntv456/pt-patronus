package v1

import (
	"errors"
	"fmt"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// ===== 登出（TokenVersion 失效） =====

// Logout bump 当前用户 TokenVersion，使其所有已签发 JWT 立即失效。
func (h *AuthHandler) Logout(c *gin.Context) {
	uid, _ := c.Get("uid")
	id, _ := uid.(uint)
	if id == 0 {
		errcode.Respond(c, errcode.APIKeyNoLogout)
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
	h.clearAuthCookie(c) // 登出清 httpOnly cookie（TokenVersion bump 已使 JWT 本身失效）
	c.JSON(http.StatusOK, gin.H{"revoked": true})
}

// ===== Legacy / 兼容端点 =====

// Setup 本地初始化已停用（改好学登录），保留路由返回 410 引导。
func (h *AuthHandler) Setup(c *gin.Context) {
	errcode.Respond(c, errcode.OIDCDeprecated)
}

// Login 本地登录已停用（改好学登录），保留路由返回 410 引导。
func (h *AuthHandler) Login(c *gin.Context) {
	errcode.Respond(c, errcode.OIDCDeprecated)
}

type activateReq struct {
	Code     string `json:"code" binding:"required,max=128"`
	DeviceID string `json:"device_id" binding:"max=64"`
}

// Activate（legacy）消费好学 APP 专属 VIP 激活码。不再作实例级准入 gate，保留兼容。
func (h *AuthHandler) Activate(c *gin.Context) {
	var req activateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if req.DeviceID == "" {
		req.DeviceID = "default"
	}
	level, err := h.act.Activate(req.Code, req.DeviceID)
	if err != nil {
		audit(c, "activate", "fail", slog.String("reason", err.Error()))
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
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
	if h.oidc != nil {
		oc := h.oidc.LoadSettings()
		resp["oidc_enabled"] = oc.Enabled && oc.Issuer != "" && oc.ClientID != ""
		resp["oidc_display_name"] = oc.DisplayName
	} else {
		resp["oidc_enabled"] = false
	}
	if user != nil {
		resp["user"] = user
	}
	c.JSON(http.StatusOK, resp)
}

type verifyVIPReq struct {
	Cookie string `json:"cookie" binding:"max=8192"`
}

// VerifyVIP 用好学 cookie 校验 APP 专属 VIP 身份（复用 HxptAuthenticator.Verify）。
func (h *AuthHandler) VerifyVIP(c *gin.Context) {
	if h.hxptAuth == nil {
		errcode.Respond(c, errcode.VIPCheckDisabled)
		return
	}
	var req verifyVIPReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
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

// ===== OIDC / SSO（Authorization Code + PKCE） =====

// oidcRedirectURI 计算回调地址（本后端的 /api/v1/auth/oidc/callback）。
// 优先用请求的对外基址（X-Forwarded-Proto/Host），回退 Request.Host。
func oidcRedirectURI(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/api/v1/auth/oidc/callback", scheme, c.Request.Host)
}

// OIDCStart 发起 OIDC 授权：写 state/verifier cookie 并 302 跳转到 Provider 授权端点。
func (h *AuthHandler) OIDCStart(c *gin.Context) {
	if h.oidc == nil {
		errcode.Respond(c, errcode.OIDCNotConfigured)
		return
	}
	flow, err := h.oidc.StartFlow(c.Request.Context(), oidcRedirectURI(c))
	if err != nil {
		audit(c, "oidc_start", "fail", slog.String("err", err.Error()))
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	http.SetCookie(c.Writer, &flow.StateCookie)
	http.SetCookie(c.Writer, &flow.VerifierCookie)
	c.Redirect(http.StatusFound, flow.AuthorizeURL)
}

// OIDCCallback 处理 Provider 回调：校验 state + 换令牌 + 取 userinfo + 签发本地 JWT + 302 回前端。
func (h *AuthHandler) OIDCCallback(c *gin.Context) {
	if h.oidc == nil {
		errcode.Respond(c, errcode.OIDCNotConfigured)
		return
	}
	stateCookie, err := c.Cookie("ptp_oidc_state")
	verifier, _ := c.Cookie("ptp_oidc_verifier")
	code := c.Query("code")
	state := c.Query("state")
	if err != nil || code == "" || state == "" || state != stateCookie {
		audit(c, "oidc_callback", "fail", slog.String("reason", "state_mismatch"))
		errcode.Respond(c, errcode.OIDCStateMismatch)
		return
	}
	ui, err := h.oidc.CompleteFlow(c.Request.Context(), code, state, verifier, oidcRedirectURI(c))
	// 清掉一次性 cookie。
	clearOIDCCookie(c, "ptp_oidc_state")
	clearOIDCCookie(c, "ptp_oidc_verifier")
	if err != nil {
		audit(c, "oidc_callback", "fail", slog.String("err", err.Error()))
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	// 映射用户名：preferred_username → name → email 前缀 → sub。
	username := ui.Username
	if username == "" {
		username = ui.Name
	}
	if username == "" && ui.Email != "" {
		username = strings.SplitN(ui.Email, "@", 2)[0]
	}
	if username == "" {
		username = ui.Sub
	}
	token, err := h.issueForOIDC("oidc:"+ui.Sub, username)
	if err != nil {
		// 单用户限制拒绝（errcode 信封透传给前端）与签发失败分开呈现。
		if errors.Is(err, errcode.AuthUserRejected) {
			audit(c, "oidc_callback", "reject", slog.String("sub", ui.Sub), slog.String("reason", "single_user_limit"))
			errcode.Respond(c, errcode.AuthUserRejected)
			return
		}
		errcode.Respond(c, errcode.AuthTokenFailed)
		return
	}
	audit(c, "oidc_callback", "ok", slog.String("sub", ui.Sub), slog.String("user", username))
	// 302 回前端：用 fragment(#oidc_token=) 传递短 TTL JWT。fragment 不进服务器访问日志、
	// 不随 Referrer 头外泄、不留在浏览器历史的 query 串里，比 query 更安全（仍建议生产走 HTTPS）。
	cfg := h.oidc.LoadSettings()
	target := strings.TrimSpace(cfg.FrontendRedirect)
	if target == "" {
		target = "/login"
	}
	fragSep := "#"
	if strings.Contains(target, "#") {
		fragSep = "&"
	}
	c.Redirect(http.StatusFound, target+fragSep+"oidc_token="+url.QueryEscape(token))
}

func clearOIDCCookie(c *gin.Context, name string) {
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1})
}

// issueForOIDC 按 OIDC sub upsert User 并签发 JWT。OIDC 由管理员显式启用，视为可信身份：
// VipLevel=1 + VipVerifiedAt=now（放行 VIP 准入）；首个 OIDC 用户成为 admin（供配置者自助）。
// 单用户硬限制（与好学登录同口径）：实例已有真实用户时拒绝第二账号创建。
func (h *AuthHandler) issueForOIDC(hxptUID, username string) (string, error) {
	now := time.Now()
	var u models.User
	err := h.db.Where("hxpt_uid = ?", hxptUID).First(&u).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		if uid := h.existingRealUserUID(); uid != "" && uid != hxptUID {
			slog.Warn("SECURITY: second oidc user rejected (single-user instance)", slog.String("sub", hxptUID))
			return "", errcode.AuthUserRejected
		}
		u = models.User{
			Username: username, HxptUID: hxptUID, HxptUsername: username,
			VipLevel: 1, VipVerifiedAt: &now, IsAdmin: h.firstOIDCUser(),
		}
		if err := h.db.Create(&u).Error; err != nil {
			return "", err
		}
	case err != nil:
		return "", err
	default:
		u.VipLevel = 1
		u.VipVerifiedAt = &now
		if u.Username == "" {
			u.Username = username
		}
		if err := h.db.Save(&u).Error; err != nil {
			return "", err
		}
	}
	return h.jwt.Issue(u.ID, u.Username, u.IsAdmin, u.TokenVersion, u.VipLevel, now)
}

// firstOIDCUser 报告是否尚无 OIDC 绑定用户（首个 OIDC 用户成为 admin）。
func (h *AuthHandler) firstOIDCUser() bool {
	var n int64
	h.db.Model(&models.User{}).Where("hxpt_uid LIKE 'oidc:%'").Count(&n)
	return n == 0
}

// firstHxptUser 报告是否尚无好学 VIP 绑定用户（首个 HXPT 用户成为 admin，与 firstOIDCUser 对称）。
func (h *AuthHandler) firstHxptUser() bool {
	return h.existingRealUserUID() == ""
}

// existingRealUserUID 返回已有的任一「真实」用户（好学/OIDC 绑定账号）的 hxpt_uid；
// 无则返回 ""。demo 演示账号与 emergency:% 应急账号不计——demo 由 DemoMode 开关控制、
// emergency 需 SECRET_KEY 派生口令，均非「第二个能自助注册的身份」。单用户硬限制
// 依据此判定：非空即实例已有主人，任何新身份一律拒绝创建。
func (h *AuthHandler) existingRealUserUID() string {
	var u models.User
	err := h.db.Where("hxpt_uid NOT LIKE 'oidc:%' AND hxpt_uid NOT LIKE 'emergency:%' AND hxpt_uid != 'demo'").
		Order("id ASC").First(&u).Error
	if err != nil {
		return ""
	}
	return u.HxptUID
}

// maskUID 日志脱敏：UID 只留前 6 位 + 长度（防审计日志反推完整好学账号）。
func maskUID(uid string) string {
	if len(uid) <= 6 {
		return uid
	}
	return uid[:6] + "…"
}

// ===== OIDC 配置（管理员） =====

// oidcConfigResp 对外脱敏配置视图。
type oidcConfigResp struct {
	Enabled          bool   `json:"enabled"`
	Issuer           string `json:"issuer"`
	ClientID         string `json:"client_id"`
	ClientSecretSet  bool   `json:"client_secret_configured"`
	Scopes           string `json:"scopes"`
	DisplayName      string `json:"display_name"`
	FrontendRedirect string `json:"frontend_redirect"`
}

// GetOIDCConfig 返回脱敏配置。
func (h *AuthHandler) GetOIDCConfig(c *gin.Context) {
	if h.oidc == nil {
		errcode.Respond(c, errcode.OIDCNotConfigured)
		return
	}
	s := h.oidc.LoadSettings()
	c.JSON(http.StatusOK, oidcConfigResp{
		Enabled: s.Enabled, Issuer: s.Issuer, ClientID: s.ClientID,
		ClientSecretSet: s.ClientSecret != "", Scopes: s.Scopes,
		DisplayName: s.DisplayName, FrontendRedirect: s.FrontendRedirect,
	})
}

// oidcConfigReq 管理员提交（client_secret 空串=保留旧值）。
type oidcConfigReq struct {
	Enabled          bool   `json:"enabled"`
	Issuer           string `json:"issuer"`
	ClientID         string `json:"client_id"`
	ClientSecret     string `json:"client_secret"`
	Scopes           string `json:"scopes"`
	DisplayName      string `json:"display_name"`
	FrontendRedirect string `json:"frontend_redirect"`
}

// SetOIDCConfig 保存配置（client_secret 空串保留）。
func (h *AuthHandler) SetOIDCConfig(c *gin.Context) {
	if h.oidc == nil {
		errcode.Respond(c, errcode.OIDCNotConfigured)
		return
	}
	var req oidcConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	cur := h.oidc.LoadSettings()
	next := cur
	next.Enabled = req.Enabled
	next.Issuer = req.Issuer
	next.ClientID = req.ClientID
	if req.ClientSecret != "" {
		next.ClientSecret = req.ClientSecret
	}
	if req.Scopes != "" {
		next.Scopes = req.Scopes
	}
	if req.DisplayName != "" {
		next.DisplayName = req.DisplayName
	}
	if req.FrontendRedirect != "" {
		next.FrontendRedirect = req.FrontendRedirect
	}
	if err := h.oidc.SaveSettings(next); err != nil {
		errcode.Respond(c, errcode.SaveFailed)
		return
	}
	audit(c, "oidc_config", "ok", slog.Bool("enabled", next.Enabled))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
