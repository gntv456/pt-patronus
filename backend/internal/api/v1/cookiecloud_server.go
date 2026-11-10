// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/cookiecloud"
)

// CookieCloudServerHandler 内置 CookieCloud 服务端：浏览器扩展直接推 cookie 到
// PTPatronus（免单独部署 CookieCloud 容器）。协议与官方一致，扩展零改装：
//
//	POST /api/v1/cookiecloud/update   {"uuid","encrypted","crypto_type"?} → {"action":"done"}
//	GET  /api/v1/cookiecloud/get/:uuid → {"encrypted":"...","crypto_type":"..."}
//
// 公开路由（扩展无登录会话）；鉴权 = uuid+密码（端到端加密，服务端只存密文）。
type CookieCloudServerHandler struct {
	ch *chain.CookieCloudChain
}

// NewCookieCloudServerHandler 构造。
func NewCookieCloudServerHandler(ch *chain.CookieCloudChain) *CookieCloudServerHandler {
	return &CookieCloudServerHandler{ch: ch}
}

// setCCCors 扩展 origin 是 chrome-extension://<id>（每台机器不同，白名单不可枚举），
// 无 cookie 凭证故 ACAO * 安全。官方扩展用 text/plain 发送避免 preflight；
// 若客户端触发 OPTIONS，全局 CORS 中间件已兜住（方法/头均覆盖）。
func setCCCors(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin", "*")
}

// Get GET /get/:uuid（官方扩展「从云端同步」及第三方客户端拉取用）。
func (h *CookieCloudServerHandler) Get(c *gin.Context) {
	setCCCors(c)
	if !h.ch.BuiltinActive() {
		c.String(http.StatusNotFound, "Not Found")
		return
	}
	env, ok := h.ch.ServerEnvelope(c.Param("uuid"))
	if !ok {
		// 官方语义：无数据 404 纯文本（对齐 api/app.js）
		c.String(http.StatusNotFound, "Not Found")
		return
	}
	body, err := cookiecloud.MarshalEnvelope(env)
	if err != nil {
		c.String(http.StatusInternalServerError, "Internal Server Error")
		return
	}
	c.Data(http.StatusOK, "application/json", body)
}

// Update POST /update（官方扩展「上传/定时推送」用）。官方扩展 body 是 gzip 过的 JSON
// （Content-Encoding: gzip），gin 不自动解压，这里手动处理。
func (h *CookieCloudServerHandler) Update(c *gin.Context) {
	setCCCors(c)
	if !h.ch.BuiltinActive() {
		c.JSON(http.StatusNotFound, gin.H{"action": "error", "message": "Not Found"})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 20<<20))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"action": "error", "message": "read body failed"})
		return
	}
	if c.GetHeader("Content-Encoding") == "gzip" {
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"action": "error", "message": "invalid gzip body"})
			return
		}
		defer zr.Close()
		raw, err = io.ReadAll(io.LimitReader(zr, 20<<20))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"action": "error", "message": "decompress failed"})
			return
		}
	}
	if _, err := h.ch.ServerUpdate(raw); err != nil {
		// uuid 不匹配/参数错：官方语义 action=error；不区分原因避免探测
		c.JSON(http.StatusBadRequest, gin.H{"action": "error", "message": "update failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"action": "done"})
}
