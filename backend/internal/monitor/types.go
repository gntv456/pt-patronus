// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package monitor 实现设备监控（服务器/NAS/摄像头）的采集、调度、告警与 RTSP 转发。
// 设计：Collector 接口 + Registry 按 (type,protocol) 分发；Service 单 ticker 扫表编排；
// Alerter 阈值状态转换去重；RTSPProxy 把 RTSP 转 WebSocket 供前端 jessibuca 解码。
package monitor

import (
	"log/slog"
	"strings"

	"github.com/hxpt/ptpatronus/internal/sysinfo"
)

// 指标类型复用 sysinfo 包的唯一定义（类型别名），避免 local 采集器与 agent 二进制重复维护；
// agent 上报的 JSON 在服务端 json.Unmarshal 直接映射回这些类型。
type (
	Metrics    = sysinfo.Metrics
	DiskMetric = sysinfo.DiskMetric
	TempMetric = sysinfo.TempMetric
	DeviceInfo = sysinfo.DeviceInfo
)

// Thresholds 阈值规则。序列化为 JSON 存 MonitorTarget.Thresholds；空字段取默认。
type Thresholds struct {
	CPUUsage  float64 `json:"cpu_usage,omitempty"`  // 默认 90
	MemUsage  float64 `json:"mem_usage,omitempty"`  // 默认 90
	DiskUsage float64 `json:"disk_usage,omitempty"` // 默认 90
	Temp      float64 `json:"temp,omitempty"`       // 默认 75
	Offline   *bool   `json:"offline,omitempty"`    // 默认 true（离线即告警）

	HTTPMethod      string  `json:"http_method,omitempty"`       // GET|HEAD
	HTTPExpectedMin int     `json:"http_expected_min,omitempty"` // 默认 200
	HTTPExpectedMax int     `json:"http_expected_max,omitempty"` // 默认 399
	ResponseMS      float64 `json:"response_ms,omitempty"`       // HTTP/TCP/脚本响应耗时阈值（ms）
	ProcessName     string  `json:"process_name,omitempty"`
	ProcessMinCount int     `json:"process_min_count,omitempty"` // 默认 1
	ScriptCommand   string  `json:"script_command,omitempty"`
}

// DefaultThresholds 返回内置默认阈值。
func DefaultThresholds() Thresholds {
	t := true
	return Thresholds{CPUUsage: 90, MemUsage: 90, DiskUsage: 90, Temp: 75, Offline: &t}
}

// ParseThresholds 解析存储的 JSON；空串或解析失败回退默认值。
func ParseThresholds(raw string) Thresholds {
	t := DefaultThresholds()
	if raw == "" {
		return t
	}
	// 用默认值打底，再反序列化覆盖（0 值字段保留默认）。
	out := DefaultThresholds()
	if err := decodeJSON(raw, &out); err != nil {
		// 回默认本身是设计（缺省项要能用），但「整份读不出来」必须留痕：
		// 用户以为设了 50% 而实际按 90% 报警，除了这条 Warn 无从发现是哪台设备的 JSON 被写坏。
		slog.Warn("monitor thresholds unreadable, using built-in defaults",
			"raw", raw[:min(len(raw), 120)], "err", err)
		return t
	}
	if out.CPUUsage == 0 {
		out.CPUUsage = t.CPUUsage
	}
	if out.MemUsage == 0 {
		out.MemUsage = t.MemUsage
	}
	if out.DiskUsage == 0 {
		out.DiskUsage = t.DiskUsage
	}
	if out.Temp == 0 {
		out.Temp = t.Temp
	}
	if out.Offline == nil {
		o := true
		out.Offline = &o
	}
	if out.HTTPExpectedMin == 0 {
		out.HTTPExpectedMin = 200
	}
	if out.HTTPExpectedMax == 0 {
		out.HTTPExpectedMax = 399
	}
	if out.ProcessMinCount == 0 {
		out.ProcessMinCount = 1
	}
	return out
}

// ThresholdsReadable 判阈值 JSON 能否真被 ParseThresholds 读出来：与消费方同一张表
// （顶层须为对象；空串=交给内置默认，属合法值）。写入侧用它做边界校验，
// 免得在 oper 再抄一遍 Unmarshal——同一判据抄第二遍的那份会先漂移。
func ThresholdsReadable(raw string) bool {
	if strings.TrimSpace(raw) == "" {
		return true
	}
	var probe map[string]any
	return decodeJSON(raw, &probe) == nil
}

// MetricsJSON 序列化 Metrics 为 JSON 字符串（存 MetricSnapshot.Metrics）。
func MetricsJSON(m Metrics) string { return encodeJSON(m) }

// Credentials 是解密后的凭据。由 Oper 解密注入，Collector 不碰加密。
type Credentials struct {
	Username   string
	Password   string // SSH 密码 / 群晖密码 / TrueNAS token / ONVIF 密码
	PrivateKey string // SSH 私钥 PEM（无口令）；非空时 dialSSH 优先用 publickey
}
