// Package middleware holds shared Gin middlewares.
package middleware

import (
	"crypto/subtle"
	"strings"

	"github.com/hxpt/ptpatronus/internal/api/errcode"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/core/security"
)

// JWTAuth 校验凭证（优先级：X-API-Key > Authorization Bearer > ptp_token httpOnly cookie）
// 并写入身份；无凭证的请求放行（公开路由），用 RequireAuth 强制鉴权。
// cookie 通道是 Web 端收权路径：登录响应同时种 httpOnly cookie（XSS 读不到），
// 浏览器同源请求自动携带；扩展/移动端继续走 Bearer（跨源 cookie 不发送，互不干扰）。
// tvCache 非 nil 时做 TokenVersion 失效校验（登出/改绑后旧 token 拒绝）；为 nil 则跳过（兼容旧测试）。
//
// skipCookiePaths：这些路径上浏览器无法控制 cookie 头（WebSocket 升级请求由浏览器
// 自动附带 ptp_token，且应用无法添加 Authorization 头），鉴权由路由自身的 ticket/参数
// 完成。此时残留的过期 cookie 不能抢先判 401 —— 否则用户会话一过期，WS 会在
// JWTAuth 层被脏 cookie 永久拒绝（前端表现为实时推送静默停摆 + 每 3s 401 循环），
// 连换新 ticket 都无法恢复。空切片保持原行为。
func JWTAuth(j *security.JWT, apiKey string, tvCache *auth.TokenVersionCache, skipCookiePaths ...string) gin.HandlerFunc {
	shouldSkipCookie := func(path string) bool {
		for _, p := range skipCookiePaths {
			if path == p {
				return true
			}
		}
		return false
	}
	return func(c *gin.Context) {
		// API Key 程序化访问（命中配置密钥即授予管理员身份）
		if apiKey != "" {
			// 恒定时间比较，避免逐字节短路比较的计时侧信道（API-Key 命中即授 admin 身份）。
			if k := c.GetHeader("X-API-Key"); k != "" && subtle.ConstantTimeCompare([]byte(k), []byte(apiKey)) == 1 {
				c.Set("uid", uint(0))
				c.Set("username", "apikey")
				c.Set("is_admin", true)
				c.Set("auth_via", "apikey")
				c.Next()
				return
			}
		}
		token := ""
		if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
			token = strings.TrimPrefix(h, "Bearer ")
		} else if !shouldSkipCookie(c.Request.URL.Path) {
			if ck, err := c.Cookie("ptp_token"); err == nil && ck != "" {
				token = ck
			}
		}
		if token == "" {
			c.Next()
			return
		}
		claims, err := j.Parse(token)
		if err != nil {
			errcode.Respond(c, errcode.InvalidToken)
			return
		}
		// TokenVersion 失效校验：与 User.TokenVersion 不符（登出/改绑后）即拒绝。
		if tvCache != nil {
			cur, err := tvCache.Get(claims.UserID)
			if err != nil || cur != claims.TokenVersion {
				errcode.Respond(c, errcode.TokenMismatch)
				return
			}
		}
		c.Set("uid", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("is_admin", claims.IsAdmin)
		c.Set("auth_via", "jwt")
		c.Set("vip_verified_at", claims.VipVerifiedAt) // int64 unix；0=未验证（供 RequireVIP 快速判宽限）
		c.Set("vip_level", claims.VipLevel)
		c.Set("claims", claims) // 完整 claims（Refresh 滑动续期需要 FirstIssuedAt）
		c.Next()
	}
}

// RequireAuth aborts with 401 if no authenticated user is present.
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := c.Get("uid"); !ok {
			errcode.Respond(c, errcode.UnauthorizedOp)
			return
		}
		c.Next()
	}
}

// RequireAdmin aborts with 403 if the authenticated user is not an admin.
// 身份由 JWTAuth 注入：API-Key 旁路与首个好学 VIP 用户为 admin。用于收敛高破坏操作
// （删除资源 / 备份导入覆盖配置 / agent 密钥管理 / webhook 触发工作流）——
// 非 admin 的 VIP 用户可读可用，但不能拆系统。
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		v, ok := c.Get("is_admin")
		isAdmin, _ := v.(bool)
		if !ok || !isAdmin {
			errcode.Respond(c, errcode.AdminRequired)
			return
		}
		c.Next()
	}
}
