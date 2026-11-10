// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
)

// 市场安装失败必须能按语义分档（#162）：接口层靠这些哨兵挑状态码，
// 而从前它们全被压成一句英文 + 400「请求参数有误」——市场源挂了、包被改过、
// 宿主不兼容都成了用户填错表。本用例逐档钉住分类，顺带钉「文案是中文的」。
func writeMarketIndex(t *testing.T, dir string, item MarketPlugin) string {
	t.Helper()
	idx := MarketIndex{
		Version: 1,
		Plugins: []MarketPlugin{item},
	}
	raw, err := json.Marshal(idx)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "plugin-market.json")
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func baseMarketItem() MarketPlugin {
	return MarketPlugin{
		ID:               "classify-demo",
		Name:             "Classify Demo",
		Version:          "1.0.0",
		APIVersion:       CurrentPluginAPIVersion,
		AppVersion:       ">=0.1.0",
		HostCapabilities: []string{HostCapabilityRuntimeExternalHTTPBaseURL},
		Runtime:          RuntimeExternalHTTP,
		Archive:          "packages/classify-demo.zip",
		Permissions:      []string{"event:read"},
	}
}

func newClassifyManager(t *testing.T, root string) *Manager {
	t.Helper()
	db := newPluginTestDB(t)
	m := NewManager(db, event.New(), nil, nil, nil)
	m.SetPluginRoot(filepath.Join(root, "plugins"))
	return m
}

func TestInstallFailureClassification(t *testing.T) {
	manifest := Manifest{
		ID:          "classify-demo",
		Name:        "Classify Demo",
		Version:     "1.0.0",
		APIVersion:  CurrentPluginAPIVersion,
		AppVersion:  ">=0.1.0",
		Runtime:     RuntimeExternalHTTP,
		Entry:       &RuntimeEntry{BaseURL: "http://127.0.0.1:19091", Health: "/health"},
		Permissions: []string{"event:read"},
	}
	root := t.TempDir()
	pkgDir := filepath.Join(root, "packages")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	zipBytes := makePluginZip(t, manifest, map[string]string{"plugin.py": "print('x')\n"})
	sum := sha256.Sum256(zipBytes)

	cases := []struct {
		name     string
		item     func() MarketPlugin
		options  MarketInstallOptions
		mutate   func(*MarketPlugin)
		wantKind error
	}{
		{
			name: "市场源取不到",
			item: func() MarketPlugin {
				it := baseMarketItem()
				it.Archive = "packages/does-not-exist.zip"
				return it
			},
			options:  MarketInstallOptions{AcceptedPermissions: []string{"event:read"}},
			wantKind: ErrMarketUnreachable,
		},
		{
			name:    "包校验不通过",
			item:    baseMarketItem,
			options: MarketInstallOptions{AcceptedPermissions: []string{"event:read"}},
			mutate: func(p *MarketPlugin) {
				p.SHA256 = hex.EncodeToString([]byte("0000000000000000000000000000000000000000000000000000000000000000"))
			},
			wantKind: ErrUntrustedPackage,
		},
		{
			name:     "宿主不兼容",
			item:     baseMarketItem,
			options:  MarketInstallOptions{AcceptedPermissions: []string{"event:read"}},
			mutate:   func(p *MarketPlugin) { p.APIVersion = "v9999-不存在" },
			wantKind: ErrPluginNotInstallable,
		},
		{
			name:     "权限未确认",
			item:     baseMarketItem,
			options:  MarketInstallOptions{}, // 没带确认
			wantKind: ErrPermissionsUnconfirmed,
		},
		{
			// 包是好的、sha 也对得上，但没人给它签名：默认配置下同样归「不可信包」，
			// 不能因为「只是没签名」就落进参数有误那一档。
			name:     "未签名包",
			item:     baseMarketItem,
			options:  MarketInstallOptions{AcceptedPermissions: []string{"event:read"}},
			mutate:   func(p *MarketPlugin) { p.SHA256 = hex.EncodeToString(sum[:]) },
			wantKind: ErrUntrustedPackage,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			it := tc.item()
			if tc.mutate != nil {
				tc.mutate(&it)
			}
			if tc.wantKind == ErrUntrustedPackage {
				if err := os.WriteFile(filepath.Join(pkgDir, "classify-demo.zip"), zipBytes, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			m := newClassifyManager(t, root)
			indexPath := writeMarketIndex(t, root, it)
			_, err := m.InstallFromMarketWithOptions(context.Background(), indexPath, it.ID, tc.options)
			if err == nil {
				t.Fatalf("应当失败（%s），却成功了", tc.name)
			}
			if !errors.Is(err, tc.wantKind) {
				t.Fatalf("分档 = %v，期望 %s（err=%v）", err, tc.wantKind, err)
			}
			// 用户看到的必须是中文头一句，不能只剩 Go 的英文原文。
			if !strings.Contains(err.Error(), tc.wantKind.Error()) {
				t.Errorf("错误文案应带上哨兵的中文抬头，got %q", err.Error())
			}
		})
	}

	t.Run("市场里没有这个插件", func(t *testing.T) {
		m := newClassifyManager(t, root)
		indexPath := writeMarketIndex(t, root, baseMarketItem())
		_, err := m.InstallFromMarketWithOptions(context.Background(), indexPath, "no-such-plugin", MarketInstallOptions{AcceptedPermissions: []string{}})
		if !errors.Is(err, ErrPluginNotInMarket) {
			t.Fatalf("应归到「市场里没有」，got %v", err)
		}
	})
}

// recHandler 收 slog 记录用（判「有没有在默认可见级别留痕」）。
type recHandler struct {
	mu     sync.Mutex
	levels []string
}

func (h *recHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.levels = append(h.levels, r.Level.String()+"|"+r.Message)
	return nil
}
func (h *recHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recHandler) WithGroup(string) slog.Handler      { return h }

func (h *recHandler) hasWarn(msg string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, l := range h.levels {
		if strings.HasPrefix(l, "WARN|") && strings.Contains(l, msg) {
			return true
		}
	}
	return false
}

// TestInstallKeepsTamperEvidence 篡改证据必须落在默认可见的级别。
// 接口层对 4xx 只把原因写到 Debug（errcode.Respond 的分级），而「市场声明的 sha256
// 与实际不符」是安全事件——只在 debug 模式下存在等于没留痕，事后查不到是哪个源给的坏包。
func TestInstallKeepsTamperEvidence(t *testing.T) {
	root := t.TempDir()
	pkgDir := filepath.Join(root, "packages")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		ID: "evidence-demo", Name: "Evidence", Version: "1.0.0",
		APIVersion: CurrentPluginAPIVersion, AppVersion: ">=0.1.0",
		Runtime:     RuntimeExternalHTTP,
		Entry:       &RuntimeEntry{BaseURL: "http://127.0.0.1:19092", Health: "/health"},
		Permissions: []string{"event:read"},
	}
	zipBytes := makePluginZip(t, manifest, map[string]string{"plugin.py": "print('e')\n"})
	if err := os.WriteFile(filepath.Join(pkgDir, "evidence-demo.zip"), zipBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	item := baseMarketItem()
	item.ID = manifest.ID
	item.Name = manifest.Name
	item.Archive = "packages/evidence-demo.zip"
	item.SHA256 = hex.EncodeToString(make([]byte, 32)) // 故意与市场声明错位
	indexPath := writeMarketIndex(t, root, item)

	rec := &recHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(rec))
	t.Cleanup(func() { slog.SetDefault(prev) })

	m := newClassifyManager(t, root)
	_, err := m.InstallFromMarketWithOptions(context.Background(), indexPath, item.ID,
		MarketInstallOptions{AcceptedPermissions: item.Permissions})
	if !errors.Is(err, ErrUntrustedPackage) {
		t.Fatalf("应归到不可信包，got %v", err)
	}
	if !rec.hasWarn("plugin package sha256 mismatch") {
		t.Errorf("坏包必须 Warn 留痕，实际记录：%v", rec.levels)
	}
}
