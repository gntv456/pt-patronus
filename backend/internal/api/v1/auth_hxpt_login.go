package v1

// 好学登录三路径（账号密码 / Cookie 兜底 / 演示账号）与 token 签发。
// 从 auth.go 迁出;challenge 重组、应急管理员兜底、VIP 校验与审计逻辑不变。

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/ocr"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// （前端可预填，识别失败/未配置仍走人工输入——不 block 登录链路）。
func (h *AuthHandler) HxptCaptcha(c *gin.Context) {
	if h.hxptAuth == nil {
		errcode.Respond(c, errcode.AuthServiceUnavailable)
		return
	}
	ch, err := h.hxptAuth.FetchCaptcha(c.Request.Context())
	if err != nil {
		audit(c, "hxpt_captcha", "fail", slog.String("err", err.Error()))
		errcode.Respond(c, errcode.AuthCaptchaFailed.WithErr(err))
		return
	}
	resp := gin.H{
		"imagehash":      ch.ImageHash,
		"image":          ch.ImageB64,
		"need_captcha":   ch.NeedCaptcha,
		"challenge_mode": ch.ChallengeMode,
		"extra_form":     ch.ExtraForm,
	}
	if ch.NeedCaptcha && h.ocrCfg != nil {
		if cli := ocr.New(h.ocrCfg()); cli.Enabled() {
			if text, err := cli.Recognize(c.Request.Context(), ch.ImageB64); err == nil && text != "" {
				resp["ocr_text"] = text
			} else if err != nil {
				slog.Debug("captcha ocr failed", "err", err)
			}
		}
	}
	c.JSON(http.StatusOK, resp)
}

type hxptLoginReq struct {
	Username      string            `json:"username" binding:"max=64"`
	Password      string            `json:"password" binding:"max=128"`
	ImageHash     string            `json:"imagehash" binding:"max=64"`
	ImageString   string            `json:"imagestring" binding:"max=32"`
	TwoStepCode   string            `json:"two_step_code" binding:"max=64"`
	ExtraForm     map[string]string `json:"extra_form" binding:"dive,max=512"`
	ChallengeMode bool              `json:"challenge_mode"` // 由 captcha 步探得，回传以驱动挑战响应分支
}

// validateEmergencyAdmin 校验应急登录：env 形如 "<username>:<口令>"，口令为
// HMAC-SHA256(SECRET_KEY, username) 的前 8 字节 hex（运维可用
// `openssl dgst -sha256 -hmac "$SECRET_KEY"` 自行生成）。用户名与口令都对上才放行；
// 返回 (username, true)。错配仍落审计日志，便于发现探测。
func (h *AuthHandler) validateEmergencyAdmin(c *gin.Context, env, username, password string) (string, bool) {
	user, pass, ok := strings.Cut(env, ":")
	if !ok || user == "" || pass == "" || username == "" || password == "" {
		return "", false
	}
	if !constEqStr(user, username) {
		return "", false
	}
	if !constEqStr(pass, password) {
		audit(c, "hxpt_login", "fail", slog.String("via", "emergency"), slog.String("reason", "bad emergency passcode"))
		return "", false
	}
	return user, true
}

// constEqStr 恒定时间字符串比较（应急口令校验防时序侧信道）。
func constEqStr(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// HxptLogin 账号登录 Step B：POST takelogin.php + VIP 校验。
func (h *AuthHandler) HxptLogin(c *gin.Context) {
	if h.hxptAuth == nil {
		errcode.Respond(c, errcode.AuthServiceUnavailable)
		return
	}
	var req hxptLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	// 应急兜底：好学 POST 链路坏掉时，运维设 HXPT_EMERGENCY_ADMIN 临时放行。
	// 值不再是裸用户名，而是 "<username>:<hex(hmac_sha256(secret, username)[0:8])>"：
	// 知道用户名不够，必须持有 SECRET_KEY 派生的口令段才能登录（防 env 值仅作
	// 用户名列表泄露时被冒用）；issued token 仍走 issueForHxpt 全量签发并审计。
	if emerg := os.Getenv("HXPT_EMERGENCY_ADMIN"); emerg != "" {
		if user, ok := h.validateEmergencyAdmin(c, emerg, req.Username, req.Password); ok {
			slog.Warn("SECURITY: emergency admin bypass — disable HXPT_EMERGENCY_ADMIN now", slog.String("user", user), slog.String("ip", c.ClientIP()))
			audit(c, "hxpt_login", "ok", slog.String("via", "emergency"), slog.String("user", user))
			h.issueForHxpt(c, "emergency:"+user, user, "")
			return
		}
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
			errcode.Respond(c, errcode.ChallengeMode)
		case errors.Is(err, site.ErrLoginFailed):
			audit(c, "hxpt_login", "fail", slog.String("user", req.Username), slog.String("reason", "creds/captcha/2fa"))
			errcode.Respond(c, errcode.AuthBadCredentials)
		default:
			audit(c, "hxpt_login", "fail", slog.String("err", err.Error()))
			errcode.Respond(c, errcode.BadGateway.WithErr(err))
		}
		return
	}
	if !out.VIP.Verified {
		audit(c, "hxpt_login", "fail", slog.String("reason", "not_vip"))
		c.JSON(http.StatusForbidden, gin.H{"code": errcode.NotVIP.Code, "message": errcode.NotVIP.Message, "error": errcode.NotVIP.Message, "vip": out.VIP})
		return
	}
	h.issueForHxpt(c, out.HxptUID, out.VIP.Username, out.Cookie)
}

// ===== 好学 Cookie 登录（2FA / 挑战响应账号的兜底） =====

type hxptCookieLoginReq struct {
	Cookie string `json:"cookie" binding:"max=8192"`
}

// HxptCookieLogin 直接用好学 cookie 校验 VIP 并登录（最稳路径，覆盖 2FA/挑战响应账号）。
func (h *AuthHandler) HxptCookieLogin(c *gin.Context) {
	if h.hxptAuth == nil {
		errcode.Respond(c, errcode.AuthServiceUnavailable)
		return
	}
	var req hxptCookieLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if req.Cookie == "" {
		errcode.Respond(c, errcode.AuthCookieRequired)
		return
	}
	status := h.hxptAuth.Verify(c.Request.Context(), req.Cookie)
	audit(c, "hxpt_cookie_login", boolStr(status.Verified), slog.String("level", status.Level))
	if !status.Verified {
		c.JSON(http.StatusForbidden, gin.H{"code": errcode.NotVIP.Code, "message": status.Message, "error": status.Message, "vip": status})
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
		errcode.Respond(c, errcode.DemoDisabled)
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
		errcode.Respond(c, errcode.AuthTokenFailed)
		return
	}
	audit(c, "demo_login", "ok")
	h.setAuthCookie(c, token)
	c.JSON(http.StatusOK, gin.H{"token": token, "user": u})
}

// issueForHxpt 按 HxptUID upsert User 并签发带 TokenVersion + VIP 快照的 JWT。
// 仅在 VIP 校验通过后调用：VipLevel=1，VipVerifiedAt=now，cookie 加密落库（供定期重验）。
func (h *AuthHandler) issueForHxpt(c *gin.Context, hxptUID, username, cookie string) {
	now := time.Now()
	cookieEnc, encErr := h.crypto.Encrypt(cookie)
	if encErr != nil {
		// 加密失败不能落空串：否则定期 VIP 重验解密空串失败 → 用户静默掉 VIP 身份并写入脏数据。
		dbFail(c, "hxpt_cookie_encrypt", encErr)
		return
	}
	var u models.User
	err := h.db.Where("hxpt_uid = ?", hxptUID).First(&u).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		// 单用户硬限制（产品定位：整实例只归属首位登录者）：已有其他真实用户（非
		// demo/emergency）时拒绝创建第二账号——VIP 验证只证明「是好学会员」，不能
		// 证明「是实例主人」；不拦则任何好学 VIP 都能登录公网实例拿到非 admin 账号。
		ownerUID, err := h.existingRealUserUID()
		if err != nil {
			dbFail(c, "hxpt_owner_lookup", err)
			return
		}
		if ownerUID != "" && ownerUID != hxptUID {
			audit(c, "hxpt_login", "reject", slog.String("reason", "single_user_limit"), slog.String("hxpt_uid", hxptUID))
			slog.Warn("SECURITY: second hxpt user rejected (single-user instance)", slog.String("hxpt_uid", hxptUID), slog.String("owner_prefix", maskUID(ownerUID)), slog.String("ip", c.ClientIP()))
			errcode.Respond(c, errcode.AuthUserRejected)
			return
		}
		isFirst, err := h.firstHxptUser()
		if err != nil {
			dbFail(c, "hxpt_first_user_lookup", err)
			return
		}
		u = models.User{
			Username: username, HxptUID: hxptUID, HxptUsername: username,
			VipLevel: 1, VipVerifiedAt: &now, HxptCookieEnc: cookieEnc,
			IsAdmin: isFirst,
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
		errcode.Respond(c, errcode.AuthTokenFailed)
		return
	}
	audit(c, "hxpt_login", "ok", slog.String("hxpt_uid", hxptUID))
	h.setAuthCookie(c, token) // Web 端 httpOnly cookie（XSS 不可读）；扩展/移动端仍用 body token
	c.JSON(http.StatusOK, gin.H{"token": token, "user": u})
}

// Refresh 滑动续期：拿仍有效的 Bearer JWT 换新 token（再延一个完整过期窗口，
// 但受 7 天绝对寿命钳制——超限返回 session_expired，客户端须重新登录）。
// 供长时客户端（浏览器扩展等）免好学 cookie 静默续期——JWTAuth 已验签 + TokenVersion
// 校验通过才到这；此处以 User 表真值重发 claims（admin 被收回/VIP 状态变化即时生效，
// 不沿用旧 token 快照）。不进 guarded 组：RequireVIP 不该拦续期（宽限期内的用户
// 仍应能续，过期与否由登录后的业务端点裁决）。
