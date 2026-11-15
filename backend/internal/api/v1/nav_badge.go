// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
)

// NavBadgeHandler 侧栏微角标聚合（GET /nav/badges，侧栏 60s 轮询）。
type NavBadgeHandler struct {
	dash *chain.DashboardChain
}

// NewNavBadgeHandler 构造。
func NewNavBadgeHandler(d *chain.DashboardChain) *NavBadgeHandler {
	return &NavBadgeHandler{dash: d}
}

// Badges 返回各入口角标（Count<=0 的键不返回）。
func (h *NavBadgeHandler) Badges(c *gin.Context) {
	c.JSON(http.StatusOK, h.dash.NavBadges())
}
