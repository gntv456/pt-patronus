package v1

import (
	"encoding/json"
	"errors"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/middleware"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/ocr"
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
	errcode.Respond(c, errcode.AuthServerBusy)
}

// AuthHandler 处理好学账号/Cookie 登录、登出与（legacy）激活码。
type AuthHandler struct {
	db         *gorm.DB
	jwt        *security.JWT
	act        *auth.Activation        // legacy 激活码（不再作准入 gate，保留 Activate 端点）
	hxptAuth   auth.HxptAuthenticator  // 好学登录 + VIP 校验
	crypto     *security.Crypto        // 加密 HxptCookieEnc
	tvCache    *auth.TokenVersionCache // 登出 invalidate
	tickets    *auth.TicketStore       // WS/RTSP 一次性 ticket（替代 query JWT）
	demoMode   bool                    // 显式开启演示登录（默认关闭）
	oidc       *auth.OIDCService       // OIDC/SSO（可空：未装配时 OIDC 端点返 503）
	ocrCfg     func() ocr.Config       // OCR 服务配置读取（可空：无 OCR，验证码人工输入）
	secureCookie bool                  // 反代 TLS / APP_BASE_URL 为 https 时 cookie 加 Secure
}

// NewAuthHandler 构造。secureCookie 决定登录 cookie 是否带 Secure 属性。
func NewAuthHandler(db *gorm.DB, jwt *security.JWT, act *auth.Activation,
	hxptAuth auth.HxptAuthenticator, crypto *security.Crypto, tvCache *auth.TokenVersionCache,
	tickets *auth.TicketStore, demoMode bool, oidc *auth.OIDCService, secureCookie bool) *AuthHandler {
	return &AuthHandler{db: db, jwt: jwt, act: act, hxptAuth: hxptAuth, crypto: crypto, tvCache: tvCache, tickets: tickets, demoMode: demoMode, oidc: oidc, secureCookie: secureCookie}
}

// setAuthCookie 登录成功时把 JWT 同时种进 httpOnly cookie（收权加固：
// Web 端 XSS 无法读取 token，配合前端「cookie 优先 + Bearer 兜底」双通道）。
// Lax + Path=/api/v1：跨站 POST 不携带，只服务同源 API；非 TLS 部署由 secureCookie 控制。
func (h *AuthHandler) setAuthCookie(c *gin.Context, token string) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "ptp_token",
		Value:    token,
		Path:     "/api/v1",
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.jwt.Expires().Seconds()),
	})
}

// clearAuthCookie 登出/失效时清 cookie。
func (h *AuthHandler) clearAuthCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "ptp_token",
		Value:    "",
		Path:     "/api/v1",
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// SetOCRConfig 注入 OCR 配置读取器（app 装配期；nil=OCR 未启用）。
func (h *AuthHandler) SetOCRConfig(f func() ocr.Config) { h.ocrCfg = f }

// WsTicket 签发一次性 WS/RTSP ticket（guarded 内：header JWT 已校验，含 TokenVersion 失效校验）。
// 客户端用返回的 ticket 以 ?ticket= 连 WS/RTSP，避免 JWT 落 URL。
func (h *AuthHandler) WsTicket(c *gin.Context) {
	tk, exp, err := h.tickets.Issue(c.GetUint("uid"))
	if err != nil {
		audit(c, "ws_ticket", "fail", slog.String("err", err.Error()))
		errcode.Respond(c, errcode.AuthTicketFailed)
		return
	}
	audit(c, "ws_ticket", "ok")
	c.JSON(http.StatusOK, gin.H{
		"ticket":     tk,
		"expires_in": int(time.Until(exp).Seconds()),
	})
}

// ===== 好学账号登录（验证码两步） =====

// HxptCaptcha 账号登录 Step A：取登录页验证码 / 探测模式。OCR 已配置时附带自动识别结果
func (h *AuthHandler) Refresh(c *gin.Context) {
	uid, _ := c.Get("uid")
	id, _ := uid.(uint)
	if id == 0 {
		errcode.Respond(c, errcode.APIKeyNoRefresh)
		return
	}
	var u models.User
	if err := h.db.First(&u, id).Error; err != nil {
		dbFail(c, "refresh_user_lookup", err)
		return
	}
	vipAt := time.Time{}
	if u.VipVerifiedAt != nil {
		vipAt = *u.VipVerifiedAt
	}
	// 保留旧 token 的首次签发时间，滑动续期不突破绝对寿命上限。
	var oldClaims *security.Claims
	if raw, ok := c.Get("claims"); ok {
		oldClaims, _ = raw.(*security.Claims)
	}
	var token string
	var err error
	if oldClaims != nil {
		// 滑动续期：沿用旧 token 首次签发时间（绝对寿命钳制），admin/VIP 以 User 表真值重发。
		token, err = h.jwt.IssueSliding(&security.Claims{
			UserID:        u.ID,
			Username:      u.Username,
			IsAdmin:       u.IsAdmin,
			FirstIssuedAt: oldClaims.FirstIssuedAt,
		}, u.TokenVersion, u.VipLevel, vipAt)
	} else {
		token, err = h.jwt.Issue(u.ID, u.Username, u.IsAdmin, u.TokenVersion, u.VipLevel, vipAt)
	}
	if err != nil {
		errcode.Respond(c, errcode.AuthTokenFailed)
		return
	}
	audit(c, "refresh", "ok")
	h.setAuthCookie(c, token) // 滑动续期同步刷新 cookie MaxAge
	c.JSON(http.StatusOK, gin.H{"token": token, "user": u})
}

// ---- 用户偏好（跨浏览器同步，对标 MSaber 界面偏好随账号同步） ----

const userPrefMaxValue = 64 * 1024 // 64KB：偏好 JSON 上限（防滥用）

// GetPreferences GET /auth/preferences/{key}：读单域偏好（无则空对象）。
func (h *AuthHandler) GetPreferences(c *gin.Context) {
	uid := c.GetUint("uid")
	key := c.Param("key")
	if key == "" || len(key) > 64 {
		errcode.Respond(c, errcode.InvalidKey)
		return
	}
	var p models.UserPreference
	err := h.db.Where("user_id = ? AND key = ?", uid, key).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusOK, gin.H{"key": key, "value": "{}"})
		return
	}
	if err != nil {
		dbFail(c, "user_pref_get", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"key": key, "value": p.Value})
}

// SetPreferences PUT /auth/preferences/{key}：整域覆盖保存。
func (h *AuthHandler) SetPreferences(c *gin.Context) {
	uid := c.GetUint("uid")
	key := c.Param("key")
	if key == "" || len(key) > 64 {
		errcode.Respond(c, errcode.InvalidKey)
		return
	}
	var in struct {
		Value string `json:"value" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if len(in.Value) > userPrefMaxValue {
		errcode.Respond(c, errcode.ValueTooLarge)
		return
	}
	// 校验为合法 JSON（前端存 JSON 串；脏数据会在端上解析失败）
	if !json.Valid([]byte(in.Value)) {
		errcode.Respond(c, errcode.ValueInvalidJSON)
		return
	}
	var p models.UserPreference
	err := h.db.Where("user_id = ? AND key = ?", uid, key).First(&p).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		p = models.UserPreference{UserID: uid, Key: key, Value: in.Value}
		if err := h.db.Create(&p).Error; err != nil {
			dbFail(c, "user_pref_create", err)
			return
		}
	case err != nil:
		dbFail(c, "user_pref_lookup", err)
		return
	default:
		p.Value = in.Value
		if err := h.db.Save(&p).Error; err != nil {
			dbFail(c, "user_pref_save", err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
