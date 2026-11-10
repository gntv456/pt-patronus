// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// collectTrueNAS 用 API Key（creds.Password 作 Token）取系统信息与存储池。
func collectTrueNAS(ctx context.Context, hc *httpclient.Client, t *models.MonitorTarget, creds Credentials) (Metrics, error) {
	base := normalizeBase(t.Host)
	if base == "" {
		return Metrics{}, fmt.Errorf("nas host is required")
	}
	hdr := map[string]string{"Authorization": "Token " + creds.Password}
	infoRaw, err := tnGet(ctx, hc, base+"/api/v2.0/system/info", hdr)
	if err != nil {
		return Metrics{}, fmt.Errorf("truenas info: %w", err)
	}
	m := Metrics{Online: true}
	var info map[string]any
	_ = json.Unmarshal(infoRaw, &info)
	version := firstStr(info, "version")
	if version != "" {
		m.OS = "TrueNAS " + version
	} else {
		m.OS = "TrueNAS"
	}
	m.Hostname = firstStr(info, "hostname")
	if v := numUint(info["uptime_seconds"]); v > 0 {
		m.Uptime = v
	}
	// 存储池
	if poolRaw, err := tnGet(ctx, hc, base+"/api/v2.0/pool", hdr); err == nil && len(poolRaw) > 0 {
		var pools []map[string]any
		if json.Unmarshal(poolRaw, &pools) == nil {
			for _, p := range pools {
				d := DiskMetric{Type: "pool"}
				d.Device = firstStr(p, "name")
				d.Total = numUint(p["size"])
				d.Used = numUint(p["allocated"])
				if d.Total > 0 {
					d.Usage = float64(d.Used) / float64(d.Total) * 100
				}
				m.Disks = append(m.Disks, d)
			}
		}
	}
	return m, nil
}

// tnGet 带 Token 鉴权 GET（ssrf=false：内网）。校验 HTTP 状态码，避免把 HTML 错误页当 JSON 解析。
func tnGet(ctx context.Context, hc *httpclient.Client, u string, hdr map[string]string) (json.RawMessage, error) {
	resp, err := hc.GetWithHeaders(ctx, u, "", hdr, false)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("truenas API %s returned HTTP %d (check token/permissions)", u, resp.StatusCode)
	}
	return json.RawMessage(resp.Body), nil
}

func powerTrueNAS(ctx context.Context, hc *httpclient.Client, t *models.MonitorTarget, creds Credentials, action string) error {
	base := normalizeBase(t.Host)
	if base == "" {
		return fmt.Errorf("nas host is required")
	}
	endpoint := ""
	switch action {
	case PowerActionReboot:
		endpoint = "reboot"
	case PowerActionShutdown:
		endpoint = "shutdown"
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedPowerAction, action)
	}
	hdr := map[string]string{"Authorization": "Token " + creds.Password}
	resp, err := hc.Do(ctx, http.MethodPost, base+"/api/v2.0/system/"+endpoint, nil, hdr, "", false)
	if err != nil {
		return fmt.Errorf("truenas %s: %w", endpoint, err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("truenas %s failed (status %d)", endpoint, resp.StatusCode)
	}
	return nil
}
