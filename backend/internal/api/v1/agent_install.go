// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed agent_install.sh
var agentInstallScript string

// ServeAgentInstall 返回 agent 一键安装脚本（公开文本，不含 secret；secret 由 env 传入，不入 URL/日志）。
// 路由放 guarded 外：安装时在被监控机 curl 执行，无用户 JWT。
func ServeAgentInstall(c *gin.Context) {
	c.Data(http.StatusOK, "text/x-shellscript; charset=utf-8", []byte(agentInstallScript))
}
