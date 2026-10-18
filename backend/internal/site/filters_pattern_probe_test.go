package site

import (
	"path/filepath"
	"testing"
)

// #326 启动期体检的覆盖面：现网 definitions/*.yaml（306 个）+ 内置 defaults 都必须干净；
// 正控注入一条 RE2 不支持的写法，必须被抓出——否则「0 条」这个读数不算证据。

func TestLintFilterPatternsCoversDefinitionsAndControl(t *testing.T) {
	r := NewRegistry()
	dir := filepath.Join("..", "..", "definitions")
	if err := r.LoadDefinitions(dir); err != nil {
		t.Fatalf("加载 %s 失败：%v", dir, err)
	}
	defs := r.Definitions()
	if len(defs) < 100 {
		t.Fatalf("站点定义基数只有 %d（探针没打到东西，下面的「0 条」不算证据）", len(defs))
	}
	bad := r.LintFilterPatterns()
	t.Logf("YAML 站点定义基数=%d，体检命中=%d", len(defs), len(bad))
	for _, b := range bad {
		// 历史上这里抓到过 audiobookbay 的 subtitle（用了 RE2 不支持的 (?=…)）：
		// 那条规则从未生效过，抓取时把整段 postContent 当成 subtitle 显示。
		t.Errorf("站点 %s 的规则永远不生效 path=%s name=%s pattern=%q err=%s", b.Site, b.Path, b.Name, b.Pattern, b.Err)
	}

	// 内置 defaults（Go 字面量，不走 YAML）同判据。
	builtinBad := 0
	builtinN := 0
	for _, schema := range []string{"gazelle", "luminance", "nexusphp", "unit3d", "mtorrent", "pter", "hdchina", "hdbits", "smallhorse", "tcg", "avistaz", "flarum"} {
		def := EnsureDefaults(&SiteMetadata{ID: "builtin:" + schema, Schema: schema})
		builtinN++
		if got := LintFilterPatterns(def); len(got) > 0 {
			builtinBad += len(got)
			for _, b := range got {
				t.Errorf("内置 defaults %s 规则不生效 path=%s pattern=%q err=%s", schema, b.Path, b.Pattern, b.Err)
			}
		}
	}
	t.Logf("内置 defaults 抽查=%d，体检命中=%d", builtinN, builtinBad)

	// 正控 1：regexp 支，RE2 不支持的先行断言。
	control := &SiteMetadata{
		ID:       "control",
		UserInfo: UserInfoDef{Pages: []UserInfoPageDef{{Fields: map[string]ElementQuery{"ratio": {Filters: []Filter{{Name: "regexp", Args: `Ratio:\s+(.+?)(?= bonus)`}}}}}}},
	}
	got := LintFilterPatterns(control)
	if len(got) != 1 {
		t.Fatalf("正控失效：注入一条坏 regexp 必须抓到 1 条，实际 %d 条", len(got))
	}
	if got[0].Name != "regexp" || got[0].Site != "control" {
		t.Errorf("正控读数形状不对（要能指出是哪个站哪条规则）：%+v", got[0])
	}

	// 正控 2：sum_matches 支也在判据覆盖内（同族两处，别只盯 regexp）。
	control2 := &SiteMetadata{
		ID:       "control2",
		UserInfo: UserInfoDef{Pages: []UserInfoPageDef{{Fields: map[string]ElementQuery{"size": {Filters: []Filter{{Name: "sum_matches", Args: `([\d.]+`}}}}}}},
	}
	if got2 := LintFilterPatterns(control2); len(got2) != 1 {
		t.Errorf("sum_matches 支必须同样被体检到，实际 %d 条（判据漏了这一族支路）", len(got2))
	}

	// 正控 3：args 带 :::N 组号后缀时，被编译的必须是去掉后缀的 pattern。
	control3 := &SiteMetadata{
		ID:       "control3",
		UserInfo: UserInfoDef{Pages: []UserInfoPageDef{{Fields: map[string]ElementQuery{"x": {Filters: []Filter{{Name: "regexp", Args: `ok:::1`}}}}}}},
	}
	if got3 := LintFilterPatterns(control3); len(got3) != 0 {
		t.Errorf("`ok:::1` 的 pattern 是 `ok`，不该被判坏：%+v", got3)
	}
	control4 := &SiteMetadata{
		ID:       "control4",
		UserInfo: UserInfoDef{Pages: []UserInfoPageDef{{Fields: map[string]ElementQuery{"x": {Filters: []Filter{{Name: "regexp", Args: `(?::::1`}}}}}}},
	}
	if got4 := LintFilterPatterns(control4); len(got4) != 1 {
		t.Errorf("坏 pattern 带 :::1 后缀也必须被抓到（后缀要在编译前切掉），实际 %d 条", len(got4))
	}
}
