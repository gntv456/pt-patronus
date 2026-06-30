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
	// parse_time：相对时间应解析为今天附近的日期（非空、非原值）
	if got := ApplyFilters("2 days ago", []Filter{{Name: "parse_time"}}); got == "" || got == "2 days ago" {
		t.Errorf("parse_time relative 应解析为日期, got %q", got)
	}
	// parse_time：无法识别保留原值
	if got := ApplyFilters("hello", []Filter{{Name: "parse_time"}}); got != "hello" {
		t.Errorf("parse_time 无法识别应保留原值, got %q", got)
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
