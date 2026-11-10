// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/agentrpc"
)

// agent_upgrade.go — agent 自更新 handler。
// AGENT_DOWNLOAD_URL：agent 二进制下载基址（含 {arch} 占位，agent 端替换为 GOARCH）。
//
//	默认 GitHub Release latest：https://github.com/gntv456/pt-patronus/releases/latest/download/ptpatronus-agent-linux-{arch}
//
// AGENT_LATEST_VERSION：最新 agent 版本号（前端展示+判断是否需升级）；默认读 AGENT_DOWNLOAD_URL 推断。
const defaultAgentDownloadURL = "https://github.com/gntv456/pt-patronus/releases/latest/download/ptpatronus-agent-linux-{arch}"

// LatestAgentVersion 返回最新 agent 版本号 + 下载地址（供前端展示）。
func (h *AgentHandler) LatestAgentVersion(c *gin.Context) {
	ver := os.Getenv("AGENT_LATEST_VERSION")
	if ver == "" {
		ver = "unknown"
	}
	url := os.Getenv("AGENT_DOWNLOAD_URL")
	if url == "" {
		url = defaultAgentDownloadURL
	}
	c.JSON(http.StatusOK, gin.H{"version": ver, "download_url": url})
}

// Upgrade 触发指定 agent 自更新：通过 WS 下发 upgrade 命令。
// agent 收到后下载新二进制、sha256 校验、原子替换、退出（systemd 重启）。
func (h *AgentHandler) Upgrade(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	url := os.Getenv("AGENT_DOWNLOAD_URL")
	if url == "" {
		url = defaultAgentDownloadURL
	}
	cmd, err := agentUpgradeCommand(url)
	if err != nil {
		// 没配校验值就别把注定被拒的命令发出去：agent 侧会拒（无校验=把执行权交给下载源），
		// 但那边只会回一句「缺少 sha256 校验值」，看不出是服务端少配了环境变量、该去哪儿补。
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": userErrText(err)})
		return
	}
	cli, err := h.hub.FileClient(uint(id))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": userErrText(err)})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := cli.Upgrade(ctx, cmd); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": userErrText(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// agentUpgradeCommand 组装下发自更新命令。AGENT_CHECKSUM 支持两种写法：
// 单个 sha256（所有 agent 同架构时）或 "amd64=<hex> arm64=<hex>"（按架构分别配置，agent 端自选）。
// 空值一律不下发：agent 侧必然拒绝（拒绝无校验的二进制替换），提前失败才能给出可操作原因。
func agentUpgradeCommand(downloadURL string) (agentrpc.Command, error) {
	sum := strings.TrimSpace(os.Getenv("AGENT_CHECKSUM"))
	if sum == "" {
		return agentrpc.Command{}, fmt.Errorf("服务端未配置 AGENT_CHECKSUM，已拒绝下发无校验的自更新：请填写目标二进制的 sha256，多架构可写成「amd64=<hex> arm64=<hex>」")
	}
	return agentrpc.Command{Op: "upgrade", Dst: downloadURL, Checksum: sum}, nil
}
