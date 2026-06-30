package site

import (
	"math"
	"os"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"strings"
)

// 本测试用「真实 yaml 定义 + 真实页面结构 fixture」钉死三站统计抽取：
// 好学(cute-top-profile) / 财神(Audiences menu-base-info) / 蟹黄堡(经典 info_block)。
// fixture 截取自 D:\html 真实页面，若站点改版或选择器写错，断言会失败。

const tiB = float64(1 << 40)
const giB = float64(1 << 30)

func loadFixture(t *testing.T, path string) *goquery.Document {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return doc
}

func mustRegistry(t *testing.T) *Registry {
	t.Helper()
	r := NewRegistry()
	if err := r.LoadDefinitions("../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	return r
}

func approxF(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-3 {
		t.Errorf("%s: got %g, want approx %g", name, got, want)
	}
}

// TestHxptUserInfo 好学 usercp(cute-top-profile) + userdetails(rowhead)。
func TestHxptUserInfo(t *testing.T) {
	def, ok := mustRegistry(t).Definition("hxpt")
	if !ok {
		t.Fatal("hxpt definition missing")
	}
	cp := loadFixture(t, "testdata/hxpt_usercp.html")

	if uid := ExtractUID(cp); uid != "8" {
		t.Errorf("uid: got %q want 8", uid)
	}
	u := ParseUserInfo(cp, def)
	approxF(t, "uploaded(TiB)", float64(u.Uploaded)/tiB, 104.564)
	approxF(t, "downloaded(TiB)", float64(u.Downloaded)/tiB, 2.839)
	approxF(t, "ratio", u.Ratio, 36.832)
	approxF(t, "bonus", u.Bonus, 79888500.6)
	if u.Seeding != 27 {
		t.Errorf("seeding: got %d want 27", u.Seeding)
	}
	if u.Leeching != 0 {
		t.Errorf("leeching: got %d want 0", u.Leeching)
	}

	// 合并 userdetails 的 level/join_date（多页合并不应清零已得统计）
	detail := loadFixture(t, "testdata/hxpt_userdetails.html")
	ParseUserDetail(detail, def, u)
	if u.Level != "维护开发员" {
		t.Errorf("level: got %q want 维护开发员", u.Level)
	}
	if u.JoinDate != "2025-07-16" {
		t.Errorf("join_date: got %q want 2025-07-16", u.JoinDate)
	}
	if u.Uploaded == 0 {
		t.Error("multi-page merge zeroed uploaded")
	}
}

// TestCsptUserInfo 财神 usercp(Audiences menu-base-info，color_* display:none + SVG title) + userdetails(main_rowhead)。
func TestCsptUserInfo(t *testing.T) {
	def, ok := mustRegistry(t).Definition("cspt")
	if !ok {
		t.Fatal("cspt definition missing")
	}
	cp := loadFixture(t, "testdata/cspt_usercp.html")

	if uid := ExtractUID(cp); uid != "10014" {
		t.Errorf("uid: got %q want 10014", uid)
	}
	u := ParseUserInfo(cp, def)
	approxF(t, "uploaded(TiB)", float64(u.Uploaded)/tiB, 44.069)
	approxF(t, "downloaded(TiB)", float64(u.Downloaded)/tiB, 3.312)
	approxF(t, "ratio", u.Ratio, 13.305)
	approxF(t, "bonus", u.Bonus, 5401685.6)
	if u.Seeding != 5 {
		t.Errorf("seeding: got %d want 5", u.Seeding)
	}
	if u.Leeching != 0 {
		t.Errorf("leeching: got %d want 0", u.Leeching)
	}

	detail := loadFixture(t, "testdata/cspt_userdetails.html")
	ParseUserDetail(detail, def, u)
	if u.Level != "总版主" {
		t.Errorf("level: got %q want 总版主", u.Level)
	}
	if u.JoinDate != "2025-02-22" {
		t.Errorf("join_date: got %q want 2025-02-22", u.JoinDate)
	}
}

// TestCrabptUserInfo 蟹黄堡 usercp(经典 info_block，3 个 color_bonus + 大 span 父节点) + userdetails(rowhead)。
// 关键：bonus 必须取首个 color_bonus(蟹币值)，uploaded 必须锚定「上传量」标签而非误抓 ratio 66.250。
func TestCrabptUserInfo(t *testing.T) {
	def, ok := mustRegistry(t).Definition("crabpt")
	if !ok {
		t.Fatal("crabpt definition missing")
	}
	cp := loadFixture(t, "testdata/crabpt_usercp.html")

	if uid := ExtractUID(cp); uid != "15299" {
		t.Errorf("uid: got %q want 15299", uid)
	}
	u := ParseUserInfo(cp, def)
	approxF(t, "uploaded(TiB)", float64(u.Uploaded)/tiB, 1.261)
	approxF(t, "downloaded(GiB)", float64(u.Downloaded)/giB, 19.50)
	approxF(t, "ratio", u.Ratio, 66.25)
	approxF(t, "bonus", u.Bonus, 1621449.1)
	if u.Seeding != 12 {
		t.Errorf("seeding: got %d want 12", u.Seeding)
	}
	if u.Leeching != 0 {
		t.Errorf("leeching: got %d want 0", u.Leeching)
	}

	detail := loadFixture(t, "testdata/crabpt_userdetails.html")
	ParseUserDetail(detail, def, u)
	if u.Level != "User" {
		t.Errorf("level: got %q want User", u.Level)
	}
	if u.JoinDate != "2024-09-19" {
		t.Errorf("join_date: got %q want 2024-09-19", u.JoinDate)
	}
}

// TestEngineSelectorsFallback Selectors 回退列表：坏选择器跳过、首个非空命中。
func TestEngineSelectorsFallback(t *testing.T) {
	doc := loadFixture(t, "testdata/hxpt_usercp.html")
	q := ElementQuery{
		Selectors: []string{".does-not-exist", "font.color_ratio"},
		Parent:    true,
		Filters:   []Filter{{Name: "regexp", Args: `(?:分享率)[：:]?\s*([-+]?\d+\.?\d*)`}},
	}
	if got := Extract(doc.Selection, q); got != "36.832" {
		t.Errorf("fallback extract: got %q want 36.832", got)
	}
}

// TestEngineRegexpGroup :::N 取指定捕获组（当前活动「27 0」一锚拆做种/下载中）。
func TestEngineRegexpGroup(t *testing.T) {
	doc := loadFixture(t, "testdata/crabpt_usercp.html")
	base := ElementQuery{Selector: "font.color_active", Parent: true}
	base.Filters = []Filter{{Name: "regexp", Args: `当前活动[：:]?\s*(\d+)\D+(\d+):::1`}}
	if got := ToInt(Extract(doc.Selection, base)); got != 12 {
		t.Errorf("group1(seeding): got %d want 12", got)
	}
	base.Filters = []Filter{{Name: "regexp", Args: `当前活动[：:]?\s*(\d+)\D+(\d+):::2`}}
	if got := ToInt(Extract(doc.Selection, base)); got != 0 {
		t.Errorf("group2(leeching): got %d want 0", got)
	}
}

// TestEngineParentNotSet 不带 Parent 时只取命中节点自身文本（回归保护：旧 search 选择器行为）。
func TestEngineParentNotSet(t *testing.T) {
	doc := loadFixture(t, "testdata/hxpt_usercp.html")
	// font.color_uploaded 自身文本只是「上传量:」，不应拿到 104.564 TB
	q := ElementQuery{Selector: "font.color_uploaded"}
	got := Extract(doc.Selection, q)
	if strings.Contains(got, "104") {
		t.Errorf("without parent, should not get value text; got %q", got)
	}
}
