// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

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

// TestHxptUserInfo 已随好学改版下线：站点引擎从 NexusPHP 切到 FluxTorrent，
// 用户信息改由 schemas/fluxtorrent.go 解析（见 fluxtorrent_test.go）；
// 旧 usercp/userdetails 刮削选择器与本用例一并删除。fixture 保留供
// 引擎回退/Parent 语义测试（TestEngineSelectorsFallback 等）使用。

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

func TestBTSchoolCurrentSeedingSize(t *testing.T) {
	def, ok := mustRegistry(t).Definition("btschool")
	if !ok {
		t.Fatal("btschool definition missing")
	}
	doc := docFrom(t, `<table>
		<tr><td class="rowhead">当前做种</td><td>3 torrents / 1.50 TiB</td></tr>
	</table>`)
	u := ParseUserInfo(doc, def)
	if u.Seeding != 3 {
		t.Fatalf("seeding = %d, want 3", u.Seeding)
	}
	if want := int64(1536 * (1 << 30)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}

func TestUnit3DDefinitionSeedingSizeFilters(t *testing.T) {
	tests := []struct {
		siteID string
		html   string
	}{
		{
			siteID: "aither",
			html: `<section>
				<span class="user-profile-card__meta-item-title">Seeding Size</span>
				<span class="user-profile-card__meta-item-value">1.50 TiB</span>
			</section>`,
		},
		{
			siteID: "capybarabr",
			html:   `<dl><dt>Tamanho em seeding</dt><dd>1.50 TiB</dd></dl>`,
		},
		{
			siteID: "latteam",
			html:   `<dl><dt>Tamaño total compartiendo</dt><dd>1.50 TiB</dd></dl>`,
		},
		{
			siteID: "portugas",
			html:   `<dl><dt>Tamanho das Sementes</dt><dd>1.50 TiB</dd></dl>`,
		},
	}
	reg := mustRegistry(t)
	for _, tt := range tests {
		t.Run(tt.siteID, func(t *testing.T) {
			def, ok := reg.Definition(tt.siteID)
			if !ok {
				t.Fatalf("%s definition missing", tt.siteID)
			}
			u := ParseUserInfo(docFrom(t, tt.html), def)
			if want := int64(1536 * (1 << 30)); u.SeedingSize != want {
				t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
			}
		})
	}
}

func TestJSONDefinitionSeedingSizeFilters(t *testing.T) {
	reg := mustRegistry(t)
	tests := []struct {
		name   string
		siteID string
		fields map[string]ElementQuery
		body   string
		want   int64
	}{
		{
			name:   "gazellegames",
			siteID: "gazellegames",
			body:   `{"response":{"community":{"seedSize":"1.25 TiB"}}}`,
			want:   int64(1280 * (1 << 30)),
		},
		{
			name:   "yemapt",
			siteID: "yemapt",
			body:   `{"data":{"fileSize":"2.50 TiB"}}`,
			want:   int64(2560 * (1 << 30)),
		},
		{
			name:   "zhuque",
			siteID: "zhuque",
			body:   `{"data":{"seedingSize":3221225472}}`,
			want:   int64(3 * (1 << 30)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, ok := reg.Definition(tt.siteID)
			if !ok {
				t.Fatalf("%s definition missing", tt.siteID)
			}
			fields := def.UserInfo.Fields
			if tt.siteID == "yemapt" {
				fields = def.UserInfo.Pages[2].Fields
			}
			if tt.siteID == "zhuque" {
				fields = def.UserInfo.Pages[0].Fields
			}
			u := &UserInfo{}
			ParseJSONUserInfoFields([]byte(tt.body), fields, u)
			if u.SeedingSize != tt.want {
				t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, tt.want)
			}
		})
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
