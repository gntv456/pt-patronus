package v1

// 本地用户名密码登录（好学站点关闭后的主登录路径，#<issue>）：
// POST /auth/local/login 建号/校验 + POST /auth/local/password 改密。
// 复用单用户硬限制与「首位登录者即 admin」闸门（auth_oidc.go），JWT/cookie
// 签发口径与好学登录完全一致（{token, user}），三端解析免改。

import (
	"errors"
	"log/slog"
	"net/http"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

type localLoginReq struct {
	Username string `json:"username" binding:"required,max=64"`
	Password string `json:"password" binding:"required,max=128"`
}

type changePasswordReq struct {
	OldPassword string `json:"old_password" binding:"required,max=128"`
	NewPassword string `json:"new_password" binding:"required,max=128"`
}

// validatePassword 弱密码闸门：≥8 位且同时含字母与数字（#<issue>）。
// 纯数字/纯字母/短口令在公网自托管实例上撑不住限流窗口内的爆破。
func validatePassword(pwd string) bool {
	if len(pwd) < 8 {
		return false
	}
	hasLetter, hasDigit := false, false
	for _, r := range pwd {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	return hasLetter && hasDigit
}

// LocalLogin POST /auth/local/login：本地密码登录（好学站点关闭后的主路径）。
// 首次登录自动建号（首位登录者即 admin，与好学/OIDC 同口径）；实例已有主人时
// 拒绝第二个账号自助注册（单用户硬限制）。存量好学账号未设密码时提示走
// reset-admin --password 初始化，不静默建平行账号。
func (h *AuthHandler) LocalLogin(c *gin.Context) {
	var req localLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	var u models.User
	err := h.db.Where("username = ?", req.Username).First(&u).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		// 单用户硬限制：实例已有主人（好学/OIDC/本地任一真实身份）时拒绝建号。
		ownerUID, err := h.existingRealUserUID()
		if err != nil {
			dbFail(c, "local_owner_lookup", err)
			return
		}
		if ownerUID != "" {
			audit(c, "local_login", "reject",
				slog.String("reason", "single_user_limit"), slog.String("username", req.Username))
			slog.Warn("SECURITY: second local user rejected (single-user instance)",
				slog.String("username", req.Username), slog.String("ip", c.ClientIP()))
			errcode.Respond(c, errcode.AuthUserRejected)
			return
		}
		if !validatePassword(req.Password) {
			errcode.Respond(c, errcode.AuthWeakPassword)
			return
		}
		hash, err := security.HashPassword(req.Password)
		if err != nil {
			dbFail(c, "local_password_hash", err)
			return
		}
		now := time.Now()
		u = models.User{
			Username: req.Username, PasswordHash: hash,
			HxptUID: "local:" + req.Username, HxptUsername: req.Username,
			VipLevel: 1, VipVerifiedAt: &now, IsAdmin: true,
		}
		if err := h.db.Create(&u).Error; err != nil {
			dbFail(c, "local_user_create", err)
			return
		}
	case err != nil:
		dbFail(c, "local_user_lookup", err)
		return
	default:
		if u.PasswordHash == "" {
			// 存量好学/OIDC 账号：从未设过本地密码，不能凭任意口令冒登。
			audit(c, "local_login", "fail",
				slog.String("reason", "password_not_set"), slog.String("username", req.Username))
			errcode.Respond(c, errcode.AuthPasswordNotSet)
			return
		}
		if !security.VerifyPassword(u.PasswordHash, req.Password) {
			audit(c, "local_login", "fail",
				slog.String("reason", "bad_credentials"), slog.String("username", req.Username))
			errcode.Respond(c, errcode.AuthBadCredentials)
			return
		}
	}
	token, err := h.jwt.Issue(u.ID, u.Username, u.IsAdmin, u.TokenVersion, u.VipLevel, time.Now())
	if err != nil {
		errcode.Respond(c, errcode.AuthTokenFailed)
		return
	}
	audit(c, "local_login", "ok", slog.String("user", u.Username))
	h.setAuthCookie(c, token)
	c.JSON(http.StatusOK, gin.H{"token": token, "user": u})
}

// ChangePassword POST /auth/local/password（guarded）：校验旧密码后改新密码。
// 未设过密码的存量账号同样要求先走 reset-admin 初始化，避免「知道用户名即可
// 占用改密入口给自己设密」的提权路径。
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req changePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	if !validatePassword(req.NewPassword) {
		errcode.Respond(c, errcode.AuthWeakPassword)
		return
	}
	uid := c.GetUint("uid")
	var u models.User
	if err := h.db.First(&u, uid).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// guarded 语义下不该到这（JWTAuth 已校验过 uid）；按未设密兜底拒绝。
			audit(c, "local_password", "fail", slog.String("reason", "user_missing"))
			errcode.Respond(c, errcode.AuthPasswordNotSet)
			return
		}
		dbFail(c, "password_user_lookup", err)
		return
	}
	if u.PasswordHash == "" || !security.VerifyPassword(u.PasswordHash, req.OldPassword) {
		audit(c, "local_password", "fail", slog.String("reason", "bad_old_password"))
		errcode.Respond(c, errcode.AuthBadCredentials)
		return
	}
	hash, err := security.HashPassword(req.NewPassword)
	if err != nil {
		dbFail(c, "local_password_hash", err)
		return
	}
	u.PasswordHash = hash
	if err := h.db.Save(&u).Error; err != nil {
		dbFail(c, "local_password_save", err)
		return
	}
	audit(c, "local_password", "ok")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
