// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"fmt"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// PID 黑名单：禁止结束关键系统进程（防止误杀导致主机失联）。
// PID 1=init/systemd，PID 2=kthreadd。
var protectedPIDs = map[uint]bool{0: true, 1: true, 2: true}

// Kill 结束目标主机上的指定进程（kill/kill -9）。
func (h *ProcHandler) Kill(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in procKillRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	if !in.Confirm {
		errcode.Respond(c, errcode.ConfirmRequired)
		return
	}
	if protectedPIDs[in.PID] {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("PID %d is protected (kernel/init), cannot kill", in.PID)})
		return
	}

	sig := "TERM"
	if in.Force {
		sig = "KILL"
	}
	// 用 kill 内置信号名，避免任意信号注入；PID 走 strconv 防 shell 注入。
	cmd := fmt.Sprintf("kill -%s %s", sig, strconv.FormatUint(uint64(in.PID), 10))
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 10)
	if err != nil {
		respondOpsTargetError(c, err)
		return
	}
	if !result.Success {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": result.Stderr, "stdout": result.Stdout})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "pid": in.PID, "signal": sig})
}
