package v1

import (
	"errors"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"log/slog"
	"net/http"
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

// Activate（legacy）消费历史激活码。不再作实例级准入 gate，保留兼容。
func (h *AuthHandler) Activate(c *gin.Context) {
	var req activateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
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
		"auth_site_on":  false, // 好学改版 FluxTorrent 后旧站点登录链路（验证码/VIP 校验）已下线
		"local_login":   true,  // 本地密码登录恒可用（好学站点关闭后的主路径，#<issue>）
		"vip_verified":  verified,
		"demo_mode":     h.demoMode,
		// 升级引导：已登录账号是否已设本地密码。存量好学账号为 false，前端据此弹
		// 「首次设置本地密码」对话框，用户无需在终端跑 CLI / 改 compose。
		"has_local_password": false,
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

// issueForOIDC 按 OIDC sub upsert User 并签发 JWT。OIDC 由管理员显式启用，视为可信身份：
// VipLevel=1 + VipVerifiedAt=now（放行 VIP 准入）；首个 OIDC 用户成为 admin（供配置者自助）。
// 单用户硬限制（与好学登录同口径）：实例已有真实用户时拒绝第二账号创建。
func (h *AuthHandler) issueForOIDC(hxptUID, username string) (string, error) {
	now := time.Now()
	var u models.User
	err := h.db.Where("hxpt_uid = ?", hxptUID).First(&u).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		ownerUID, err := h.existingRealUserUID()
		if err != nil {
			return "", err
		}
		if ownerUID != "" && ownerUID != hxptUID {
			slog.Warn("SECURITY: second oidc user rejected (single-user instance)", slog.String("sub", hxptUID))
			return "", errcode.AuthUserRejected
		}
		isFirst, err := h.firstOIDCUser()
		if err != nil {
			return "", err
		}
		u = models.User{
			Username: username, HxptUID: hxptUID, HxptUsername: username,
			VipLevel: 1, VipVerifiedAt: &now, IsAdmin: isFirst,
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
// 查不出来时返回错误而不是 true：把 DB 故障当「还没有 OIDC 用户」，
// 等于这一次登录直接被提成 admin。
func (h *AuthHandler) firstOIDCUser() (bool, error) {
	var n int64
	if err := h.db.Model(&models.User{}).Where("hxpt_uid LIKE 'oidc:%'").Count(&n).Error; err != nil {
		return false, err
	}
	return n == 0, nil
}

// existingRealUserUID 返回已有的任一「真实」用户（好学/OIDC 绑定账号）的 hxpt_uid；
// 无则返回 ""。demo 演示账号与 emergency:% 应急账号不计——demo 由 DemoMode 开关控制、
// emergency 需 SECRET_KEY 派生口令，均非「第二个能自助注册的身份」。单用户硬限制
// 依据此判定：非空即实例已有主人，任何新身份一律拒绝创建。
// 查询出错必须与「确实没有主人」区分：把故障当「没主人」会同时打开单用户硬限制
// 与「首位登录者即 admin」两条闸门。
func (h *AuthHandler) existingRealUserUID() (string, error) {
	var u models.User
	err := h.db.Where("hxpt_uid NOT LIKE 'oidc:%' AND hxpt_uid NOT LIKE 'emergency:%' AND hxpt_uid != 'demo'").
		Order("id ASC").First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return u.HxptUID, nil
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
		respondBindError(c, err)
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
		// #353 带上原因：底层是系统配置写库（store.Set），失败原因具体却被丢掉，界面只剩「保存失败」。
		errcode.Respond(c, errcode.SaveFailed.WithErr(err))
		return
	}
	audit(c, "oidc_config", "ok", slog.Bool("enabled", next.Enabled))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
