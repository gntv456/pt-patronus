package middleware

import (
	"crypto/subtle"
	"net/http"

	"github.com/gin-gonic/gin"
)

// MetricsAuth /metrics 抓取鉴权：Bearer token（Authorization: Bearer <t>）或
// X-API-Key 头，恒定时间比较。token 为空（debug 未配置）时放行——release 模式
// 下 router 已直接不注册该端点，此处不承担 release 防线。
func MetricsAuth(token string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if token == "" {
			c.Next()
			return
		}
		bearer := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if len(bearer) > len(prefix) && bearer[:len(prefix)] == prefix {
			bearer = bearer[len(prefix):]
		} else {
			bearer = ""
		}
		key := c.GetHeader("X-API-Key")
		if subtle.ConstantTimeCompare([]byte(bearer), []byte(token)) == 1 ||
			(key != "" && subtle.ConstantTimeCompare([]byte(key), []byte(token)) == 1) {
			c.Next()
			return
		}
		c.AbortWithStatus(http.StatusUnauthorized)
	}
}
