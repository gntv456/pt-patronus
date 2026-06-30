package middleware

import "github.com/gin-gonic/gin"

// CORS 按允许的来源列表设置跨域头。
//   - 无参或含 "*"：开发模式全开（Access-Control-Allow-Origin: *）
//   - 否则：仅当请求 Origin 命中列表时回显该 Origin（生产收敛）
func CORS(allowed ...string) gin.HandlerFunc {
	allowAll := len(allowed) == 0
	set := make(map[string]bool, len(allowed))
	for _, o := range allowed {
		if o == "*" {
			allowAll = true
		}
		set[o] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		credentialed := false
		switch {
		case allowAll:
			// 开发全开：ACAO:*。注意 ACAO:* 与 Allow-Credentials:true 是浏览器禁止的组合，
			// 且本项目鉴权走 Authorization header（非 cookie），开发模式无需凭据跨域 → 不设 ACAC。
			c.Header("Access-Control-Allow-Origin", "*")
		case origin != "" && set[origin]:
			// 生产白名单命中：回显具体 origin + 允许凭据（供未来 cookie 会话用）。
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			credentialed = true
		default:
			// origin 不在白名单：不发 ACAO，浏览器即拒跨域（最严）。
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-API-Key, X-Request-ID, X-Device-ID")
		if credentialed {
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		c.Header("Access-Control-Max-Age", "86400")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
