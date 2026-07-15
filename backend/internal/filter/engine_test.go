package filter

import (
	"testing"

	"github.com/hxpt/ptpatronus/internal/site"
)

// ruleRules 构造一份规则集（内置若干 + 自造）。
func testRules() map[string]Rule {
	return map[string]Rule{
		"4K":    BuiltinRuleSet["4K"],
		"1080P": BuiltinRuleSet["1080P"],
		"BLU":   BuiltinRuleSet["BLU"],
		"H265":  BuiltinRuleSet["H265"],
		"FREE":  BuiltinRuleSet["FREE"],
		"MYINC": {ID: "MYINC", Include: []string{`特效`}},
		"MYEXC": {ID: "MYEXC", Include: []string{`1080p`}, Exclude: []string{`x265`}},
	}
}

func TestMatchOrderCascade(t *testing.T) {
	rules := testRules()
	asts, err := parseLevels("4K > 1080P")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		title string
		want  int
	}{
		{"Movie 2024 2160p UHD BluRay", 100}, // 4K 命中 → pri 100
		{"Movie 2024 1080p WEB-DL x264", 99}, // 4K 未命中、1080P 命中 → 99
		{"Movie 2024 720p HDTV x264", 0},     // 都未命中 → 剔除
	}
	for _, c := range cases {
		tor := site.Torrent{Title: c.title}
		got := matchOrder(tor, asts, nil, rules)
		if got != c.want {
			t.Errorf("title=%q pri=%d want=%d", c.title, got, c.want)
		}
	}
}

func TestMatchRuleIncludeExclude(t *testing.T) {
	rules := testRules()
	// MYEXC：include 1080p 且 exclude x265。
	ok := matchRule(site.Torrent{Title: "Show 1080p x264"}, "MYEXC", nil, rules)
	if !ok {
		t.Fatal("1080p x264 应匹配 MYEXC")
	}
	bad := matchRule(site.Torrent{Title: "Show 1080p x265"}, "MYEXC", nil, rules)
	if bad {
		t.Fatal("1080p x265 应被 MYEXC 的 exclude 拒绝")
	}
}

func TestMatchRuleFree(t *testing.T) {
	rules := testRules()
	free := matchRule(site.Torrent{Title: "X", Free: true}, "FREE", nil, rules)
	if !free {
		t.Fatal("Free=true 应匹配 FREE(factor 0)")
	}
	notFree := matchRule(site.Torrent{Title: "X", Free: false}, "FREE", nil, rules)
	if notFree {
		t.Fatal("Free=false 不应匹配 FREE")
	}
}

func TestMatchRuleTMDBShortCircuit(t *testing.T) {
	rules := map[string]Rule{
		"CNSUB": BuiltinRuleSet["CNSUB"], // tmdb original_language=zh,cn
	}
	media := &MediaInfo{OriginalLanguage: "zh"}
	// 标题里没有任何中字特征，仅靠 TMDB 原声 zh 短路命中。
	got := matchRule(site.Torrent{Title: "English.Title.2024"}, "CNSUB", media, rules)
	if !got {
		t.Fatal("TMDB original_language=zh 应短路命中 CNSUB")
	}
	// 无 media → 不能短路 → 标题无中字 → 不命中。
	got2 := matchRule(site.Torrent{Title: "English.Title.2024"}, "CNSUB", nil, rules)
	if got2 {
		t.Fatal("无 media 且标题无中字特征时 CNSUB 不应命中")
	}
}

func TestMatchSizeRange(t *testing.T) {
	// matchSize(sizeStr, range, episodeCount)
	if !matchSize("3GB", "1000-5000", 1) {
		t.Fatal("3GB 应在 1000-5000MB 区间")
	}
	if matchSize("500MB", "1000-5000", 1) {
		t.Fatal("500MB 不在 1000-5000MB 区间")
	}
	if !matchSize("2GB", ">1000", 1) {
		t.Fatal("2GB 应 >= 1000MB")
	}
	if matchSize("500MB", ">1000", 1) {
		t.Fatal("500MB 不 >= 1000MB")
	}
	if !matchSize("500MB", "<1000", 1) {
		t.Fatal("500MB 应 <= 1000MB")
	}
	// 纯数字（无 >/</-）→ unknown → 不匹配（对齐 MP-2）。
	if matchSize("3GB", "1000", 1) {
		t.Fatal("纯数字 size_range 应为 unknown 不匹配")
	}
}

func TestParseSizeRangeFormats(t *testing.T) {
	if op, min, max := parseSizeRange("1000-5000"); op != opBetween || min != 1000*sizeUnit || max != 5000*sizeUnit {
		t.Errorf("between 解析异常 op=%s min=%d max=%d", op, min, max)
	}
	if op, min, _ := parseSizeRange(">1000"); op != opGTE || min != 1000*sizeUnit {
		t.Errorf("gte 解析异常 op=%s min=%d", op, min)
	}
	if op, _, max := parseSizeRange("<5000"); op != opLTE || max != 5000*sizeUnit {
		t.Errorf("lte 解析异常 op=%s max=%d", op, max)
	}
	if op, _, _ := parseSizeRange("1000"); op != opUnknown {
		t.Errorf("纯数字应为 unknown，实际 %s", op)
	}
}

// TestIsFree：订阅「仅免费」过滤依赖 IsFree（下载因子=0 即免费）。须与 torrentDownloadFactor
// 一致：DownloadVolumeFactor>0 时以其值为准（≠0 即不免费），否则看 Free。
func TestIsFree(t *testing.T) {
	cases := []struct {
		name string
		t    site.Torrent
		want bool
	}{
		{"Free 标记", site.Torrent{Free: true}, true},
		{"无任何标记", site.Torrent{}, false},
		{"促销因子0.5", site.Torrent{DownloadVolumeFactor: 0.5}, false},
		{"促销因子1（全速）", site.Torrent{DownloadVolumeFactor: 1}, false},
		{"Free 但有非零因子（因子优先）", site.Torrent{Free: true, DownloadVolumeFactor: 0.5}, false},
		{"Free 且因子0", site.Torrent{Free: true, DownloadVolumeFactor: 0}, true},
	}
	for _, c := range cases {
		if got := IsFree(c.t); got != c.want {
			t.Errorf("%s: IsFree=%v want %v (factor=%v free=%v)",
				c.name, got, c.want, c.t.DownloadVolumeFactor, c.t.Free)
		}
	}
}
