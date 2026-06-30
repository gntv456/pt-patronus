package site

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"gopkg.in/yaml.v3"
)

// 真页校准（开发期探针）：加载 D:\html 下 5 个目标站的真实控制面板/用户详情页，
// 用生产配置（真实 YAML + EnsureDefaults）跑 ParseUserInfo/ParseUserDetail，
// 逐字段报告抓到了什么，并 dump 关键 label 周围的原始标记窗口，用于校准选择器。
//
// 仅本机有 D:\html 时才跑；CI 无该目录自动 skip。这是临时校准工具，最终应转为
// 可移植的内联 HTML 回归测试（见 five_sites_calib_test.go 的固化断言）。
const calibHTMLDir = `D:/html`

type calibCase struct {
	site    string // definitions/<site>.yaml
	usercp  string // 控制面板 (usercp.php) —— 基础统计
	detail  string // 用户详情 (userdetails.php) —— 富字段
	skipCP  bool   // 该站无控制面板样本（如 Audiences 仅用户详情）
	skipDet bool   // 该站无用户详情样本
}

var calibCases = []calibCase{
	{site: "audiences", usercp: "Audiences __ 控制面板 - 首页 - Powered by NexusPHP.html", detail: "Audiences __ 用户详情 - gntv - Powered by NexusPHP.html"},
	{site: "cbg", usercp: "CBG __ 控制面板 - 首页 - Powered by NexusPHP.html", detail: "CBG __ 用户详情 - gntv - Powered by NexusPHP.html"},
	{site: "novahd", usercp: "NovaHD __ 控制面板 - 首页 - Powered by NexusPHP.html", detail: "NovaHD __ 用户详情 - gntv - Powered by NexusPHP.html"},
	{site: "ubits", usercp: "UBits __ 控制面板 - 首页 - Powered by NexusPHP.html", detail: "UBits __ 用户详情 - gntv - Powered by NexusPHP.html"},
	{site: "tangpt", usercp: "不可躺 __ 控制面板 - 首页 - Powered by NexusPHP.html", detail: "不可躺 __ 用户详情 - gntv - Powered by NexusPHP.html"},
}

// 校准探针关注的 label（用户条/资料页统计项），按站可能略有不同。
var calibLabels = []string{"上传量", "下载量", "分享率", "魔力值", "U币", "爆米花", "做种", "等级", "加入日期"}

func loadCalibDef(t *testing.T, site string) *SiteMetadata {
	t.Helper()
	p := filepath.Join("..", "..", "definitions", site+".yaml")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read def %s: %v", p, err)
	}
	var def SiteMetadata
	if err := yaml.Unmarshal(data, &def); err != nil {
		t.Fatalf("unmarshal %s: %v", p, err)
	}
	return EnsureDefaults(&def)
}

func loadCalibDoc(t *testing.T, name string) (*goquery.Document, string) {
	t.Helper()
	p := filepath.Join(calibHTMLDir, name)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, ""
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return doc, string(data)
}

func TestCalibFiveSites(t *testing.T) {
	if _, err := os.Stat(calibHTMLDir); err != nil {
		t.Skipf("calib dir %s not present: %v", calibHTMLDir, err)
	}
	for _, c := range calibCases {
		t.Run(c.site, func(t *testing.T) {
			def := loadCalibDef(t, c.site)
			t.Logf("=== %s (schema=%s) ===", c.site, def.Schema)

			// 基础统计：控制面板页（usercp.php）
			if !c.skipCP {
				if doc, raw := loadCalibDoc(t, c.usercp); doc != nil {
					u := ParseUserInfo(doc, def)
					t.Logf("[usercp] uploaded=%d downloaded=%d ratio=%.3f bonus=%.1f seeding=%d leeching=%d",
						u.Uploaded, u.Downloaded, u.Ratio, u.Bonus, u.Seeding, u.Leeching)
					if c.site == "audiences" {
						uid, name := ExtractUIDAndName(doc)
						t.Logf("  [audiences-usercp] uid=%q name=%q ud-stat卡片数=%d color_锚点数=%d",
							uid, name, doc.Find("div.ud-stat__value").Length(), doc.Find("font[class^='color_']").Length())
					}
					diagColorAnchors(t, doc) // 精确诊断 color_* 锚点命中与父节点文本
					dumpLabelWindows(t, c.site, "usercp", raw)
				} else {
					t.Logf("[usercp] SAMPLE MISSING: %s", c.usercp)
				}
			}

			// 富字段：用户详情页（userdetails.php）
			if !c.skipDet {
				if doc, raw := loadCalibDoc(t, c.detail); doc != nil {
					u := &UserInfo{}
					ParseUserDetail(doc, def, u)
					t.Logf("[detail] uploaded=%d downloaded=%d ratio=%.3f bonus=%.1f seeding=%d name=%q email=%q level=%q join_date=%q",
						u.Uploaded, u.Downloaded, u.Ratio, u.Bonus, u.Seeding, u.Name, u.Email, u.Level, u.JoinDate)
					if c.site == "audiences" {
						diagAudiencesDetail(t, doc) // 自定义皮肤 ud-stat 候选选择器校准
					}
					dumpLabelWindows(t, c.site, "detail", raw)
				} else {
					t.Logf("[detail] SAMPLE MISSING: %s", c.detail)
				}
			}
		})
	}
}

// diagColorAnchors 精确诊断 NexusPHP color_* 锚点：对每个锚点 font 打印命中数、
// 父节点 tag#id.class、父节点文本（折叠空白，前 200 字符）——定位 Parent+regex 失配根因。
func diagColorAnchors(t *testing.T, doc *goquery.Document) {
	for _, cls := range []string{"color_uploaded", "color_downloaded", "color_ratio", "color_bonus", "color_active"} {
		sel := doc.Find("font." + cls)
		n := sel.Length()
		if n == 0 {
			t.Logf("  [anchor] font.%s → 0 命中", cls)
			continue
		}
		first := sel.First()
		p := first.Parent()
		tag := goquery.NodeName(p)
		pid, _ := p.Attr("id")
		pcls, _ := p.Attr("class")
		ptxt := strings.Join(strings.Fields(p.Text()), " ")
		if len(ptxt) > 200 {
			ptxt = ptxt[:200] + "…"
		}
		t.Logf("  [anchor] font.%s → %d 命中; parent=<%s id=%q class=%q> text=%q", cls, n, tag, pid, pcls, ptxt)
	}
}

// diagAudiencesDetail 校准 Audiences 自定义皮肤（ud-stat__value/ud-stat__label）的候选选择器。
// Audiences 无 NexusPHP color_* 锚点，统计全在用户详情页 ud-stat 卡片 + hero meta，逐候选探测命中值。
func diagAudiencesDetail(t *testing.T, doc *goquery.Document) {
	type cand struct {
		name string
		q    ElementQuery
	}
	// ud-stat 卡片：<div class="ud-stat__body"><div class="ud-stat__value">V</div><div class="ud-stat__label">L</div></div>
	cands := []cand{
		{"uploaded", ElementQuery{Selectors: []string{`div.ud-stat__body:contains("上传量") div.ud-stat__value`}, Filters: []Filter{{Name: "parse_size"}}}},
		{"downloaded", ElementQuery{Selectors: []string{`div.ud-stat__body:contains("下载量") div.ud-stat__value`}, Filters: []Filter{{Name: "parse_size"}}}},
		{"ratio", ElementQuery{Selectors: []string{`div.ud-stat__body:contains("分享率") div.ud-stat__value`}}},
		{"bonus", ElementQuery{Selectors: []string{`div.ud-stat__body:contains("爆米花") div.ud-stat__value`}, Filters: []Filter{{Name: "parse_number"}}}},
		{"seeding_bonus", ElementQuery{Selectors: []string{`div.ud-stat__body:contains("做种") div.ud-stat__value`}, Filters: []Filter{{Name: "parse_number"}}}},
		{"level_badge", ElementQuery{Selectors: []string{`span.ud-class b`, `b[class$="_Name"]`}}},
		{"join_date", ElementQuery{Selectors: []string{`i.fa-calendar-plus`}, Parent: true, Filters: []Filter{{Name: "regexp", Args: `(\d{4}-\d{2}-\d{2})`}}}},
		{"seeding", ElementQuery{Selectors: []string{`span.site-userbar__compact-metric--active`, `[title*="活跃"]`}, Attr: "title", Filters: []Filter{{Name: "regexp", Args: `活跃[^0-9]*(\d+)`}}}},
		{"name_attr", ElementQuery{Selectors: []string{`[data-uploader-name]`}, Attr: "data-uploader-name"}},
	}
	for _, c := range cands {
		got := Extract(doc.Selection, c.q)
		t.Logf("  [audiences-cand] %s → %q", c.name, got)
	}
}

// dumpLabelWindows 在原始 HTML 里定位每个 label，打印其前后 ~250 字符的标记窗口，
// 便于直接看到真实 DOM（class/结构/值的位置），据此设计选择器。
func dumpLabelWindows(t *testing.T, site, where, raw string) {
	for _, label := range calibLabels {
		idx := strings.Index(raw, label)
		if idx < 0 {
			continue // 该 label 本页没有，跳过（不打噪声）
		}
		start := idx - 250
		if start < 0 {
			start = 0
		}
		end := idx + 250
		if end > len(raw) {
			end = len(raw)
		}
		// 折叠多余空白便于阅读
		win := strings.Join(strings.Fields(raw[start:end]), " ")
		t.Logf("[%s/%s] %q → …%s…", site, where, label, win)
	}
}
