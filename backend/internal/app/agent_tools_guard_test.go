// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

import (
	"context"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/mcp"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

// 工具参数不经 schema 校验（aiagent.ToolRegistry.Exec 只查白名单与超时），
// argInt/argFloat/argString/argBool 取不到就补 0/""/false——而这几个工具里
// 「补出来的默认值」在下载器端是有真实后果的动作。这里钉住：缺参数必须先报错、
// 且报错发生在解引用 chain 之前（dl 传 nil，走到 chain 就是 panic）。

var guardedDownloadTools = []struct {
	tool string
	args map[string]any // 已给出「其它必填项」，只缺那个破坏性默认值的参数
	miss string         // 必须点名的缺失参数
}{
	{"torrents_set_category", map[string]any{"downloader_id": 1, "hashes": []any{"H"}}, "category"},
	{"torrents_set_limits", map[string]any{"downloader_id": 1, "hashes": []any{"H"}, "upload_limit": 100}, "download_limit"},
	{"torrents_set_share_limits", map[string]any{"downloader_id": 1, "hashes": []any{"H"}, "ratio_limit": 0.5}, "seeding_time_limit"},
	{"torrents_set_force_start", map[string]any{"downloader_id": 1, "hashes": []any{"H"}}, "enable"},
}

func agentTool(t *testing.T, name string) mcp.Tool {
	t.Helper()
	for _, tl := range buildDownloaderAdvTools(nil) {
		if tl.Name == name {
			return tl
		}
	}
	t.Fatalf("agent 工具表里没有 %s", name)
	return mcp.Tool{}
}

func pluginTool(t *testing.T, name string) plugins.RegisteredHostTool {
	t.Helper()
	reg := plugins.NewHostToolRegistry()
	registerDownloaderTools(reg, nil)
	rt, ok := reg.Lookup(name)
	if !ok {
		t.Fatalf("plugin 工具表里没有 %s", name)
	}
	return rt
}

// TestAgentToolsRejectMissingDestructiveDefaults AI 工具面：缺参数必须早失败。
func TestAgentToolsRejectMissingDestructiveDefaults(t *testing.T) {
	for _, tc := range guardedDownloadTools {
		tl := agentTool(t, tc.tool)
		out, err := tl.Handler(context.Background(), tc.args)
		if err == nil {
			t.Errorf("%s: 缺 %s 必须报错，got %s", tc.tool, tc.miss, out)
			continue
		}
		if !strings.Contains(err.Error(), tc.miss) {
			t.Errorf("%s: 错误应点名缺哪个参数，got %v", tc.tool, err)
		}
	}
}

// TestPluginToolsRejectMissingDestructiveDefaults 插件工具面是同一批操作的另一份拷贝，
// 必须同样收口（否则绕过 AI 面的插件照样能一句话清掉分类/摘掉限速）。
func TestPluginToolsRejectMissingDestructiveDefaults(t *testing.T) {
	for _, tc := range guardedDownloadTools {
		rt := pluginTool(t, tc.tool)
		out, err := rt.Handler(context.Background(), tc.args)
		if err == nil {
			t.Errorf("%s: 缺 %s 必须报错，got %+v", tc.tool, tc.miss, out)
			continue
		}
		if !strings.Contains(err.Error(), tc.miss) {
			t.Errorf("%s: 错误应点名缺哪个参数，got %v", tc.tool, err)
		}
	}
}

// TestGuardedParamsAreDeclaredAndRequired schema 半边：参数没写进 properties，
// 模型根本不知道要给；没进 required，省略就没人拦。两个面都要齐。
func TestGuardedParamsAreDeclaredAndRequired(t *testing.T) {
	want := map[string][]string{
		"torrents_set_category":     {"category"},
		"torrents_set_limits":       {"upload_limit", "download_limit"},
		"torrents_set_share_limits": {"ratio_limit", "seeding_time_limit"},
		"torrents_set_force_start":  {"enable"},
		"torrents_set_tags":         {"tags"},
	}
	for name, keys := range want {
		check := func(surface string, props map[string]any, required []string) {
			for _, k := range keys {
				if _, ok := props[k]; !ok {
					t.Errorf("%s(%s): properties 缺 %s，模型无从得知", name, surface, k)
				}
				found := false
				for _, r := range required {
					if r == k {
						found = true
					}
				}
				if !found {
					t.Errorf("%s(%s): required 缺 %s，省略参数不会有人拦", name, surface, k)
				}
			}
		}
		tl := agentTool(t, name)
		check("agent", propMap(tl.InputSchema), reqList(tl.InputSchema))
		rt := pluginTool(t, name)
		check("plugin", propMap(rt.InputSchema), reqList(rt.InputSchema))
	}
}

func propMap(schema map[string]any) map[string]any {
	p, _ := schema["properties"].(map[string]any)
	return p
}

func reqList(schema map[string]any) []string {
	switch r := schema["required"].(type) {
	case []string:
		return r
	case []any:
		var out []string
		for _, v := range r {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// TestMissingArgsTreatsExplicitValuesAsPresent 正控边界：显式给出的 0/""/false
// 都是合法意图（清除分类、取消限速、取消强制开始），不能当成没给——
// 否则这个闸门会把用户真正想做的操作也拦死。
func TestMissingArgsTreatsExplicitValuesAsPresent(t *testing.T) {
	if got := missingArgs(map[string]any{"category": ""}, "category"); len(got) != 0 {
		t.Errorf("显式空串应算给出，got %v", got)
	}
	if got := missingArgs(map[string]any{"upload_limit": 0, "download_limit": 0}, "upload_limit", "download_limit"); len(got) != 0 {
		t.Errorf("显式 0 应算给出（0=取消限速是真实意图），got %v", got)
	}
	if got := missingArgs(map[string]any{"enable": false}, "enable"); len(got) != 0 {
		t.Errorf("显式 false 应算给出，got %v", got)
	}
	if got := missingArgs(map[string]any{"enable": nil}, "enable"); len(got) != 1 {
		t.Errorf("显式 null 应算没给，got %v", got)
	}
	if err := requireArgs(map[string]any{"a": 1}, "a"); err != nil {
		t.Errorf("齐了就不该报错，got %v", err)
	}
}
