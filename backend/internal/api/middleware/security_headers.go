package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders 下发基础安全响应头，缓解点击劫持、MIME 嗅探与 Referrer 泄露。
// 不设严格 CSP：SPA + element-plus 含大量内联样式/脚本，需 nonce/hash 化后再引入，
// 错误的 CSP 反而会打断 UI，故留作后续专项。
func SecurityHeaders() gin.HandlerFunc {
	release := gin.Mode() == gin.ReleaseMode
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("X-XSS-Protection", "0") // 关闭已废弃、自身有缺陷的浏览器 XSS Auditor
		if release {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}
