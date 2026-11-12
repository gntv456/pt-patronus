// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
)

func TestCloudflareSubscribeUpdateHosts(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("1.1.1.1 example.com\nbad line\n2606:4700:4700::1111 ipv6.example.com\n"))
	}))
	defer source.Close()

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	mustRegister(t, m, NewCloudflareSubscribePlugin())
	if _, err := m.SaveConfig("cloudflare-subscribe", map[string]any{"allow_private_network": true}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := m.SetEnabled("cloudflare-subscribe", true); err != nil {
		t.Fatalf("enable: %v", err)
	}

	out, err := m.ExecuteAction(context.Background(), "cloudflare-subscribe", "update_hosts", map[string]any{
		"subscriptions":  "cf|" + source.URL,
		"existing_hosts": "127.0.0.1 localhost\n# ===== old 订阅开始 ===== #\n1.1.1.1 old.test\n# ===== old 订阅结束 (1 条记录) ===== #",
	})
	if err != nil {
		t.Fatalf("update hosts: %v", err)
	}
	hosts := out["hosts"].(string)
	if !strings.Contains(hosts, "127.0.0.1 localhost") || !strings.Contains(hosts, "example.com") || strings.Contains(hosts, "old.test") {
		t.Fatalf("unexpected hosts output:\n%s", hosts)
	}
	if out["records"] != 2 {
		t.Fatalf("records = %#v", out["records"])
	}
}

func TestTwoFAHelperGenerateOne(t *testing.T) {
	code, remaining, err := plTOTPCode("JBSWY3DPEHPK3PXP", mustTime(t, 59), 6, 30)
	if err != nil {
		t.Fatalf("totp: %v", err)
	}
	if code == "" || remaining <= 0 {
		t.Fatalf("unexpected totp code=%q remaining=%d", code, remaining)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	mustRegister(t, m, NewTwoFAHelperPlugin())
	if err := m.SetEnabled("twofa-helper", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	out, err := m.ExecuteAction(context.Background(), "twofa-helper", "generate_one", map[string]any{
		"name":   "demo",
		"secret": "JBSWY3DPEHPK3PXP",
	})
	if err != nil {
		t.Fatalf("generate one: %v", err)
	}
	if out["code"] == "" {
		t.Fatalf("empty code: %#v", out)
	}
}

func TestGroupChatZoneSendMessage(t *testing.T) {
	var gotMessage string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shoutbox.php":
			if r.Method == http.MethodGet && r.URL.Query().Get("shbox_text") != "" {
				gotMessage = r.URL.Query().Get("shbox_text")
			}
			_, _ = w.Write([]byte(`<table><tr><td class="shoutrow">系统 @demo 奖励 10G 上传</td></tr></table>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	mustRegister(t, m, NewGroupChatZonePlugin())
	if _, err := m.SaveConfig("group-chat-zone", map[string]any{"allow_private_network": true}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := m.SetEnabled("group-chat-zone", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	out, err := m.ExecuteAction(context.Background(), "group-chat-zone", "send_message", map[string]any{
		"base_url":       server.URL,
		"cookie":         "uid=1",
		"message":        "求上传",
		"mode":           "nexus_get",
		"fetch_feedback": true,
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if gotMessage != "求上传" || out["status_code"] != 200 {
		t.Fatalf("unexpected send result message=%q out=%#v", gotMessage, out)
	}
}

func TestXiaomiRouterStatusAndPortList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/luci/api/xqsystem/init_info":
			_ = json.NewEncoder(w).Encode(map[string]any{"newEncryptMode": 0, "key": "abc", "routerId": "001122"})
		case "/cgi-bin/luci/api/xqsystem/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "token": "token123"})
		case "/cgi-bin/luci/;stok=token123/api/misystem/status":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"displayName": "Xiaomi",
				"wan":         map[string]any{"upload": 1024, "download": 2048, "upspeed": 512, "downspeed": 1024},
				"cpu":         map[string]any{"load": 10},
				"mem":         map[string]any{"usage": 30},
				"count":       map[string]any{"online": 3},
				"upTime":      3600,
			})
		case "/cgi-bin/luci/;stok=token123/api/xqnetwork/portforward":
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "list": []map[string]any{{"name": "ssh", "proto": 1, "srcport": 22, "destip": "192.168.31.10", "destport": 22}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	mustRegister(t, m, NewXiaomiRouterPlugin())
	if _, err := m.SaveConfig("xiaomi-router", map[string]any{
		"router_url":          server.URL,
		"password":            "pass",
		"allow_public_router": true,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := m.SetEnabled("xiaomi-router", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	out, err := m.ExecuteAction(context.Background(), "xiaomi-router", "status", map[string]any{})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	status := out["status"].(map[string]any)
	if status["online_count"] != 3 {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestAutoSpeedRunSpeedtest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(strings.Repeat("x", 128*1024)))
		case http.MethodPost:
			_, _ = io.Copy(io.Discard, r.Body)
			_, _ = w.Write([]byte("ok"))
		}
	}))
	defer server.Close()

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	mustRegister(t, m, NewAutoSpeedPlugin())
	if _, err := m.SaveConfig("auto-speed", map[string]any{
		"download_url":          server.URL + "/down",
		"upload_url":            server.URL + "/up",
		"allow_private_network": true,
		"download_bytes":        128 * 1024,
		"upload_bytes":          64 * 1024,
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := m.SetEnabled("auto-speed", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	out, err := m.ExecuteAction(context.Background(), "auto-speed", "run_speedtest", map[string]any{})
	if err != nil {
		t.Fatalf("speedtest: %v", err)
	}
	if pluginInt(out["download_bytes"], 0) == 0 || out["download_mbps"] == nil {
		t.Fatalf("unexpected speedtest output: %#v", out)
	}
}

func mustTime(t *testing.T, unix int64) time.Time {
	t.Helper()
	return time.Unix(unix, 0)
}
