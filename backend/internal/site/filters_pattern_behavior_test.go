package site

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// #326 regexp 族 filter 编译失败的处理：过去是裸 `continue`（整条规则不生效），
// 于是原始父节点文本继续往下被 parse_size/parse_number 抽数 ⇒ 界面拿到「错值」而不是「没值」。
// 现在与「未命中」同一终态：给空值。

func TestApplyFiltersBrokenRegexpGivesNoValueNotRawText(t *testing.T) {
	raw := "File Size: 350.6 MB  Language: English  魔力值: 18889"
	got := ApplyFilters(raw, []Filter{{Name: "regexp", Args: `(?=Category:\s+(.+))`}, {Name: "parse_size"}})
	if got != "" {
		t.Errorf("编不出正则必须给空值，不许把原始文本往下传（旧行为会抽出一个无关数字）：实际 %q", got)
	}
	// 正控：同一链路的合法写法照旧抽得到值。
	ok := ApplyFilters("Category: Audio Books Language: English", []Filter{
		{Name: "regexp", Args: `Category:\s+(.+?)\s+(?:Language:|Keywords:)`},
	})
	if ok != "Audio Books" {
		t.Errorf("正控失效：合法 regexp 规则应抽出 %q，实际 %q", "Audio Books", ok)
	}
}

func TestApplyFiltersBrokenSumMatchesGivesNoValue(t *testing.T) {
	got := ApplyFilters("1.5 GB 2 GB 魔力值: 18889", []Filter{{Name: "sum_matches", Args: `(\d+`}, {Name: "parse_number"}})
	if got != "" {
		t.Errorf("sum_matches 编不出正则也必须给空值（不是「合计 0」，也不是原始文本）：实际 %q", got)
	}
	// 正控：合法 pattern 照旧求和。
	sum := ApplyFilters("1.5 GB 2 GB", []Filter{{Name: "sum_matches", Args: `([\d.]+)`}})
	if sum != "3.5" {
		t.Errorf("正控失效：sum_matches 应得 3.5，实际 %q", sum)
	}
}

// TestAudiobookbayCategoryPattern 钉住这条活洞的修法：definitions/audiobookbay.yaml
// 的 subtitle 用了 RE2 不支持的先行断言，规则从未生效过。
func TestAudiobookbayCategoryPattern(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "definitions", "audiobookbay.yaml"))
	if err != nil {
		t.Fatalf("读不到 audiobookbay.yaml：%v", err)
	}
	pat := ""
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.Index(line, `args: "`); i >= 0 && strings.Contains(line, "Category:") {
			rest := line[i+len(`args: "`):]
			j := strings.LastIndex(rest, `"`)
			if j > 0 {
				pat = strings.ReplaceAll(rest[:j], `\\`, `\`)
			}
			break
		}
	}
	if pat == "" {
		t.Fatalf("没从 YAML 里取出 Category 那条 pattern（判据就成了空的）")
	}
	if _, err := regexp.Compile(pat); err != nil {
		t.Errorf("audiobookbay 的 subtitle 规则仍然编不出来（该站 subtitle 会一直空）：%q err=%v", pat, err)
	}
	got := ApplyFilters("Category: Audio Books > Fiction  Language: English  Keywords: audiobooks", []Filter{{Name: "regexp", Args: pat}})
	if got != "Audio Books > Fiction" {
		t.Errorf("subtitle 应只取到分类段，实际 %q（含 Language/Keywords 就是污染）", got)
	}
	if strings.Contains(got, "Language") || strings.Contains(got, "Keywords") {
		t.Errorf("抽出的值里不该带后续 label：%q", got)
	}
}
