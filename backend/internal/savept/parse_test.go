package savept

import (
	"os"
	"strings"
	"testing"
)

// 本测试用模拟 savept.icu 结构的 fixture 钉死解析器：卡片 data-* 抽取、状态归一、
// 公告切片、抢救时长回填。真实页面结构若有变动，按站点选择器校准工作流另存页面校准。

func mustReadFixture(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestParseSavept 卡片抽取 + 状态归一。
func TestParseSavept(t *testing.T) {
	html := mustReadFixture(t, "testdata/savept.html")
	cards, alerts := parseSavept(html)

	if len(cards) != 3 {
		t.Fatalf("cards: got %d want 3", len(cards))
	}
	byName := map[string]SaveptSiteCard{}
	for _, c := range cards {
		byName[c.Name] = c
	}
	ex := byName["示例站"]
	if ex.Status != HealthHealthy {
		t.Errorf("示例站 status: got %q want healthy", ex.Status)
	}
	if ex.Year != "2020" {
		t.Errorf("示例站 year: got %q want 2020", ex.Year)
	}
	if ex.Domain != "example.pt" {
		t.Errorf("示例站 domain: got %q want example.pt", ex.Domain)
	}
	if ex.SiteType != TypeInternal {
		t.Errorf("示例站 type: got %q want internal", ex.SiteType)
	}
	if ex.OpenTime != "2020-03-15" {
		t.Errorf("示例站 open_time: got %q want 2020-03-15", ex.OpenTime)
	}
	if ex.CloseTime != "" {
		t.Errorf("示例站 close_time: got %q want empty", ex.CloseTime)
	}
	if ex.AnniversaryFlag != "yes" {
		t.Errorf("示例站 anniversary_flag: got %q want yes", ex.AnniversaryFlag)
	}
	if ex.AnniversaryText != "今天站庆" {
		t.Errorf("示例站 anniversary_text: got %q want 今天站庆", ex.AnniversaryText)
	}
	if byName["病危站"].Status != HealthCritical {
		t.Errorf("病危站 status: got %q want critical", byName["病危站"].Status)
	}
	if byName["关站示例"].Status != HealthClosed {
		t.Errorf("关站示例 status: got %q want closed", byName["关站示例"].Status)
	}
	if got := byName["关站示例"].CloseTime; got != "2024-12-31" {
		t.Errorf("关站示例 close_time: got %q want 2024-12-31", got)
	}
	// 抢救时长回填：病危公告「病危站 已抢救 2 天」→ "2天"
	if got := byName["病危站"].FaultDuration; got != "2天" {
		t.Errorf("病危站 fault_duration: got %q want 2天", got)
	}

	// 公告：病危(danger) + 恢复(warning) + 站庆(info) + 讣告(obituary)
	var hasDanger, hasWarning, hasObituary, hasInfo bool
	var obituaryContent string
	for _, a := range alerts {
		switch a.Type {
		case AlertDanger:
			hasDanger = true
		case AlertWarning:
			hasWarning = true
		case AlertObituary:
			hasObituary = true
			obituaryContent = a.Content
		case AlertInfo:
			hasInfo = true
		}
	}
	if !hasDanger {
		t.Error("missing danger alert")
	}
	if !hasWarning {
		t.Error("missing warning alert")
	}
	if !hasObituary {
		t.Error("missing obituary alert")
	}
	if !hasInfo {
		t.Error("missing info alert")
	}
	// strong 定位：讣告正文不得卷入页脚联系方式（全页面关键词扫描会误卷）
	if strings.Contains(obituaryContent, "联系方式") {
		t.Errorf("obituary polluted by footer: %q", obituaryContent)
	}
}

// TestExtractFaultDurations 单位归一（天/小时/分钟）。
func TestExtractFaultDurations(t *testing.T) {
	cases := []struct {
		content string
		want    string
	}{
		{"病危站 已抢救 2 天", "2天"},
		{"某站 已抢救 48 小时", "2天"}, // 48h → 2天
		{"某站 已抢救 30 分钟", "抢救中（30分钟）"},
	}
	for _, c := range cases {
		got := extractFaultDurations([]SaveptAlert{{Type: AlertDanger, Content: c.content}})
		if len(got) != 1 || got[0].duration != c.want {
			t.Errorf("extractFaultDurations(%q): got %+v want %q", c.content, got, c.want)
		}
	}
}

// TestNormalizeStatus 关键词归一。
func TestNormalizeStatus(t *testing.T) {
	cases := []struct {
		raw, text string
		want      SiteHealthStatus
	}{
		{"", "正常运行", HealthHealthy},
		{"critical", "", HealthCritical},
		{"", "濒临关站", HealthCritical},
		{"", "已关站", HealthClosed},
		{"", "", HealthUnknown},
	}
	for _, c := range cases {
		if got := normalizeStatus(c.raw, c.text); got != c.want {
			t.Errorf("normalizeStatus(%q,%q) = %q want %q", c.raw, c.text, got, c.want)
		}
	}
}
