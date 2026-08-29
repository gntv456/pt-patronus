// Package middleware holds shared Gin middlewares.
package middleware

import (
	"crypto/subtle"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/core/security"
)

// JWTAuth 校验 Bearer token（若存在）并写入身份；支持 X-API-Key 程序化访问。
// 无凭证的请求放行（公开路由）；用 RequireAuth 强制鉴权。tvCache 非 nil 时做 TokenVersion
// 失效校验（登出/改绑后旧 token 拒绝）；为 nil 则跳过（兼容旧测试）。
func JWTAuth(j *security.JWT, apiKey string, tvCache *auth.TokenVersionCache) gin.HandlerFunc {
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
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.Next()
			return
		}
		token := strings.TrimPrefix(h, "Bearer ")
		claims, err := j.Parse(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
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
		c.Next()
	}
}

// RequireAuth aborts with 401 if no authenticated user is present.
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := c.Get("uid"); !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
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
