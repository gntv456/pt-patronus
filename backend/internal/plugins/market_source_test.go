package plugins

import (
	"reflect"
	"testing"
)

// TestNormalizeMarketSource 覆盖 GitHub 仓库 URL → raw 转换、简写、tree 分支/子目录、以及
// raw/file/其它 http(s) 源原样返回等分支。
func TestNormalizeMarketSource(t *testing.T) {
	cases := []struct{ in, want string }{
		// GitHub 仓库页 → raw 默认分支 main / 根目录 plugin-market.json
		{"https://github.com/owner/repo", "https://raw.githubusercontent.com/owner/repo/main/plugin-market.json"},
		{"https://github.com/owner/repo/", "https://raw.githubusercontent.com/owner/repo/main/plugin-market.json"},
		// tree/{branch} 与 tree/{branch}/{path}
		{"https://github.com/owner/repo/tree/dev", "https://raw.githubusercontent.com/owner/repo/dev/plugin-market.json"},
		{"https://github.com/owner/repo/tree/dev/market", "https://raw.githubusercontent.com/owner/repo/dev/market/plugin-market.json"},
		// 无 scheme 与 owner/repo 简写
		{"github.com/owner/repo", "https://raw.githubusercontent.com/owner/repo/main/plugin-market.json"},
		{"owner/repo", "https://raw.githubusercontent.com/owner/repo/main/plugin-market.json"},
		{"https://www.github.com/owner/repo", "https://raw.githubusercontent.com/owner/repo/main/plugin-market.json"},
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
