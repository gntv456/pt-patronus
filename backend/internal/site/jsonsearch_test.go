package site

import "testing"

func TestJSONNavigate(t *testing.T) {
	root := map[string]any{"data": []any{map[string]any{"id": 1.0, "name": "x"}}}
	if _, ok := jsonNavigate(root, "data").([]any); !ok {
		t.Errorf("data 应导航到数组")
	}
	// 嵌套路径 data.torrents（zhuque 形）
	root2 := map[string]any{"data": map[string]any{"torrents": []any{map[string]any{"id": 7}}}}
	nested, _ := jsonNavigate(root2, "data.torrents").([]any)
	if len(nested) != 1 {
		t.Errorf("嵌套 data.torrents len=%d", len(nested))
	}
	// 数组索引
	arr := []any{map[string]any{"id": "a"}, map[string]any{"id": "b"}}
	if jsonNavigate(arr, "1") == nil {
		t.Errorf("数组索引 1 应命中")
	}
	// 缺键 → nil
	if jsonNavigate(root, "nope") != nil {
		t.Errorf("缺键应返 nil")
	}
}

func TestParseJSONSearchHDBits(t *testing.T) {
	body := []byte(`{"data":[{"id":1,"file_name":"X","size":1000,"seeders":5,"leechers":1,"times_completed":9,"type_category":"HD","utadded":"2024-01-01"}]}`)
	def := &SiteMetadata{Selectors: SiteSelectors{
		Rows: ElementQuery{Selector: "data"},
		Fields: map[string]ElementQuery{
			"title":    {Selector: "file_name"},
			"detail":   {Selector: "id"},
			"size":     {Selector: "size"},
			"seeders":  {Selector: "seeders"},
			"category": {Selector: "type_category"},
		},
	}}
	ts := ParseJSONSearch(body, def, "https://hdbits.org")
	if len(ts) != 1 {
		t.Fatalf("len=%d", len(ts))
	}
	if ts[0].Title != "X" || ts[0].Seeders != 5 || ts[0].Category != "HD" {
		t.Errorf("解析错误: %+v", ts[0])
	}
	// 大数字不被科学计数法破坏（UseNumber）：size 1000 → "1000"
	if ts[0].Size != "1000" {
		t.Errorf("size=%q 应为 1000（无科学计数法）", ts[0].Size)
	}
}

func TestParseJSONSearchNested(t *testing.T) {
	body := []byte(`{"data":{"torrents":[{"id":1,"title":"T","size":2000,"seeding":3,"leeching":1}]}}`)
	def := &SiteMetadata{Selectors: SiteSelectors{
		Rows: ElementQuery{Selector: "data.torrents"},
		Fields: map[string]ElementQuery{
			"title":   {Selector: "title"},
			"seeders": {Selector: "seeding"},
			"size":    {Selector: "size"},
		},
	}}
	ts := ParseJSONSearch(body, def, "https://zhuque.in")
	if len(ts) != 1 || ts[0].Title != "T" || ts[0].Seeders != 3 {
		t.Errorf("嵌套路径解析错误: %+v", ts)
	}
}

func TestParseJSONSearchSelectorsFallback(t *testing.T) {
	// 复数 selectors 回退（beyondhd 形：download 用 selectors 列表 + replace）
	body := []byte(`{"data":[{"id":1,"name":"T","download_url":"/dl/1.torrent"}]}`)
	def := &SiteMetadata{Selectors: SiteSelectors{
		Rows: ElementQuery{Selector: "data"},
		Fields: map[string]ElementQuery{
			"title": {Selector: "name"},
			"download": {Selectors: []string{"download_link", "download_url"}},
		},
	}}
	ts := ParseJSONSearch(body, def, "https://x.com")
	if len(ts) != 1 || ts[0].DownloadURL != "https://x.com/dl/1.torrent" {
		t.Errorf("复数回退+ResolveURL 错误: %+v", ts)
	}
}

func TestBuildJSONSearchURLPathPlaceholder(t *testing.T) {
	def := &SiteMetadata{SearchEntry: "/torrents/browse/list/query", SearchParam: "query"}
	u := BuildJSONSearchURL("https://www.torrentleech.org", def, "batman", 1)
	want := "https://www.torrentleech.org/torrents/browse/list/batman"
	if u != want {
		t.Errorf("路径占位代入错: got %q want %q", u, want)
	}
}

func TestBuildJSONSearchURLQueryParam(t *testing.T) {
	def := &SiteMetadata{SearchEntry: "/api/torrents", SearchParam: "search"}
	u := BuildJSONSearchURL("https://hdbits.org", def, "kw", 1)
	if u != "https://hdbits.org/api/torrents?search=kw" {
		t.Errorf("query-param 错: got %q", u)
	}
}

func TestLooksLikeJSON(t *testing.T) {
	cases := map[string]bool{
		`  {"a":1}`: true,
		`[1,2]`:     true,
		`<html>`:    false,
		``:          false,
	}
	for body, want := range cases {
		if got := LooksLikeJSON([]byte(body)); got != want {
			t.Errorf("LooksLikeJSON(%q)=%v want %v", body, got, want)
		}
	}
}
