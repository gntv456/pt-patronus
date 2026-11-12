// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestIDKey 是 gin.Context 里请求 ID 的键。导出供包外消费方（errcode 错误日志
// 等）按同一键取值关联——原 private 导致 500 的 api error 日志无法与访问日志的
// req_id 对上，排障只能靠时间戳猜。
const RequestIDKey = "request_id"

// RequestID 为每个请求注入 X-Request-ID（优先沿用客户端传入），便于全链路追踪。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = randID(8)
		}
		c.Set(RequestIDKey, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

// RequestIDOf 取当前请求的 ID（供 handler 写审计日志）。
func RequestIDOf(c *gin.Context) string {
	if v, ok := c.Get(RequestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// AccessLog 结构化访问日志（含请求 ID / 方法 / 路径 / 状态 / 耗时 / IP）。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("http",
			slog.String("req_id", RequestIDOf(c)),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Int("size", c.Writer.Size()),
			slog.String("ip", c.ClientIP()),
			slog.Duration("dur", time.Since(start)),
		)
	}
}

func randID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
