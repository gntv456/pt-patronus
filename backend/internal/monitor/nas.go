// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// NASCollector 按 target.Vendor 分发到具体厂商采集器（群晖 / TrueNAS / SNMP）。
type NASCollector struct {
	hc *httpclient.Client
}

// NewNASCollector 构造；hc 用于群晖/TrueNAS 的 HTTP API（SNMP 走 gosnmp 不经 hc）。
func NewNASCollector(hc *httpclient.Client) NASCollector { return NASCollector{hc: hc} }

func (NASCollector) String() string { return "nas" }

func (c NASCollector) Probe(ctx context.Context, t *models.MonitorTarget, creds Credentials) (bool, error) {
	m, err := c.collect(ctx, t, creds)
	if err != nil {
		return false, err
	}
	return m.Online, nil
}

func (c NASCollector) Collect(ctx context.Context, t *models.MonitorTarget, creds Credentials) (Metrics, error) {
	return c.collect(ctx, t, creds)
}

func (c NASCollector) Power(ctx context.Context, t *models.MonitorTarget, creds Credentials, action string) error {
	action = strings.ToLower(strings.TrimSpace(action))
	if !isSupportedPowerAction(action) {
		return fmt.Errorf("%w: %s", ErrUnsupportedPowerAction, action)
	}
	var perr error
	switch strings.ToLower(t.Vendor) {
	case "truenas", "freenas":
		perr = powerTrueNAS(ctx, c.hc, t, creds, action)
	case "snmp":
		return fmt.Errorf("%w for nas/snmp", ErrUnsupportedPowerAction)
	default:
		perr = powerSynology(ctx, c.hc, t, creds, action)
	}
	// NAS 的 reboot/shutdown 一生效就会掐断正在等回执的 HTTP 连接，
	// 那类传输层错误按「已下发」处理（判据与 SSH 通道共用一份）。
	if perr != nil && powerDispatchConfirmed(perr) {
		slog.Info("nas power action lost the reply (expected)", "target", t.Name, "action", action, "err", perr)
		return nil
	}
	return perr
}

func (c NASCollector) collect(ctx context.Context, t *models.MonitorTarget, creds Credentials) (Metrics, error) {
	switch strings.ToLower(t.Vendor) {
	case "truenas", "freenas":
		return collectTrueNAS(ctx, c.hc, t, creds)
	case "snmp":
		return collectSNMP(ctx, t, creds)
	default: // 默认 synology
		return collectSynology(ctx, c.hc, t, creds)
	}
}

// nasTimeout 取采集超时（默认 15s）。
func nasTimeout(t *models.MonitorTarget) time.Duration {
	sec := t.TimeoutSec
	if sec < 5 {
		sec = 15
	}
	return time.Duration(sec) * time.Second
}

// normalizeBase 补全 scheme 并去尾斜杠。Host 可为 "192.168.1.2:5000" 或 "https://nas..."。
func normalizeBase(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "/")
	if raw != "" && !strings.Contains(raw, "://") {
		if isLikelyHTTPSPort(raw) {
			raw = "https://" + raw
		} else {
			raw = "http://" + raw
		}
	}
	return raw
}

// isLikelyHTTPSPort judges whether host[:port] should use HTTPS (avoid plaintext credentials).
// 443 (generic HTTPS), 5001 (Synology HTTPS), 8443 (common HTTPS alt) -> HTTPS.
func isLikelyHTTPSPort(raw string) bool {
	if i := strings.LastIndexByte(raw, ':'); i >= 0 {
		portStr := raw[i+1:]
		if port, err := strconv.Atoi(portStr); err == nil {
			switch port {
			case 443, 5001, 8443:
				return true
			}
		}
	}
	return false
}

// —— JSON 宽松取值 helper（厂商 API 字段类型不一：float64/json.Number/string 混用）——

func numFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}

// numUint 从多个候选字段取首个非零值（存储池容量字段名因 DSM 版本而异）。
func numUint(vs ...any) uint64 {
	for _, v := range vs {
		if v == nil {
			continue
		}
		f := numFloat(v)
		if f > 0 {
			return uint64(f)
		}
	}
	return 0
}

// firstStr 取首个非空字符串候选。
func firstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}
