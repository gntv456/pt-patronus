package v1

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
)

// Sessions 返回目标主机的活跃 SSH 会话列表（who 输出解析）。
func (h *SshAdminHandler) Sessions(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	// w -h 输出：USER TTY FROM LOGIN@ IDLE JCPU PCPU WHAT
	cmd := `w -h 2>/dev/null`
	r, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 10)
	if err != nil {
		respondOpsTargetError(c, err)
		return
	}
	resp := sshSessionListResp{Sessions: []sshSessionRow{}}
	for _, line := range strings.Split(r.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		// w -h 格式：USER TTY FROM LOGIN@ IDLE [JCPU PCPU] WHAT...
		// FROM 可能是 "-"（本地登录）
		s := sshSessionRow{
			User:  f[0],
			TTY:   f[1],
			From:  f[2],
			Login: f[3],
			Idle:  f[4],
		}
		// WHAT 字段（可能有也可能没有，从第 7 列开始）
		if len(f) > 7 {
			s.What = strings.Join(f[7:], " ")
		} else if len(f) == 7 {
			s.What = f[6]
		}
		// 查会话 PID（用 pts 名查）——用于断开
		if strings.HasPrefix(s.TTY, "pts") || strings.HasPrefix(s.TTY, "tty") {
			pidCmd := "ps -o pid= -t " + s.TTY + " 2>/dev/null | head -1"
			pr, _ := h.runner.RunCommandNow(c.Request.Context(), id, pidCmd, 5)
			pidStr := strings.TrimSpace(pr.Stdout)
			if pid, err := strconv.ParseUint(pidStr, 10, 32); err == nil {
				s.PID = uint32(pid)
			}
		}
		resp.Sessions = append(resp.Sessions, s)
	}
	c.JSON(http.StatusOK, resp)
}

// KillSession 断开指定 SSH 会话（kill 会话 shell 进程）。
func (h *SshAdminHandler) KillSession(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	pidStr := c.Param("pid")
	pid, err := strconv.ParseUint(pidStr, 10, 32)
	if err != nil || pid <= 2 {
		errcode.Respond(c, errcode.InvalidID)
		return
	}
	cmd := "kill -TERM " + pidStr + " 2>&1"
	r, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 10)
	if err != nil {
		respondOpsTargetError(c, err)
		return
	}
	if !r.Success {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": r.Stderr})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "pid": pid})
}

// Logs 返回 SSH 登录日志（journalctl 解析）。
func (h *SshAdminHandler) Logs(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	// 优先 journalctl 查 sshd 的 Accepted/Failed；回退 last
	lines := c.DefaultQuery("lines", "100")
	n, err := strconv.Atoi(lines)
	if err != nil || n <= 0 || n > 1000 {
		n = 100
	}
	// journalctl 抓 sshd 单元的 Accepted/Failed 行
	cmd := "journalctl -u sshd -u ssh --no-pager -n " + strconv.Itoa(n) + ` 2>/dev/null | grep -E 'Accepted|Failed|Disconnected' || last -n ` + strconv.Itoa(n)
	r, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 15)
	if err != nil {
		respondOpsTargetError(c, err)
		return
	}
	resp := sshLogListResp{Logs: []sshLogRow{}}
	for _, line := range strings.Split(r.Stdout, "\n") {
		// 匹配 "Accepted password/publickey for USER from IP port PORT ssh2"
		// 或 "Failed password for invalid user USER from IP port PORT ssh2"
		row, ok := parseSSHLogLine(line)
		if ok {
			resp.Logs = append(resp.Logs, row)
		}
	}
	c.JSON(http.StatusOK, resp)
}

// parseSSHLogLine 解析单条 SSH 日志行。
func parseSSHLogLine(line string) (sshLogRow, bool) {
	// 提取类型
	logType := ""
	if strings.Contains(line, "Accepted") {
		logType = "Accepted"
	} else if strings.Contains(line, "Failed") {
		logType = "Failed"
	} else if strings.Contains(line, "Disconnected") {
		logType = "Disconnected"
	} else {
		return sshLogRow{}, false
	}

	row := sshLogRow{Type: logType}

	// 提取时间戳（journalctl 行首的日期时间）
	// 格式："MMM DD HH:MM:SS hostname sshd[pid]: ..."
	// 或 journalctl ISO 格式 "2024-01-01 12:00:00 +0800 ..."
	timePart := ""
	if idx := strings.Index(line, "sshd["); idx > 0 {
		timePart = strings.TrimSpace(line[:idx])
		// 只保留日期时间部分（去掉主机名）
		if fields := strings.Fields(timePart); len(fields) >= 3 {
			timePart = strings.Join(fields[len(fields)-3:], " ")
		}
	}
	row.Time = timePart

	// 提取认证方式
	if strings.Contains(line, "publickey") {
		row.Method = "publickey"
	} else if strings.Contains(line, "password") {
		row.Method = "password"
	}

	// 提取用户： "for USER" 或 "for invalid user USER"
	if idx := strings.Index(line, "for invalid user "); idx >= 0 {
		rest := line[idx+len("for invalid user "):]
		row.User = strings.Fields(rest)[0]
	} else if idx := strings.Index(line, "for "); idx >= 0 {
		rest := line[idx+len("for "):]
		row.User = strings.Fields(rest)[0]
	}

	// 提取来源： "from IP"
	if idx := strings.Index(line, "from "); idx >= 0 {
		rest := line[idx+len("from "):]
		row.From = strings.Fields(rest)[0]
	}

	// 提取端口： "port PORT"
	if idx := strings.Index(line, "port "); idx >= 0 {
		rest := line[idx+len("port "):]
		row.Port = strings.Fields(rest)[0]
	}

	return row, true
}
