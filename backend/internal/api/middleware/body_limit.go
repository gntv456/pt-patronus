// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// MaxBody 限制请求体字节数，防御超大 JSON / body 灌内存的 DoS
// （典型向量：未认证的公开 POST 端点塞 100MB JSON 或数千键的 map）。
//
// 取 64MB 兜底：远大于现有 multipart 上传端点（backup import-ptd 等自带 8MB 限制），
// 不影响正常上传；仅拦截明显恶意的超大 body——超限后 handler 读 body 时收到错误并返回 413/400。
func MaxBody(n int64) gin.HandlerFunc {
	if n <= 0 {
		n = 64 << 20
	}
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}
