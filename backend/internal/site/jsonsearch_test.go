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
	if ts[0].Size != "1000 B" {
		t.Errorf("size=%q 应为 1000 B", ts[0].Size)
	}
}

func TestParseJSONSearchFormatsNumericSizeBytes(t *testing.T) {
	body := []byte(`{"data":[{"title":"T","size":1024}]}`)
	def := &SiteMetadata{Selectors: SiteSelectors{
		Rows: ElementQuery{Selector: "data"},
		Fields: map[string]ElementQuery{
			"title": {Selector: "title"},
			"size":  {Selector: "size"},
		},
	}}
	ts := ParseJSONSearch(body, def, "https://json.test")
	if len(ts) != 1 {
		t.Fatalf("len=%d", len(ts))
	}
	if ts[0].Size != "1.00 KiB" {
		t.Fatalf("size = %q, want 1.00 KiB", ts[0].Size)
	}
}

func TestParseJSONSearchHumanReadableSizePreserved(t *testing.T) {
	body := []byte(`{"data":[{"title":"T","size":"1.50 GiB"}]}`)
	def := &SiteMetadata{Selectors: SiteSelectors{
		Rows: ElementQuery{Selector: "data"},
		Fields: map[string]ElementQuery{
			"title": {Selector: "title"},
			"size":  {Selector: "size"},
		},
	}}
	ts := ParseJSONSearch(body, def, "https://json.test")
	if len(ts) != 1 {
		t.Fatalf("len=%d", len(ts))
	}
	if ts[0].Size != "1.50 GiB" {
		t.Fatalf("size = %q, want 1.50 GiB", ts[0].Size)
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

func TestParseJSONSearchRowsSelectorsFallback(t *testing.T) {
	body := []byte(`{"data":{"data":[{"id":1,"title":"Fallback","seeding":4}]}}`)
	def := &SiteMetadata{Selectors: SiteSelectors{
		Rows: ElementQuery{Selectors: []string{"missing.rows", "data.data"}},
		Fields: map[string]ElementQuery{
			"title":   {Selector: "title"},
			"seeders": {Selector: "seeding"},
		},
	}}
	ts := ParseJSONSearch(body, def, "https://huno.test")
	if len(ts) != 1 || ts[0].Title != "Fallback" || ts[0].Seeders != 4 {
		t.Fatalf("rows selectors fallback failed: %+v", ts)
	}
}

func TestParseJSONSearchRootArrayRowsSelf(t *testing.T) {
	body := []byte(`[{"id":123,"name":"Root Array","size":2147483648,"seeders":8,"leechers":1,"timesCompleted":22,"category":"Movie"}]`)
	def := &SiteMetadata{Selectors: DefaultRartrackerSelectors()}
	ts := ParseJSONSearch(body, def, "https://superbits.example")
	if len(ts) != 1 {
		t.Fatalf("len=%d", len(ts))
	}
	if ts[0].Title != "Root Array" || ts[0].Seeders != 8 || ts[0].Completed != 22 {
		t.Fatalf("torrent = %+v", ts[0])
	}
	if ts[0].DetailURL != "https://superbits.example/torrent/123" {
		t.Fatalf("detail = %q", ts[0].DetailURL)
	}
}

func TestParseJSONSearchSelectorsFallback(t *testing.T) {
	// 复数 selectors 回退（beyondhd 形：download 用 selectors 列表 + replace）
	body := []byte(`{"data":[{"id":1,"name":"T","download_url":"/dl/1.torrent"}]}`)
	def := &SiteMetadata{Selectors: SiteSelectors{
		Rows: ElementQuery{Selector: "data"},
		Fields: map[string]ElementQuery{
			"title":    {Selector: "name"},
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

func TestBuildJSONSearchURLPreservesEntryQuery(t *testing.T) {
	def := &SiteMetadata{SearchEntry: "/api/v1/torrents?limit=100", SearchParam: "searchText"}
	u := BuildJSONSearchURL("https://superbits.example", def, "kw", 2)
	if u != "https://superbits.example/api/v1/torrents?limit=100&page=2&searchText=kw" {
		t.Errorf("query-param with existing query wrong: got %q", u)
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

func TestParseJSONUserInfoFieldsExtendedTraffic(t *testing.T) {
	body := []byte(`{"data":{"trueUploaded":2199023255552,"trueDownloaded":549755813888,"trueRatio":4,"seedingPoints":12345.5,"seedingPointsPerHour":77.25,"claims":"认领 4/9","lastUpdateTime":1783404000000}}`)
	fields := map[string]ElementQuery{
		"true_uploaded":          {Selector: "data.trueUploaded"},
		"true_downloaded":        {Selector: "data.trueDownloaded"},
		"true_ratio":             {Selector: "data.trueRatio"},
		"seeding_bonus":          {Selector: "data.seedingPoints"},
		"seeding_bonus_per_hour": {Selector: "data.seedingPointsPerHour"},
		"claim_count":            {Selector: "data.claims", Filters: []Filter{{Name: "claim_count"}}},
		"max_claim":              {Selector: "data.claims", Filters: []Filter{{Name: "claim_max"}}},
		"last_access_at":         {Selector: "data.lastUpdateTime", Filters: []Filter{{Name: "unix_millis"}}},
	}

	u := &UserInfo{}
	ParseJSONUserInfoFields(body, fields, u)

	if want := int64(2 * (1 << 40)); u.TrueUploaded != want {
		t.Fatalf("true_uploaded = %d, want %d", u.TrueUploaded, want)
	}
	if want := int64(512 * (1 << 30)); u.TrueDownloaded != want {
		t.Fatalf("true_downloaded = %d, want %d", u.TrueDownloaded, want)
	}
	if u.TrueRatio != 4 || u.SeedingBonus != 12345.5 || u.SeedingBonusPerHour != 77.25 {
		t.Fatalf("extended stats = ratio %g seed_bonus %g seed_bonus_hour %g", u.TrueRatio, u.SeedingBonus, u.SeedingBonusPerHour)
	}
	if u.ClaimCount != 4 || u.MaxClaim != 9 {
		t.Fatalf("claims = %d/%d, want 4/9", u.ClaimCount, u.MaxClaim)
	}
	if u.LastAccessAt != "2026-07-07" {
		t.Fatalf("last_access_at = %q, want 2026-07-07", u.LastAccessAt)
	}
}

func TestParseJSONUserInfoFieldsProfileFields(t *testing.T) {
	body := []byte(`{"data":{"levelId":7,"numericUserId":42,"avatar":"https://img.example/avatar.png","personalAvatar":"https://img.example/personal.png","donor":true}}`)
	fields := map[string]ElementQuery{
		"level_id":        {Selector: "data.levelId"},
		"numeric_user_id": {Selector: "data.numericUserId"},
		"avatar":          {Selector: "data.avatar", Filters: []Filter{{Name: "clean_avatar"}}},
		"personal_avatar": {Selector: "data.personalAvatar", Filters: []Filter{{Name: "clean_avatar"}}},
		"is_donor":        {Selector: "data.donor", Filters: []Filter{{Name: "bool"}}},
	}

	u := &UserInfo{}
	ParseJSONUserInfoFields(body, fields, u)

	if u.LevelID != 7 || u.NumericUserID != 42 {
		t.Fatalf("ids = level_id %d numeric_user_id %d", u.LevelID, u.NumericUserID)
	}
	if u.Avatar != "https://img.example/avatar.png" || u.PersonalAvatar != "https://img.example/personal.png" {
		t.Fatalf("avatars = %q / %q", u.Avatar, u.PersonalAvatar)
	}
	if !u.IsDonor {
		t.Fatal("is_donor should be true")
	}
}
