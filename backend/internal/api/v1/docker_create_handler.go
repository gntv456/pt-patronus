// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// LocalCreateContainer 创建容器（本机模式，简化表单）。
func (h *DockerHandler) LocalCreateContainer(c *gin.Context) {
	if h.local == nil {
		errcode.Respond(c, errcode.DockerLocalNotReady)
		return
	}
	var in simpleCreateReq
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	if in.RestartPolicy == "" {
		in.RestartPolicy = "no"
	}
	id, err := h.local.LocalCreateFromSimple(c.Request.Context(), in)
	if err != nil {
		errcode.Respond(c, errcode.BadGateway.WithErr(err))
		return
	}
	// 自动启动：容器已经建好，不能因为「没起来」就把整次创建判失败（更不该偷偷删掉），
	// 但也不能把失败丢掉——从前是 `_ =`，勾了「创建后自动启动」的人看到的永远是创建成功。
	resp := gin.H{"ok": true, "id": id}
	if in.AutoStart && id != "" {
		if serr := h.local.LocalContainerAction(c.Request.Context(), id, "start"); serr != nil {
			slog.Warn("docker auto start failed", "id", id, "err", serr)
			resp["started"] = false
			resp["start_error"] = userErrText(serr)
		} else {
			resp["started"] = true
		}
	}
	c.JSON(http.StatusOK, resp)
}
