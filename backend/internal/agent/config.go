// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package agent 是 PTPatronus 监控 agent 的运行时（独立轻量二进制，跑在被监控机）。
// 通过 WebSocket 长连接主动上报本机指标到 dashboard，并响应服务端的文件管理命令，绕开 SSH 拉取的全部认证痛点。
// 极简依赖：只 import sysinfo（gopsutil 采集）+ gorilla/websocket + 标准库，绝不 import
// internal/monitor（含 gortsplib 等 RTSP 重型依赖）或 internal/filemanager（拉入 oper），否则 agent 二进制体积爆炸。
package agent

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config agent 运行配置（纯环境变量，仿 internal/core/config 的 getenv 模式，但不依赖它以免拉入服务端逻辑）。
type Config struct {
	ServerURL      string        // DASHBOARD_URL，如 ws://1.2.3.4:8088 或 wss://panel.example.com
	Secret         string        // AGENT_SECRET（创建 agent 时下发一次）
	AgentID        uint          // AGENT_ID
	ReportInterval time.Duration // REPORT_INTERVAL，默认 5s（最小 3s）
	AllowedRoots   []string      // ALLOWED_ROOTS，逗号分隔的绝对路径白名单（文件管理深度防御；空=不限，仅靠 base 钳制）
}

// LoadConfig 从环境变量读取；ServerURL/Secret/AgentID 任一缺失返回 ok=false。
func LoadConfig() (Config, bool) {
	interval, _ := strconv.Atoi(getenv("REPORT_INTERVAL", "5"))
	if interval < 3 {
		interval = 5
	}
	cfg := Config{
		ServerURL:      getenv("DASHBOARD_URL", ""),
		Secret:         getenv("AGENT_SECRET", ""),
		AgentID:        getenvUint("AGENT_ID", 0),
		ReportInterval: time.Duration(interval) * time.Second,
		AllowedRoots:   parseRoots(getenv("ALLOWED_ROOTS", "")),
	}
	return cfg, cfg.ServerURL != "" && cfg.Secret != "" && cfg.AgentID != 0
}

// parseRoots 解析逗号分隔的根路径白名单（兼容含冒号的 Windows 盘符路径）。
func parseRoots(s string) []string {
	var roots []string
	for _, r := range strings.Split(s, ",") {
		if r = strings.TrimSpace(r); r != "" {
			roots = append(roots, r)
		}
	}
	return roots
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvUint(key string, def uint) uint {
	if n, err := strconv.ParseUint(os.Getenv(key), 10, 64); err == nil {
		return uint(n)
	}
	return def
}
