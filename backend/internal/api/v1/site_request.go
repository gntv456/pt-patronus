// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
)

// RequestHandler 站点求种请求。
type RequestHandler struct {
	ch *chain.SiteChain
}

// NewSiteRequestHandler 构造。
func NewSiteRequestHandler(ch *chain.SiteChain) *RequestHandler { return &RequestHandler{ch: ch} }

// All GET /site-requests?filled=1&site_id= —— 跨站聚合求种列表（site_id 指定单站）。
func (h *RequestHandler) All(c *gin.Context) {
	filled := c.Query("filled") == "1"
	if raw := c.Query("site_id"); raw != "" {
		sid, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || sid == 0 {
			errcode.Respond(c, errcode.InvalidID)
			return
		}
		s, err := h.ch.SiteOper().Get(uint(sid))
		if err != nil {
			errcode.Respond(c, errcode.SiteNotFound)
			return
		}
		r, err := h.ch.Requests(c.Request.Context(), s, filled)
		if err != nil && r == nil {
			// #399 归统一判据：Requests 链层第一步也是 credentialWithErr，
			// 主密钥换过/站点建在内网两种「重试不会好」的失败此前都被这句
			// BadGateway.WithErr 整段吞进日志（界面只见「上游服务不可用，请稍后重试」）。
			respondFetchError(c, err, errcode.BadGateway)
			return
		}
		c.JSON(http.StatusOK, gin.H{"sites": []*chain.RequestResult{r}})
		return
	}
	results, err := h.ch.RequestsAll(c.Request.Context(), filled)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	if results == nil {
		results = []chain.RequestResult{}
	}
	c.JSON(http.StatusOK, gin.H{"sites": results})
}
