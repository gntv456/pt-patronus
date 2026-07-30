package plugins

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// TestNormalizeMarketSource 覆盖 GitHub 仓库 URL → jsDelivr CDN 转换、简写、tree 分支/子目录、以及
// raw/file/其它 http(s) 源原样返回等分支。
func TestNormalizeMarketSource(t *testing.T) {
	cases := []struct{ in, want string }{
		// GitHub 仓库页 → raw 默认分支 main / 根目录 plugin-market.json
		// GitHub 仓库页 → jsDelivr CDN 默认分支 main / 根目录 plugin-market.json
		{"https://github.com/owner/repo", "https://cdn.jsdelivr.net/gh/owner/repo@main/plugin-market.json"},
		{"https://github.com/owner/repo/", "https://cdn.jsdelivr.net/gh/owner/repo@main/plugin-market.json"},
		// 官方种子 URL 解析（锁定 EnsureDefaultMarketSource 播种地址）
		{"https://github.com/gntv456/ptpatronus-plugin-market", "https://cdn.jsdelivr.net/gh/gntv456/ptpatronus-plugin-market@main/plugin-market.json"},
		// tree/{branch} 与 tree/{branch}/{path}
		{"https://github.com/owner/repo/tree/dev", "https://cdn.jsdelivr.net/gh/owner/repo@dev/plugin-market.json"},
		{"https://github.com/owner/repo/tree/dev/market", "https://cdn.jsdelivr.net/gh/owner/repo@dev/market/plugin-market.json"},
		// 无 scheme 与 owner/repo 简写
		{"github.com/owner/repo", "https://cdn.jsdelivr.net/gh/owner/repo@main/plugin-market.json"},
		{"owner/repo", "https://cdn.jsdelivr.net/gh/owner/repo@main/plugin-market.json"},
		{"https://www.github.com/owner/repo", "https://cdn.jsdelivr.net/gh/owner/repo@main/plugin-market.json"},
		// 已是 raw / 其它 http 源 / 本地路径 → 原样
		{"https://raw.githubusercontent.com/owner/repo/main/plugin-market.json", "https://raw.githubusercontent.com/owner/repo/main/plugin-market.json"},
		{"https://example.com/market.json", "https://example.com/market.json"},
		{"/local/path/plugin-market.json", "/local/path/plugin-market.json"},
		{"", ""},
	}
	for _, c := range cases {
		got := normalizeMarketSource(c.in)
		if got != c.want {
			t.Errorf("normalizeMarketSource(%q)\n  got  %q\n  want %q", c.in, got, c.want)
		}
	}
}

// TestNormalizeSources 覆盖 trim、去空、去重（保持首次出现顺序）。
func TestNormalizeSources(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"空输入", nil, []string{}},
		{"纯空白与空串", []string{"  ", "", "\t"}, []string{}},
		{"trim 两侧空格", []string{"  a  ", "b"}, []string{"a", "b"}},
		{"去重保序", []string{"a", "b", "a", "c", "b"}, []string{"a", "b", "c"}},
		{"混合空串与重复", []string{"", "x", "x", "  y ", "", "y"}, []string{"x", "y"}},
	}
	for _, c := range cases {
		got := normalizeSources(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("normalizeSources(%v) %s\n  got  %v\n  want %v", c.in, c.name, got, c.want)
		}
	}
}

// TestMergeMarketPlugins 覆盖多源合并：不同 ID 并存、同 ID 先列者优先、空 ID 跳过、Source 保留。
func TestMergeMarketPlugins(t *testing.T) {
	p := func(id, src string) MarketPlugin { return MarketPlugin{ID: id, Name: id, Source: src} }
	t.Run("不同 ID 并存", func(t *testing.T) {
		got := mergeMarketPlugins(nil, []MarketPlugin{p("a", "s1")}, []MarketPlugin{p("b", "s2")})
		want := []MarketPlugin{p("a", "s1"), p("b", "s2")}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v want %v", got, want)
		}
	})
	t.Run("同 ID 先列者优先", func(t *testing.T) {
		first := MarketPlugin{ID: "a", Name: "from-s1", Source: "s1"}
		dup := MarketPlugin{ID: "a", Name: "from-s2", Source: "s2"}
		got := mergeMarketPlugins(nil, []MarketPlugin{first}, []MarketPlugin{dup})
		want := []MarketPlugin{first}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v want %+v", got, want)
		}
	})
	t.Run("空 ID 跳过", func(t *testing.T) {
		got := mergeMarketPlugins(nil, []MarketPlugin{{ID: "", Name: "x"}, p("a", "s1")})
		want := []MarketPlugin{p("a", "s1")}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v want %v", got, want)
		}
	})
	t.Run("保留 out 中已有项并继续合并", func(t *testing.T) {
		got := mergeMarketPlugins([]MarketPlugin{p("seed", "s0")}, []MarketPlugin{p("a", "s1")}, []MarketPlugin{p("seed", "s2"), p("b", "s2")})
		want := []MarketPlugin{p("seed", "s0"), p("a", "s1"), p("b", "s2")}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v want %v", got, want)
		}
	})
}

// newMarketSourceTestManager 构造一个仅注入 cfgOp 的 Manager（Get/Set/EnsureDefaultMarketSource
// 只依赖 cfgOp + mu，无需其它字段），用于市场源播种/读写单测。
func newMarketSourceTestManager(t *testing.T) (*Manager, *oper.SystemConfigOper) {
	t.Helper()
	cfgOp := oper.NewSystemConfigOper(newPluginTestDB(t))
	m := &Manager{}
	m.SetCfgOp(cfgOp)
	return m, cfgOp
}

// TestEnsureDefaultMarketSource_SeedsOnFreshInstall：全新安装（无任何市场源配置）→ 播种官方源。
func TestEnsureDefaultMarketSource_SeedsOnFreshInstall(t *testing.T) {
	m, _ := newMarketSourceTestManager(t)
	m.EnsureDefaultMarketSource()
	got := m.GetMarketSources()
	want := []string{OfficialMarketSource}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fresh install should seed [OfficialMarketSource]\n  got  %v\n  want %v", got, want)
	}
}

// TestEnsureDefaultMarketSource_SkipsWhenUserCleared：用户显式清空所有源（持久化 "[]"）→
// 不重新播种官方源。这是「用户可删默认官方仓库」需求 + "[]" 不变式的回归守护。
func TestEnsureDefaultMarketSource_SkipsWhenUserCleared(t *testing.T) {
	m, _ := newMarketSourceTestManager(t)
	if err := m.SetMarketSources([]string{}); err != nil {
		t.Fatalf("clear sources: %v", err)
	}
	m.EnsureDefaultMarketSource()
	if got := m.GetMarketSources(); len(got) != 0 {
		t.Fatalf("user-cleared list must NOT be re-seeded, got %v", got)
	}
}

// TestEnsureDefaultMarketSource_SkipsWhenUserHasThirdParty：用户已配置第三方源 → 保留，不注入官方源。
func TestEnsureDefaultMarketSource_SkipsWhenUserHasThirdParty(t *testing.T) {
	m, _ := newMarketSourceTestManager(t)
	third := "https://example.com/market.json"
	if err := m.SetMarketSources([]string{third}); err != nil {
		t.Fatalf("set third-party source: %v", err)
	}
	m.EnsureDefaultMarketSource()
	got := m.GetMarketSources()
	want := []string{third}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("third-party source must be preserved without injecting official\n  got  %v\n  want %v", got, want)
	}
}

// TestEnsureDefaultMarketSource_SkipsWhenLegacySet：仅旧版单源 key 已设（老版本迁移态）→
// 尊重既有源（GetMarketSources 回退读到），新 key 不被播种写入、官方源不注入。
func TestEnsureDefaultMarketSource_SkipsWhenLegacySet(t *testing.T) {
	m, cfgOp := newMarketSourceTestManager(t)
	const legacy = "https://old.example/legacy.json"
	if err := cfgOp.Set(pluginMarketSourceKey, legacy); err != nil {
		t.Fatalf("set legacy source: %v", err)
	}
	m.EnsureDefaultMarketSource()
	// GetMarketSources 走旧 key 回退
	if got, want := m.GetMarketSources(), []string{legacy}; !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy source should remain via fallback\n  got  %v\n  want %v", got, want)
	}
	// 新 key 应仍未被播种写入
	if raw, err := cfgOp.Get(pluginMarketSourcesKey); err == nil && strings.TrimSpace(raw) != "" {
		t.Fatalf("new market_sources key should remain unset, got %q", raw)
	}
}

// TestEnsureDefaultMarketSource_Idempotent：连调两次 → 仍恰好 [OfficialMarketSource]，无重复。
func TestEnsureDefaultMarketSource_Idempotent(t *testing.T) {
	m, _ := newMarketSourceTestManager(t)
	m.EnsureDefaultMarketSource()
	m.EnsureDefaultMarketSource()
	got := m.GetMarketSources()
	want := []string{OfficialMarketSource}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("repeated calls must not duplicate\n  got  %v\n  want %v", got, want)
	}
}

// TestEnsureDefaultMarketSource_NoCfgOpNoOp：cfgOp 未注入 → 安全返回不 panic。
func TestEnsureDefaultMarketSource_NoCfgOpNoOp(t *testing.T) {
	m := &Manager{}               // cfgOp nil
	m.EnsureDefaultMarketSource() // must not panic
	if got := m.GetMarketSources(); len(got) != 0 {
		t.Fatalf("no cfgOp should yield empty sources, got %v", got)
	}
}
