// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"fmt"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// SshAdminHandler 通过 SSH 远程通道管理目标主机的 SSH 服务本身。
// 注意：这是"用 SSH 管 SSH"——经 RunCommandNow 在被控机执行 systemctl/sshd_config 操作。
// 若操作导致 SSH 服务停止，当前连接可能中断（与 1Panel 行为一致，需用户知晓风险）。
type SshAdminHandler struct {
	runner DockerCommandRunner
}

func NewSshAdminHandler(svc DockerCommandRunner) *SshAdminHandler {
	return &SshAdminHandler{runner: svc}
}

var portRe = regexp.MustCompile(`^[1-9][0-9]{1,4}$`) // 1-65535
var listenRe = regexp.MustCompile(`^[0-9.*]+$`)      // IP 或 *

// detectServiceName 探测目标主机的 SSH 服务名（sshd 或 ssh）。
func (h *SshAdminHandler) detectServiceName(c *gin.Context, id uint) string {
	cmd := `systemctl list-unit-files 2>/dev/null | grep -oE 'ssh(d)?\.service' | head -1`
	r, _ := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 10)
	name := strings.TrimSpace(r.Stdout)
	if name == "" {
		return "sshd" // 默认
	}
	return name
}

// Status 返回 SSH 服务状态（运行/自启/端口/版本）。
func (h *SshAdminHandler) Status(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	svcName := h.detectServiceName(c, id)
	st := sshServiceStatus{ServiceName: svcName}

	// 运行状态 + 自启
	cmd := fmt.Sprintf(`systemctl is-active %s 2>/dev/null; echo "---"; systemctl is-enabled %s 2>/dev/null`, svcName, svcName)
	r, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 10)
	if err != nil {
		respondOpsTargetError(c, err)
		return
	}
	parts := strings.SplitN(r.Stdout, "---", 2)
	if len(parts) == 2 {
		state := strings.TrimSpace(parts[0])
		st.State = state
		st.Active = state == "active"
		st.Enabled = strings.TrimSpace(parts[1]) == "enabled"
	}

	// 端口（从 sshd_config 解析，fallback 22）
	portCmd := `grep -iE '^Port\s' /etc/ssh/sshd_config 2>/dev/null | awk '{print $2}' | head -1`
	pr, _ := h.runner.RunCommandNow(c.Request.Context(), id, portCmd, 10)
	st.Port = strings.TrimSpace(pr.Stdout)
	if st.Port == "" {
		st.Port = "22"
	}

	// 监听地址
	addrCmd := `grep -iE '^ListenAddress\s' /etc/ssh/sshd_config 2>/dev/null | awk '{print $2}' | head -1`
	ar, _ := h.runner.RunCommandNow(c.Request.Context(), id, addrCmd, 10)
	st.ListenAddr = strings.TrimSpace(ar.Stdout)
	if st.ListenAddr == "" {
		st.ListenAddr = "0.0.0.0"
	}

	// 版本
	verCmd := `sshd -V 2>&1 | head -1 || ssh -V 2>&1 | head -1`
	vr, _ := h.runner.RunCommandNow(c.Request.Context(), id, verCmd, 10)
	st.Version = strings.TrimSpace(vr.Stdout)

	c.JSON(http.StatusOK, st)
}

// Action 执行 SSH 服务操作（start/stop/restart/reload）。
func (h *SshAdminHandler) Action(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in sshActionRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	// 白名单校验
	switch in.Action {
	case "start", "stop", "restart", "reload":
	default:
		errcode.Respond(c, errcode.InvalidRequest)
		return
	}
	svcName := h.detectServiceName(c, id)
	// stop 操作不强制 confirm（1Panel 允许），但提示风险
	cmd := fmt.Sprintf(`systemctl %s %s 2>&1`, in.Action, svcName)
	r, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 20)
	if err != nil {
		respondOpsTargetError(c, err)
		return
	}
	if !r.Success {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": r.Stderr, "stdout": r.Stdout})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "action": in.Action})
}

// ToggleEnable 开启/关闭 SSH 开机自启。
func (h *SshAdminHandler) ToggleEnable(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	enable := c.Query("enable") == "true"
	svcName := h.detectServiceName(c, id)
	op := "disable"
	if enable {
		op = "enable"
	}
	cmd := fmt.Sprintf(`systemctl %s %s 2>&1`, op, svcName)
	r, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 10)
	if err != nil {
		respondOpsTargetError(c, err)
		return
	}
	if !r.Success {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": r.Stderr})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "enabled": enable})
}

// Config 更新 SSH 配置（端口/监听地址），并 reload。
func (h *SshAdminHandler) Config(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var in sshConfigRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		respondBindError(c, err)
		return
	}
	if !in.Confirm {
		errcode.Respond(c, errcode.ConfirmRequired)
		return
	}
	if in.Port != "" && !portRe.MatchString(in.Port) {
		errcode.Respond(c, errcode.InvalidRequest)
		return
	}
	if in.ListenAddr != "" && !listenRe.MatchString(in.ListenAddr) {
		errcode.Respond(c, errcode.InvalidRequest)
		return
	}
	// 用 sed 改 sshd_config，备份原文件
	var steps []string
	steps = append(steps, `cp /etc/ssh/sshd_config /etc/ssh/sshd_config.bak.$(date +%s)`)
	if in.Port != "" {
		// 先删已有 Port 行，再追加
		steps = append(steps, fmt.Sprintf(`sed -i 's/^\(#\)\?Port.*/Port %s/' /etc/ssh/sshd_config`, in.Port))
	}
	if in.ListenAddr != "" {
		steps = append(steps, fmt.Sprintf(`sed -i 's/^\(#\)\?ListenAddress.*/ListenAddress %s/' /etc/ssh/sshd_config`, in.ListenAddr))
	}
	svcName := h.detectServiceName(c, id)
	// 改完先验证配置语法，再 reload
	steps = append(steps, fmt.Sprintf(`sshd -t 2>&1 && systemctl reload %s 2>&1`, svcName))
	script := strings.Join(steps, " && ")
	r, err := h.runner.RunCommandNow(c.Request.Context(), id, script, 20)
	if err != nil {
		respondOpsTargetError(c, err)
		return
	}
	if !r.Success {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": r.Stderr, "stdout": r.Stdout, "hint": "配置可能无效，已备份原文件到 sshd_config.bak"})
		return
	}
	_ = strconv.Itoa // keep import
	c.JSON(http.StatusOK, gin.H{"ok": true, "port": in.Port, "listen_addr": in.ListenAddr})
}
