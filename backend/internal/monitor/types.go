// Package monitor 实现设备监控（服务器/NAS/摄像头）的采集、调度、告警与 RTSP 转发。
// 设计：Collector 接口 + Registry 按 (type,protocol) 分发；Service 单 ticker 扫表编排；
// Alerter 阈值状态转换去重；RTSPProxy 把 RTSP 转 WebSocket 供前端 jessibuca 解码。
package monitor

import "github.com/hxpt/ptpatronus/internal/sysinfo"

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
	if decodeJSON(raw, &out) != nil {
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
	return out
}

// MetricsJSON 序列化 Metrics 为 JSON 字符串（存 MetricSnapshot.Metrics）。
func MetricsJSON(m Metrics) string { return encodeJSON(m) }

// Credentials 是解密后的凭据。由 Oper 解密注入，Collector 不碰加密。
type Credentials struct {
	Username   string
	Password   string // SSH 密码 / 群晖密码 / TrueNAS token / ONVIF 密码
	PrivateKey string // SSH 私钥 PEM（无口令）；非空时 dialSSH 优先用 publickey
}
