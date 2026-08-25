package v1

import (
	"context"
	"net/http"
	"os"
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
	cli, err := h.hub.FileClient(uint(id))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	cmd := agentrpc.Command{Op: "upgrade", Dst: url, Checksum: os.Getenv("AGENT_CHECKSUM")}
	if err := cli.Upgrade(ctx, cmd); err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
