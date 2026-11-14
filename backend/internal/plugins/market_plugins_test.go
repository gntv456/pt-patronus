// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
)

// ptpRoot walks up from the test cwd to find the dir containing backend/go.mod.
func ptpRoot(t *testing.T) string {
	t.Helper()
	dir, _ := os.Getwd()
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "backend", "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Skip("ptp root (backend/go.mod) not found")
	return ""
}

// TestRealMarketFetchAndInstall verifies the plugins that live under market-plugins/
// and are aggregated into examples/plugin-market/ are accepted by the real host
// pipeline: FetchMarket parses + computes compatibility/trust, and InstallFromMarket
// verifies the signature, unzips, validates the manifest and registers the plugin.
//
// 全部 feature 插件都在开放市场（market-plugins/），宿主不再内置注册任何 feature 插件；
// 这里断言 NewManager 干净启动（无内置残留），且市场条目可通过真实安装管线。
func TestRealMarketFetchAndInstall(t *testing.T) {
	root := ptpRoot(t)
	marketPath := filepath.Join(root, "examples", "plugin-market", "plugin-market.json")
	if _, err := os.Stat(marketPath); err != nil {
		t.Skipf("market not built: %v", err)
	}

	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	m.SetPluginRoot(filepath.Join(t.TempDir(), "plugins"))

	// 干净启动：NewManager 不内置注册任何 feature 插件（全部走开放市场安装）。
	for _, id := range []string{"ptp-drama-scrape", "ptp-playlet-category", "xiaomi-router", "json-toolbox", "event-audit"} {
		if pl, err := m.plugin(id); err == nil && pl != nil {
			t.Errorf("%s should not be built-in (lives in open market)", id)
		}
	}

	market, err := m.FetchMarket(context.Background(), marketPath)
	if err != nil {
		t.Fatalf("FetchMarket: %v", err)
	}
	if market.PublicKey == "" {
		t.Fatalf("expected signed market (public_key present)")
	}
	want := map[string]bool{
		"ptp-ffmpeg-thumb":   false,
		"ptp-library-scrape": false, // uses media:search (P3)
		"ptp-cd2-assistant":  false, // declares requirements (P2) + a view (P4 dashboard)
		"ptp-ai-subtitle":    false,
		"ptp-miwifi":         false,
	}
	for _, p := range market.Plugins {
		if _, ok := want[p.ID]; !ok {
			continue
		}
		want[p.ID] = true
		if !p.Compatible {
			t.Errorf("%s not compatible: %s", p.ID, p.CompatibilityReason)
		}
		if p.TrustLevel != "signed" && p.TrustLevel != "verified-signed" {
			t.Errorf("%s trust=%q, want signed", p.ID, p.TrustLevel)
		}
		if !p.Installable || len(p.InstallPlan) == 0 {
			t.Errorf("%s not installable: %s", p.ID, p.InstallReason)
		}
		// resource_access / requirements 透传到市场条目（P1/P2）
		if p.ID == "ptp-cd2-assistant" && len(p.Requirements) == 0 {
			t.Errorf("ptp-cd2-assistant missing requirements in market entry")
		}
		if p.ID == "ptp-ffmpeg-thumb" && (p.ResourceAccess == nil || len(p.ResourceAccess.ExecBinaries) == 0) {
			t.Errorf("ptp-ffmpeg-thumb missing resource_access.exec_binaries")
		}
	}
	for id, found := range want {
		if !found {
			t.Errorf("plugin %s missing from market", id)
		}
	}

	// Install ptp-library-scrape: its media:search permission (P3) must be accepted,
	// and the host-side manifest validation must accept the new resource_access field (P1).
	// ptp-cd2-assistant 触发 pip install python-clouddrive2（best-effort）；低于 Python 3.11 时失败不阻断安装，
	// 只要注册成功即可。
	for _, pid := range []string{"ptp-library-scrape", "ptp-cd2-assistant"} {
		mp := filepath.Join(root, "market-plugins", pid, "plugin.json")
		rb, _ := os.ReadFile(mp)
		var pm Manifest
		if err := json.Unmarshal(rb, &pm); err != nil {
			t.Fatalf("unmarshal %s: %v", pid, err)
		}
		if _, err := m.InstallFromMarketWithOptions(context.Background(), marketPath, pid,
			MarketInstallOptions{AcceptedPermissions: pm.Permissions}); err != nil {
			t.Fatalf("install %s: %v", pid, err)
		}
		if _, err := m.Get(pid); err != nil {
			t.Fatalf("%s not registered: %v", pid, err)
		}
	}
}

type fakeMediaSearcher struct{ hits []MediaHit }

func (f fakeMediaSearcher) Search(context.Context, string, int) ([]MediaHit, error) {
	return f.hits, nil
}

// TestManagerMediaSearch 覆盖 P3：未注入 searcher 返回空；注入后透传结果（/media/search 端点薄封装于此）。
func TestManagerMediaSearch(t *testing.T) {
	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)

	got, err := m.MediaSearch(context.Background(), "anything", 0)
	if err != nil || len(got) != 0 {
		t.Fatalf("nil searcher: got %v err=%v", got, err)
	}

	m.SetMediaSearcher(fakeMediaSearcher{hits: []MediaHit{{Source: "tmdb", SourceID: "42", Title: "X"}}})
	got, err = m.MediaSearch(context.Background(), "anything", 0)
	if err != nil || len(got) != 1 || got[0].Title != "X" {
		t.Fatalf("with searcher: got %v err=%v", got, err)
	}
}
