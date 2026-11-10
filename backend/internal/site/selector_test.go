// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package site

import (
	"strconv"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestExtractTextAndAttr(t *testing.T) {
	doc := docFrom(t, `<div class="row"><a href="d.php?id=9" class="t">标题 <em>2024</em></a></div>`)
	row := doc.Find(".row")
	if got := Extract(row, ElementQuery{Selector: "a", Attr: "href"}); got != "d.php?id=9" {
		t.Errorf("attr = %q", got)
	}
	if got := Extract(row, ElementQuery{Selector: "a"}); !strings.Contains(got, "标题") {
		t.Errorf("text = %q", got)
	}
}

func TestExtractAttrOrTextFallbacks(t *testing.T) {
	doc := docFrom(t, `<div>
		<span class="rank" title="Power User">ignored</span>
		<span class="rank-text">Elite User</span>
		<time class="joined" datetime="2024-05-06T12:00:00Z" title="ignored">joined</time>
		<time class="joined-title" title="2024-05-07T12:00:00Z">joined</time>
		<time class="joined-text">2024-05-08</time>
	</div>`)
	if got := Extract(doc.Selection, ElementQuery{Selector: ".rank", Attr: "title_or_text"}); got != "Power User" {
		t.Errorf("title_or_text title = %q", got)
	}
	if got := Extract(doc.Selection, ElementQuery{Selector: ".rank-text", Attr: "title_or_text"}); got != "Elite User" {
		t.Errorf("title_or_text fallback = %q", got)
	}
	if got := Extract(doc.Selection, ElementQuery{Selector: ".joined", Attr: "datetime_or_title_or_text"}); got != "2024-05-06T12:00:00Z" {
		t.Errorf("datetime_or_title_or_text datetime = %q", got)
	}
	if got := Extract(doc.Selection, ElementQuery{Selector: ".joined-title", Attr: "datetime_or_title_or_text"}); got != "2024-05-07T12:00:00Z" {
		t.Errorf("datetime_or_title_or_text title = %q", got)
	}
	if got := Extract(doc.Selection, ElementQuery{Selector: ".joined-text", Attr: "datetime_or_title_or_text"}); got != "2024-05-08" {
		t.Errorf("datetime_or_title_or_text fallback = %q", got)
	}
}

func TestExtractRemove(t *testing.T) {
	doc := docFrom(t, `<a href="x">标题 <em>删我</em></a>`)
	got := Extract(doc.Selection, ElementQuery{Selector: "a", Remove: "em"})
	if strings.Contains(got, "删我") {
		t.Errorf("remove failed: %q", got)
	}
	if !strings.Contains(got, "标题") {
		t.Errorf("lost title: %q", got)
	}
}

// TestExtractEmptyNoSelector 无选择器且无过滤应返回空（详情页未映射字段不应吐整页文本）。
// 仅带过滤链时空选择器仍对整上下文取值（"任意位置正则"模式保留）。
func TestExtractEmptyNoSelector(t *testing.T) {
	doc := docFrom(t, `<html><head><title>整页</title></head><body>正文</body></html>`)
	// 空选择器 + 无过滤 → ""，不是整页文本
	if got := Extract(doc.Selection, ElementQuery{}); got != "" {
		t.Errorf("empty query should return \"\", got %q", got)
	}
	// 空选择器 + 有过滤 → 在整上下文上跑正则（保留"任意位置抽取"）
	got := Extract(doc.Selection, ElementQuery{Filters: []Filter{{Name: "regexp", Args: `整页`}}})
	if got != "整页" {
		t.Errorf("filter-only query should regex over doc, got %q", got)
	}
}

func TestApplyFilters(t *testing.T) {
	cases := []struct {
		in   string
		f    Filter
		want string
	}{
		{"上传: 12.3 TB", Filter{Name: "regexp", Args: `[\d.]+`}, "12.3"},
		{"hello world", Filter{Name: "replace", Args: "world:::pt"}, "hello pt"},
		{"  trim me  ", Filter{Name: "trim"}, "trim me"},
		{"3.14 GiB", Filter{Name: "parse_number"}, "3.14"},
		{"ABC", Filter{Name: "lower"}, "abc"},
	}
	for _, c := range cases {
		got := ApplyFilters(c.in, []Filter{c.f})
		if got != c.want {
			t.Errorf("ApplyFilters(%q, %+v) = %q, want %q", c.in, c.f, got, c.want)
		}
	}
}

func TestParseBytes(t *testing.T) {
	cases := map[string]int64{
		"1 GB":     1 << 30,
		"512 MB":   512 << 20,
		"2 TB":     2 << 40,
		"2T":       2 << 40,
		"1,024 KB": 1 << 20,
		"100":      100,
		"无限":       0,
		"":         0,
	}
	for in, want := range cases {
		if got := ParseBytes(in); got != want {
			t.Errorf("ParseBytes(%q) = %d, want %d", in, got, want)
		}
	}
}

// TestApplyFiltersPTDepiler 覆盖为 PT-depiler 移植新增的三个 filter：parse_size / queryparam / parse_time。
func TestApplyFiltersPTDepiler(t *testing.T) {
	// parse_size：复用 ParseBytes，转字节数字符串
	want := strconv.FormatInt(int64(1.5*float64(1<<40)), 10) // 1.5 TB = 1.5 * 2^40
	if got := ApplyFilters("1.5 TB", []Filter{{Name: "parse_size"}}); got != want {
		t.Errorf("parse_size 1.5TB = %q, want %q", got, want)
	}
	// queryparam：从 URL/含 query 串取参数
	if got := ApplyFilters("details.php?id=12345&page=2", []Filter{{Name: "queryparam", Args: "id"}}); got != "12345" {
		t.Errorf("queryparam id = %q, want 12345", got)
	}
	if got := ApplyFilters("/torrents/9876/download", []Filter{{Name: "queryparam", Args: "id"}}); got != "/torrents/9876/download" {
		t.Errorf("queryparam 无匹配应原样返回, got %q", got)
	}
	// parse_time：绝对时间确定性输出
	if got := ApplyFilters("2024-01-15 10:30:00", []Filter{{Name: "parse_time"}}); got != "2024-01-15" {
		t.Errorf("parse_time abs = %q, want 2024-01-15", got)
	}
	if got := ApplyFilters("2024/03/08", []Filter{{Name: "parse_time"}}); got != "2024-03-08" {
		t.Errorf("parse_time slash = %q, want 2024-03-08", got)
	}
	if got := ApplyFilters("12:34:56 31/12/2024", []Filter{{Name: "dmy_hms"}}); got != "2024-12-31" {
		t.Errorf("dmy_hms = %q, want 2024-12-31", got)
	}
	if got := ApplyFilters("12:34 31-Dec-2024", []Filter{{Name: "parse_time"}}); got != "2024-12-31" {
		t.Errorf("parse_time dmy short = %q, want 2024-12-31", got)
	}
	if got := ApplyFilters("070626 12:34:56", []Filter{{Name: "parse_time"}}); got != "2026-07-06" {
		t.Errorf("parse_time mmddyy = %q, want 2026-07-06", got)
	}
	if got := ApplyFilters("2024-01-0215:04:05 -07:00", []Filter{{Name: "parse_time"}}); got != "2024-01-02" {
		t.Errorf("parse_time compact tz = %q, want 2024-01-02", got)
	}
	if got := ApplyFilters("2026年7月6日 12:34:56", []Filter{{Name: "parse_time"}}); got != "2026-07-06" {
		t.Errorf("parse_time cn datetime = %q, want 2026-07-06", got)
	}
	if got := ApplyFilters("06-07-2026, 12:34:56", []Filter{{Name: "parse_time"}}); got != "2026-07-06" {
		t.Errorf("parse_time dmy comma = %q, want 2026-07-06", got)
	}
	if got := ApplyFilters("06_07_2026", []Filter{{Name: "parse_time"}}); got != "2026-07-06" {
		t.Errorf("parse_time dmy underscore = %q, want 2026-07-06", got)
	}
	if got := ApplyFilters("06/07/2026", []Filter{{Name: "parse_time"}}); got != "2026-07-06" {
		t.Errorf("parse_time dmy slash = %q, want 2026-07-06", got)
	}
	if got := ApplyFilters("Movies%2FHD", []Filter{{Name: "url_decode"}, {Name: "replace", Args: "/::: > "}}); got != "Movies > HD" {
		t.Errorf("url_decode = %q, want Movies > HD", got)
	}
	if got := ApplyFilters("UCoin240", []Filter{{Name: "regexp", Args: `UCoin(\d+)`}, {Name: "divide", Args: "24"}}); got != "10" {
		t.Errorf("divide = %q, want 10", got)
	}
	if got := ApplyFilters("Total Time Seeded: 2 Days", []Filter{{Name: "regexp", Args: `Total Time Seeded:\s*([\d,]+)\s*Days`}, {Name: "multiply", Args: "86400"}}); got != "172800" {
		t.Errorf("multiply = %q, want 172800", got)
	}
	// parse_time：相对时间应解析为今天附近的日期（非空、非原值）
	if got := ApplyFilters("2 days ago", []Filter{{Name: "parse_time"}}); got == "" || got == "2 days ago" {
		t.Errorf("parse_time relative 应解析为日期, got %q", got)
	}
	if got := ApplyFilters("1 hour and 50 minutes ago", []Filter{{Name: "parse_time"}}); got == "" || got == "1 hour and 50 minutes ago" {
		t.Errorf("parse_time compound relative 应解析为日期, got %q", got)
	}
	if got := ApplyFilters("1.5 hours ago", []Filter{{Name: "parse_time"}}); got == "" || got == "1.5 hours ago" {
		t.Errorf("parse_time decimal relative 应解析为日期, got %q", got)
	}
	// parse_time：无法识别保留原值
	if got := ApplyFilters("hello", []Filter{{Name: "parse_time"}}); got != "hello" {
		t.Errorf("parse_time 无法识别应保留原值, got %q", got)
	}
	if got := ApplyFilters("123", []Filter{{Name: "prepend", Args: "/browse/"}}); got != "/browse/123" {
		t.Errorf("prepend = %q, want /browse/123", got)
	}
	if got := ApplyFilters("2024-01-02T03:04:05+02:00", []Filter{{Name: "split", Args: "+:::0"}}); got != "2024-01-02T03:04:05" {
		t.Errorf("split = %q, want 2024-01-02T03:04:05", got)
	}
	if got := ApplyFilters("1 GiB 512 MiB", []Filter{{Name: "sum_size"}}); got != strconv.FormatInt(1536*(1<<20), 10) {
		t.Errorf("sum_size = %q, want 1610612736", got)
	}
	if got := ApplyFilters("a count:2 b count:3.5", []Filter{{Name: "sum_matches", Args: `count:([\d.]+)`}}); got != "5.5" {
		t.Errorf("sum_matches = %q, want 5.5", got)
	}
	if got := ApplyFilters("power", []Filter{{Name: "map", Args: "user=User:::power=PowerUser"}}); got != "PowerUser" {
		t.Errorf("map = %q, want PowerUser", got)
	}
	if got := ApplyFilters("a \n  b\tc", []Filter{{Name: "collapse_space"}}); got != "a b c" {
		t.Errorf("collapse_space = %q, want a b c", got)
	}
	if got := ApplyFilters("vr asian packs", []Filter{{Name: "title_case"}}); got != "VR Asian Packs" {
		t.Errorf("title_case = %q, want VR Asian Packs", got)
	}
	if got := ApplyFilters("foo sprite_r1080p bar", []Filter{{Name: "contains_map", Args: "sprite_r4k2k=4K/2K:::sprite_r1080p=1080P"}}); got != "1080P" {
		t.Errorf("contains_map = %q, want 1080P", got)
	}
	if got := ApplyFilters(`{"fid":123,"filename":"sample.torrent"}`, []Filter{{Name: "json_format", Args: "/download/{fid}/{filename}"}}); got != "/download/123/sample.torrent" {
		t.Errorf("json_format = %q, want /download/123/sample.torrent", got)
	}
	if got := ApplyFilters("anything", []Filter{{Name: "const", Args: "/"}}); got != "/" {
		t.Errorf("const = %q, want /", got)
	}
	if got := ApplyFilters("1704067200", []Filter{{Name: "unix_seconds"}}); got != "2024-01-01" {
		t.Errorf("unix_seconds = %q, want 2024-01-01", got)
	}
	if got := ApplyFilters("January 02, 2024, 15:04:05", []Filter{{Name: "parse_time"}}); got != "2024-01-02" {
		t.Errorf("parse_time full month comma = %q, want 2024-01-02", got)
	}
	if got := ApplyFilters("May 06 2024, 12:00", []Filter{{Name: "parse_time"}}); got != "2024-05-06" {
		t.Errorf("parse_time short month no comma = %q, want 2024-05-06", got)
	}
	if got := ApplyFilters("2024-01-02 15:04 +0000", []Filter{{Name: "parse_time"}}); got != "2024-01-02" {
		t.Errorf("parse_time offset = %q, want 2024-01-02", got)
	}
	if got := ApplyFilters("Yesterday at 15:04:05", []Filter{{Name: "parse_time"}}); got == "" || got == "Yesterday at 15:04:05" {
		t.Errorf("parse_time yesterday-at 应解析为日期, got %q", got)
	}
	if got := ApplyFilters("Date/time submitted: Today 01:59", []Filter{{Name: "parse_time"}}); got == "" || got == "Date/time submitted: Today 01:59" {
		t.Errorf("parse_time animetosho today 应解析为日期, got %q", got)
	}
	if got := ApplyFilters("Yesterday 23:39", []Filter{{Name: "parse_time"}}); got == "" || got == "Yesterday 23:39" {
		t.Errorf("parse_time animetosho yesterday 应解析为日期, got %q", got)
	}
	if got := ApplyFilters("Date: 2010-06-07 at 13:29 Central US (UTC -9:00)", []Filter{{Name: "anisource_time"}}); got != "2010-06-07" {
		t.Errorf("anisource_time = %q, want 2010-06-07", got)
	}
	btEtreeRow := `<td>artist</td><td><a class="details_link" href="details.php?id=1&torrentId=621061"><b>show</b></a></td><td></td><td></td><td>01/02 03:04</td>`
	if got := ApplyFilters(btEtreeRow, []Filter{{Name: "btetree_time"}}); got != "2024-01-02" {
		t.Errorf("btetree_time = %q, want 2024-01-02", got)
	}
	mamJSON := `{"seedUnsat":{"count":2,"size":100},"seedHnr":{"count":3,"size":200},"sSat":{"count":4,"size":300},"upAct":{"count":5,"size":400},"upInact":{"count":6,"size":500}}`
	if got := ApplyFilters(mamJSON, []Filter{{Name: "myanonamouse_seeding"}}); got != "14" {
		t.Errorf("myanonamouse_seeding = %q, want 14", got)
	}
	if got := ApplyFilters(mamJSON, []Filter{{Name: "myanonamouse_seeding_size"}}); got != "1000" {
		t.Errorf("myanonamouse_seeding_size = %q, want 1000", got)
	}
	if got := ApplyFilters(mamJSON, []Filter{{Name: "myanonamouse_uploads"}}); got != "11" {
		t.Errorf("myanonamouse_uploads = %q, want 11", got)
	}
	rescueHTML := `<table>
<tr><td><a href="userdetails.php?id=1">old</a></td><td>a</td><td>b</td><td>c</td><td>d</td><td>120</td></tr>
<tr><td>ignored</td><td>a</td><td>b</td><td>c</td><td>d</td><td>999</td></tr>
<tr><td><a href="userdetails.php?id=2">me</a></td><td>a</td><td>b</td><td>c</td><td>d</td><td>80</td></tr>
</table>`
	if got := ApplyFilters(rescueHTML, []Filter{{Name: "hhanclub_rescue_bonus_per_hour"}}); got != "3.333" {
		t.Errorf("hhanclub_rescue_bonus_per_hour = %q, want 3.333", got)
	}
	pterSearchHTML := `<p class="np-pager"><b>1</b><b>2</b><b>7</b></p>
<table class="torrents"><tbody><tr><th>标题</th></tr><tr><td>A</td></tr></tbody></table>`
	if got := ApplyFilters(pterSearchHTML, []Filter{{Name: "pter_upload_search_count"}}); got != "7" {
		t.Errorf("pter_upload_search_count pager = %q, want 7", got)
	}
	pterRowsHTML := `<table class="torrents"><tbody><tr><th>标题</th></tr><tr><td>A</td></tr><tr><td>B</td></tr></tbody></table>`
	if got := ApplyFilters(pterRowsHTML, []Filter{{Name: "pter_upload_search_count"}}); got != "2" {
		t.Errorf("pter_upload_search_count rows = %q, want 2", got)
	}
}

func TestExtractSelfSelector(t *testing.T) {
	doc := docFrom(t, `<div class="row" data-id="42"><a href="x">Title</a></div>`)
	row := doc.Find(".row")
	if got := Extract(row, ElementQuery{Selector: ":self", Attr: "data-id"}); got != "42" {
		t.Errorf("self selector attr = %q, want 42", got)
	}
}

func TestExtractData(t *testing.T) {
	doc := docFrom(t, `<div class="row" data-category-id="2" data-uploaded-by="me"><a href="x">Title</a></div>`)
	row := doc.Find(".row")
	if got := Extract(row, ElementQuery{Selector: ":self", Data: "categoryId"}); got != "2" {
		t.Errorf("data categoryId = %q, want 2", got)
	}
	if got := Extract(row, ElementQuery{Selector: ":self", Data: "uploaded_by"}); got != "me" {
		t.Errorf("data uploaded_by = %q, want me", got)
	}
}

func TestExtractTextConstant(t *testing.T) {
	doc := docFrom(t, `<div class="row"><a href="x">Title</a></div>`)
	if got := Extract(doc.Selection, ElementQuery{Text: "XXX"}); got != "XXX" {
		t.Errorf("text constant = %q, want XXX", got)
	}
	if got := Extract(doc.Selection, ElementQuery{Selector: ".missing", Text: "0"}); got != "0" {
		t.Errorf("text fallback = %q, want 0", got)
	}
	if got := Extract(doc.Selection, ElementQuery{Selector: "a", Text: "fallback"}); got != "Title" {
		t.Errorf("selector should win over text fallback, got %q", got)
	}
}

func TestExtractAncestorFind(t *testing.T) {
	doc := docFrom(t, `<div class="group_content"><div class="name_col"><span class="name"><b class="video_name">Group Title</b></span></div><div class="torrent_item"><a>Item</a></div></div>`)
	got := Extract(doc.Find(".torrent_item"), ElementQuery{Selector: ":self", Ancestor: ".group_content", Find: ".name_col > .name > .video_name"})
	if got != "Group Title" {
		t.Errorf("ancestor/find = %q, want Group Title", got)
	}
}

func TestExtractJQueryPseudoSelectors(t *testing.T) {
	doc := docFrom(t, `<div>
		<a href="/u/1">one</a><a href="/u/2">two</a>
		<table><tr><td class="header">Uploaded</td><td>1 GiB</td><td>2 GiB</td></tr><tr class="row"><td>A</td></tr><tr class="row"><td>B</td></tr><tr class="row"><td>C</td></tr></table>
		<section><center><h1><font color="blue">A</font><font color="blue">B</font><font color="blue">C</font></h1></center></section>
	</div>`)
	if got := Extract(doc.Selection, ElementQuery{Selector: `a[href^="/u/"]:first`}); got != "one" {
		t.Errorf(":first = %q, want one", got)
	}
	if got := Extract(doc.Selection, ElementQuery{Selector: `a[href^="/u/"]:last`}); got != "two" {
		t.Errorf(":last = %q, want two", got)
	}
	if got := Extract(doc.Selection, ElementQuery{Selector: `a:eq(1)`}); got != "two" {
		t.Errorf(":eq = %q, want two", got)
	}
	if got := Extract(doc.Selection, ElementQuery{Selector: `section center:eq(0) h1 font[color='blue']:eq(2)`}); got != "C" {
		t.Errorf("chained :eq = %q, want C", got)
	}
	if got := Extract(doc.Selection, ElementQuery{Selector: `td.header + td:first`}); got != "1 GiB" {
		t.Errorf("adjacent + :first = %q, want 1 GiB", got)
	}
	if got := Extract(doc.Selection, ElementQuery{Selector: `tr.row:gt(0)`, Attr: "text_all"}); got != "BC" {
		t.Errorf(":gt text_all = %q, want BC", got)
	}
	if got := Extract(doc.Selection, ElementQuery{Selector: `tr.row:lt(2)`, Attr: "text_all"}); got != "AB" {
		t.Errorf(":lt text_all = %q, want AB", got)
	}
}

func TestExtractLeadingCombinatorSelectors(t *testing.T) {
	doc := docFrom(t, `<table><tbody><tr class="torrent"><td>cat</td><td>title</td><td>3.5 GiB</td></tr></tbody></table>`)
	row := doc.Find("tr.torrent")
	if got := Extract(row, ElementQuery{Selector: `> td:nth-child(2)`}); got != "title" {
		t.Errorf("leading child combinator = %q, want title", got)
	}
	if got := Extract(row.Find("td").First(), ElementQuery{Selector: `+ td`}); got != "title" {
		t.Errorf("leading adjacent combinator = %q, want title", got)
	}
	if got := Extract(row.Find("td").First(), ElementQuery{Selector: `~ td`, Attr: "text_all"}); got != "title3.5 GiB" {
		t.Errorf("leading sibling combinator = %q, want title3.5 GiB", got)
	}
}

func TestApplyFiltersHTMLHelpers(t *testing.T) {
	raw := `Main <em>Title</em><br><span>Sub Title</span>`
	if got := ApplyFilters(raw, []Filter{{Name: "html_line", Args: "0"}}); got != "Main Title" {
		t.Errorf("html_line 0 = %q, want Main Title", got)
	}
	if got := ApplyFilters(raw, []Filter{{Name: "html_line", Args: "1"}}); got != "Sub Title" {
		t.Errorf("html_line 1 = %q, want Sub Title", got)
	}
	if got := ApplyFilters("A \n B\tC", []Filter{{Name: "collapse_join", Args: " | "}}); got != "A | B | C" {
		t.Errorf("collapse_join = %q, want A | B | C", got)
	}
}

func TestApplyFiltersHDBitsTitle(t *testing.T) {
	raw := `{"filename":"Release.Name.torrent","name":"Display Name","type_category":1,"type_medium":3}`
	if got := ApplyFilters(raw, []Filter{{Name: "hdbits_title"}}); got != "Release.Name" {
		t.Errorf("hdbits_title filename = %q, want Release.Name", got)
	}
	raw = `{"filename":"Disc.Name.torrent","name":"Display Name","type_category":1,"type_medium":1}`
	if got := ApplyFilters(raw, []Filter{{Name: "hdbits_title"}}); got != "Display Name" {
		t.Errorf("hdbits_title disc = %q, want Display Name", got)
	}
}

func TestApplyFiltersSiteSpecificSubtitles(t *testing.T) {
	kg := `<td></td><td><a><b>Title</b></a><span>Notes</span></td><td>Director</td><td>1999</td><td>Drama</td><td><a href="browse.php?country=1"><img title="France"></a></td>`
	if got := ApplyFilters(kg, []Filter{{Name: "karagarga_subtitle"}}); got != "Director / 1999 / France / Drama / Notes" {
		t.Errorf("karagarga_subtitle = %q", got)
	}
	rf := `<a href="/browse/t/1">Title</a><a>1080p</a><div class="x">Genre: <a href="/browse?genres=1">Drama</a></div>`
	if got := ApplyFilters(rf, []Filter{{Name: "retroflix_subtitle"}}); got != "Genres: Drama / 1080p" {
		t.Errorf("retroflix_subtitle = %q", got)
	}
	bitpt := `<a><span class="t_t">Title</span><span class="t_t_d_n"> ** Sub Title ** </span></a>`
	if got := ApplyFilters(bitpt, []Filter{{Name: "bitpt_subtitle"}}); got != "Sub Title" {
		t.Errorf("bitpt_subtitle = %q", got)
	}
}

func docFrom(t *testing.T, html string) *goquery.Document {
	t.Helper()
	d, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	return d
}
