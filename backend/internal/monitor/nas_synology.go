// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// synoResp 群晖 DSM API 统一信封。
type synoResp struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   struct {
		Code int `json:"code"`
	} `json:"error"`
}

// collectSynology 登录群晖 → 取系统利用率/存储池 → 登出。
func collectSynology(ctx context.Context, hc *httpclient.Client, t *models.MonitorTarget, creds Credentials) (Metrics, error) {
	base := normalizeBase(t.Host)
	if base == "" {
		return Metrics{}, fmt.Errorf("nas host is required")
	}
	sid, err := synoCall(ctx, hc, base, "SYNO.API.Auth", "Login", 3, url.Values{
		"account": {creds.Username}, "passwd": {creds.Password}, "session": {"Core"}, "format": {"sid"},
	})
	if err != nil {
		return Metrics{}, fmt.Errorf("synology login: %w", err)
	}
	var login struct {
		Sid string `json:"sid"`
	}
	_ = json.Unmarshal(sid, &login)
	defer func() {
		_, _ = synoCall(ctx, hc, base, "SYNO.API.Auth", "Logout", 3, url.Values{"_sid": {login.Sid}, "session": {"Core"}})
	}()
	utilRaw, err := synoCall(ctx, hc, base, "SYNO.Core.System.Utilization", "get", 1, url.Values{"_sid": {login.Sid}})
	if err != nil {
		return Metrics{}, fmt.Errorf("synology utilization: %w", err)
	}
	storageRaw, _ := synoCall(ctx, hc, base, "SYNO.Storage.CGI.Storage", "load", 2, url.Values{"_sid": {login.Sid}})
	return parseSynology(utilRaw, storageRaw), nil
}

func powerSynology(ctx context.Context, hc *httpclient.Client, t *models.MonitorTarget, creds Credentials, action string) error {
	base := normalizeBase(t.Host)
	if base == "" {
		return fmt.Errorf("nas host is required")
	}
	method := ""
	switch action {
	case PowerActionReboot:
		method = "reboot"
	case PowerActionShutdown:
		method = "shutdown"
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedPowerAction, action)
	}
	sid, err := synoCall(ctx, hc, base, "SYNO.API.Auth", "Login", 3, url.Values{
		"account": {creds.Username}, "passwd": {creds.Password}, "session": {"Core"}, "format": {"sid"},
	})
	if err != nil {
		return fmt.Errorf("synology login: %w", err)
	}
	var login struct {
		Sid string `json:"sid"`
	}
	_ = json.Unmarshal(sid, &login)
	defer func() {
		_, _ = synoCall(ctx, hc, base, "SYNO.API.Auth", "Logout", 3, url.Values{"_sid": {login.Sid}, "session": {"Core"}})
	}()
	if _, err := synoCall(ctx, hc, base, "SYNO.Core.System", method, 1, url.Values{"_sid": {login.Sid}}); err != nil {
		if _, err2 := synoCall(ctx, hc, base, "SYNO.Core.System", method, 2, url.Values{"_sid": {login.Sid}}); err2 != nil {
			// 两次尝试都留下原因：拦腰失败可能发生在第二次（如密码策略要求 OTP），只报 v1 的错会丢一半线索。
			return fmt.Errorf("synology %s: %w（v2 重试亦失败：%v）", method, err, err2)
		}
	}
	return nil
}

// synoCall 调用一个群晖 API 并返回 data 载荷（success=false 视为失败）。
func synoCall(ctx context.Context, hc *httpclient.Client, base, api, method string, version int, q url.Values) (json.RawMessage, error) {
	q.Set("api", api)
	q.Set("method", method)
	q.Set("version", strconv.Itoa(version))
	u := base + "/webapi/" + synoPath(api) + "?" + q.Encode()
	resp, err := hc.Get(ctx, u, "", false) // ssrf=false：用户主动配置的内网 NAS
	if err != nil {
		return nil, err
	}
	var r synoResp
	if err := json.Unmarshal(resp.Body, &r); err != nil {
		return nil, fmt.Errorf("decode synology response: %w", err)
	}
	if !r.Success {
		return nil, fmt.Errorf("synology %s failed (code %d)", api, r.Error.Code)
	}
	return r.Data, nil
}

// synoPath auth 用 auth.cgi，其余统一 entry.cgi。
func synoPath(api string) string {
	if api == "SYNO.API.Auth" {
		return "auth.cgi"
	}
	return "entry.cgi"
}

// parseSynology 解析系统利用率 + 存储池（字段名随 DSM 版本变化，宽匹配）。
func parseSynology(utilRaw, storageRaw json.RawMessage) Metrics {
	m := Metrics{Online: true, OS: "Synology DSM"}
	if len(utilRaw) > 0 {
		var u map[string]any
		_ = json.Unmarshal(utilRaw, &u)
		m.CPUUsage = numFloat(u["cpu_utilization"])
		if mem, ok := u["memory"].(map[string]any); ok {
			m.MemUsage = numFloat(mem["real_usage"])
			if real, ok := mem["real"].(map[string]any); ok {
				m.MemTotal = numUint(real["total"])
				m.MemUsed = numUint(real["used"])
			}
		}
	}
	if len(storageRaw) > 0 {
		var s map[string]any
		_ = json.Unmarshal(storageRaw, &s)
		for _, key := range []string{"volumes", "storage_pools"} {
			arr, _ := s[key].([]any)
			for _, it := range arr {
				pool, _ := it.(map[string]any)
				if pool == nil {
					continue
				}
				d := DiskMetric{Type: "pool"}
				d.Device = firstStr(pool, "name", "id", "disk_group_name")
				d.Total = numUint(pool["size_total"], pool["total_size"], pool["size"])
				d.Used = numUint(pool["size_used"], pool["used_size"], pool["used"])
				if d.Total > 0 {
					d.Usage = float64(d.Used) / float64(d.Total) * 100
				}
				m.Disks = append(m.Disks, d)
			}
		}
	}
	return m
}
