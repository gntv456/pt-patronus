// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
)

func TestManagerRegistersUsefulBuiltinPlugins(t *testing.T) {
	t.Skip("feature builtins moved to open plugin market")

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)

	plugins, err := m.List()
	if err != nil {
		t.Fatalf("list plugins: %v", err)
	}
	ids := map[string]bool{}
	for _, p := range plugins {
		ids[p.ID] = true
	}
	for _, id := range []string{
		"event-audit",
		"notice-forwarder",
		"http-probe",
		"json-toolbox",
		"text-toolbox",
		"webhook-transformer",
		"keyword-alert",
		"cron-helper",
		"cloudflare-subscribe",
		"group-chat-zone",
		"twofa-helper",
		"xiaomi-router",
		"auto-speed",
	} {
		if !ids[id] {
			t.Fatalf("expected built-in plugin %s, got %#v", id, ids)
		}
	}
}

func TestJSONToolboxActions(t *testing.T) {
	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	mustRegister(t, m, NewJSONToolboxPlugin())
	if err := m.SetEnabled("json-toolbox", true); err != nil {
		t.Fatalf("enable json toolbox: %v", err)
	}

	formatted, err := m.ExecuteAction(context.Background(), "json-toolbox", "format", map[string]any{
		"json":   `{"site":{"name":"PTPatronus"}}`,
		"indent": "  ",
	})
	if err != nil {
		t.Fatalf("format json: %v", err)
	}
	if formatted["bytes"].(int) == 0 {
		t.Fatalf("expected formatted bytes, got %#v", formatted)
	}

	extracted, err := m.ExecuteAction(context.Background(), "json-toolbox", "extract", map[string]any{
		"json": `{"items":[{"title":"demo"}]}`,
		"path": "items[0].title",
	})
	if err != nil {
		t.Fatalf("extract json: %v", err)
	}
	if extracted["found"] != true || extracted["value"] != "demo" {
		t.Fatalf("unexpected extract output: %#v", extracted)
	}
}

func TestTextToolboxBase64(t *testing.T) {
	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	mustRegister(t, m, NewTextToolboxPlugin())
	if err := m.SetEnabled("text-toolbox", true); err != nil {
		t.Fatalf("enable text toolbox: %v", err)
	}

	out, err := m.ExecuteAction(context.Background(), "text-toolbox", "base64", map[string]any{
		"mode": "encode",
		"text": "PTPatronus",
	})
	if err != nil {
		t.Fatalf("encode base64: %v", err)
	}
	if out["text"] != base64.StdEncoding.EncodeToString([]byte("PTPatronus")) {
		t.Fatalf("unexpected base64 output: %#v", out)
	}
}

func TestHTTPProbeAction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	mustRegister(t, m, NewHTTPProbePlugin())
	if _, err := m.SaveConfig("http-probe", map[string]any{"allow_private_network": true}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := m.SetEnabled("http-probe", true); err != nil {
		t.Fatalf("enable http probe: %v", err)
	}

	out, err := m.ExecuteAction(context.Background(), "http-probe", "probe", map[string]any{
		"url":             server.URL,
		"method":          http.MethodGet,
		"expected_status": 200,
	})
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if out["healthy"] != true || out["status_code"] != 200 {
		t.Fatalf("unexpected probe output: %#v", out)
	}
}

func TestWebhookTransformerPreview(t *testing.T) {
	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	mustRegister(t, m, NewWebhookTransformerPlugin())
	if err := m.SetEnabled("webhook-transformer", true); err != nil {
		t.Fatalf("enable transformer: %v", err)
	}

	out, err := m.ExecuteAction(context.Background(), "webhook-transformer", "preview_transform", map[string]any{
		"payload": `{"title":"部署完成","message":"ok","level":"success"}`,
	})
	if err != nil {
		t.Fatalf("preview transform: %v", err)
	}
	got, ok := out["output"].(map[string]any)
	if !ok || got["title"] != "部署完成" || got["message"] != "ok" {
		t.Fatalf("unexpected transform output: %#v", out)
	}
}

func TestKeywordAlertMatchAction(t *testing.T) {
	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	mustRegister(t, m, NewKeywordAlertPlugin())
	if err := m.SetEnabled("keyword-alert", true); err != nil {
		t.Fatalf("enable keyword alert: %v", err)
	}

	out, err := m.ExecuteAction(context.Background(), "keyword-alert", "test_match", map[string]any{
		"payload": `{"title":"部署失败","message":"restart failed"}`,
	})
	if err != nil {
		t.Fatalf("test keyword match: %v", err)
	}
	if out["matched"] != true {
		t.Fatalf("expected keyword match, got %#v", out)
	}
}

func TestCronHelperNextRuns(t *testing.T) {
	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	mustRegister(t, m, NewCronHelperPlugin())
	if err := m.SetEnabled("cron-helper", true); err != nil {
		t.Fatalf("enable cron helper: %v", err)
	}

	out, err := m.ExecuteAction(context.Background(), "cron-helper", "next_runs", map[string]any{
		"expression": "0 3 * * *",
		"count":      3,
		"timezone":   "Asia/Shanghai",
	})
	if err != nil {
		t.Fatalf("next runs: %v", err)
	}
	if runs, ok := out["next_runs"].([]string); !ok || len(runs) != 3 {
		t.Fatalf("unexpected cron output: %#v", out)
	}
}

// TestPluginSSRFDialContext_BlocksPrivateIP 验证拨号时 SSRF 校验：直连 loopback 地址被拒，
// 而非真正建连。这把校验绑定到实际连接，消除预检与连接间的 DNS 重绑定窗口（TOCTOU）。
func TestPluginSSRFDialContext_BlocksPrivateIP(t *testing.T) {
	t.Parallel()
	// 起一个本地监听（127.0.0.1 = loopback，应被 SSRF 拨号拒绝，绝不产生真实连接）。
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	connected := make(chan struct{})
	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			close(connected)
			_ = conn.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := pluginSSRFDialContext(ctx, "tcp", ln.Addr().String()); err == nil {
		t.Error("pluginSSRFDialContext should block loopback address")
	}
	select {
	case <-connected:
		t.Error("dial should not have connected to loopback address")
	default:
	}
}
