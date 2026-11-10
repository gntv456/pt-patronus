// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ProcHandler 通过 SSH 远程通道在被控机上查询进程与网络连接。
// 复用 RunCommandNow（与 Docker/Disk 一致），不直接碰宿主。
type ProcHandler struct {
	runner DockerCommandRunner
}

func NewProcHandler(svc DockerCommandRunner) *ProcHandler {
	return &ProcHandler{runner: svc}
}

// List 返回目标主机的进程列表（ps 解析）。
func (h *ProcHandler) List(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	// ps -e 全进程；输出 JSON 友好的列。--no-headers 去表头。
	// etime=累计运行；rss=KB；%cpu/%mem 已归一化。
	cmd := `ps -eo pid,ppid,user,stat,pcpu,pmem,rss,tty,etime,comm --no-headers`
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 15)
	if err != nil {
		respondOpsTargetError(c, err)
		return
	}
	if !result.Success {
		c.JSON(http.StatusBadGateway, gin.H{"error": result.Stderr, "stdout": result.Stdout})
		return
	}
	resp := procListResp{Processes: []procRow{}}
	for _, line := range strings.Split(result.Stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// ps 输出定宽列，用 strings.Fields 拆分；comm 可能含空格，取最后一段作为名称。
		// 为拿完整命令行，额外查 /proc/<pid>/cmdline 成本太高，这里用 comm 截断即可。
		f := strings.Fields(line)
		if len(f) < 9 {
			continue
		}
		pid, _ := strconv.ParseUint(f[0], 10, 32)
		ppid, _ := strconv.ParseUint(f[1], 10, 32)
		cpu, _ := strconv.ParseFloat(f[4], 64)
		mem, _ := strconv.ParseFloat(f[5], 64)
		rss, _ := strconv.ParseUint(f[6], 10, 64)
		// f[7]=tty, f[8]=etime, f[9...]=comm（可能含空格）
		tty := f[7]
		etime := f[8]
		comm := strings.Join(f[9:], " ")
		resp.Processes = append(resp.Processes, procRow{
			PID:     uint32(pid),
			PPID:    uint32(ppid),
			User:    f[2],
			Name:    comm,
			Cmd:     comm,
			State:   f[3],
			CPUPerc: cpu,
			MemPerc: mem,
			RSS:     rss,
			TTY:     tty,
			Time:    etime,
		})
	}
	c.JSON(http.StatusOK, resp)
}

// Connections 返回目标主机的网络连接列表（ss 解析）。
func (h *ProcHandler) Connections(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	// ss -tunap：tcp/udp 全状态 + 进程信息。输出 JSON 解析。
	cmd := `ss -tunapH --json 2>/dev/null || ss -tunapH`
	result, err := h.runner.RunCommandNow(c.Request.Context(), id, cmd, 15)
	if err != nil {
		respondOpsTargetError(c, err)
		return
	}
	resp := netConnListResp{Connections: []netConnRow{}}

	// 优先尝试 JSON 解析（ss --json）
	var ssJSON []struct {
		Family string `json:"family"`
		Type   string `json:"type"`
		State  string `json:"state"`
		Laddr  struct {
			IP   string `json:"ip"`
			Port int    `json:"port"`
		} `json:"local_address"`
		Paddr struct {
			IP   string `json:"ip"`
			Port int    `json:"port"`
		} `json:"peer_address"`
		Process string `json:"process"`
	}
	// ss --json 顶层是对象 { "endpoints": [...] } 或数组，兼容两种。
	var wrapper struct {
		Endpoints json.RawMessage `json:"endpoints"`
	}
	_ = json.Unmarshal([]byte(result.Stdout), &wrapper) // 顶层形状探测，失败走 else 分支
	if len(wrapper.Endpoints) > 0 {
		_ = json.Unmarshal(wrapper.Endpoints, &ssJSON)
	} else {
		_ = json.Unmarshal([]byte(result.Stdout), &ssJSON)
	}

	if len(ssJSON) > 0 {
		for _, e := range ssJSON {
			row := netConnRow{
				Proto:     familyType(e.Family, e.Type),
				State:     e.State,
				LocalAddr: e.Laddr.IP,
				LocalPort: strconv.Itoa(e.Laddr.Port),
				PeerAddr:  e.Paddr.IP,
				PeerPort:  strconv.Itoa(e.Paddr.Port),
				Process:   parseSSProcess(e.Process),
			}
			row.PID, _ = parseSSPID(e.Process)
			resp.Connections = append(resp.Connections, row)
		}
	} else {
		// 回退：纯文本 ss -tunapH 解析
		resp.Connections = parseSSLines(result.Stdout)
	}
	c.JSON(http.StatusOK, resp)
}

// familyType 把 ss 的 family+type 转成 proto 字符串。
func familyType(family, typ string) string {
	if family == "" {
		return "tcp"
	}
	proto := "tcp"
	if strings.HasPrefix(typ, "dgram") {
		proto = "udp"
	}
	if strings.Contains(family, "inet6") {
		proto += "6"
	}
	return proto
}

// parseSSProcess 从 ss 的 process 字段提取进程名（格式 "users:((\"nginx\",pid=123,fd=6))"）。
func parseSSProcess(s string) string {
	i := strings.Index(s, "(\"")
	if i < 0 {
		return ""
	}
	rest := s[i+2:]
	j := strings.Index(rest, "\"")
	if j < 0 {
		return ""
	}
	return rest[:j]
}

// parseSSPID 从 ss 的 process 字段提取 PID。
func parseSSPID(s string) (uint32, error) {
	i := strings.Index(s, "pid=")
	if i < 0 {
		return 0, nil
	}
	rest := s[i+4:]
	end := strings.IndexAny(rest, ",)")
	num := rest
	if end >= 0 {
		num = rest[:end]
	}
	n, err := strconv.ParseUint(num, 10, 32)
	return uint32(n), err
}

// parseSSLines 纯文本 ss 输出回退解析。
func parseSSLines(raw string) []netConnRow {
	var out []netConnRow
	for i, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if i == 0 || line == "" {
			continue // 跳过表头
		}
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		row := netConnRow{
			Proto:     f[0],
			State:     f[1],
			LocalAddr: f[4],
		}
		if len(f) > 5 {
			row.PeerAddr = f[5]
		}
		// 端口是 addr 最后一个 : 后的部分
		if li := strings.LastIndex(row.LocalAddr, ":"); li >= 0 {
			row.LocalPort = row.LocalAddr[li+1:]
		}
		if li := strings.LastIndex(row.PeerAddr, ":"); li >= 0 {
			row.PeerPort = row.PeerAddr[li+1:]
		}
		out = append(out, row)
	}
	return out
}
