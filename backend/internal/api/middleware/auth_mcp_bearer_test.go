// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/core/security"
)

// MCP 客户端按文档用 Authorization: Bearer <插件 token>。JWTAuth 若把它当 JWT 解析，
// /api/v1/mcp 永远回 invalid_token，整个 MCP 功能不可用（实测）。MCP 路径的 Bearer
// 必须原样放行给 mcp.Server 自己校验；其余路径行为不变（正控）。
func TestMCPBearerPassesThroughJWTAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	j := security.NewJWT("middleware-test-seed", 60)
	r := gin.New()
	r.Use(JWTAuth(j, "", nil))
	r.POST("/api/v1/mcp", func(c *gin.Context) {
		_, hasUID := c.Get("uid")
		c.JSON(http.StatusOK, gin.H{"reached": true, "has_uid": hasUID})
	})
	r.GET("/api/v1/sites", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"reached": true})
	})

	do := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer smoke-mcp-token")
		r.ServeHTTP(rec, req)
		return rec
	}

	mcp := do(http.MethodPost, "/api/v1/mcp")
	if mcp.Code != http.StatusOK {
		t.Fatalf("MCP 的 Bearer 不应被 JWT 层拦下, got %d body=%s", mcp.Code, mcp.Body.String())
	}
	if !strings.Contains(mcp.Body.String(), `"reached":true`) {
		t.Fatalf("请求未到达 MCP 处理器, body=%s", mcp.Body.String())
	}
	if strings.Contains(mcp.Body.String(), `"has_uid":true`) {
		t.Fatalf("MCP 路径不应被注入 JWT 身份, body=%s", mcp.Body.String())
	}

	other := do(http.MethodGet, "/api/v1/sites")
	if other.Code != http.StatusUnauthorized {
		t.Fatalf("正控：普通路径上的非 JWT Bearer 仍应 401, got %d body=%s", other.Code, other.Body.String())
	}
}
