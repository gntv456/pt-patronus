package site

import (
	"strings"
	"testing"
)

const searchFixture = `<html><body>
<table class="torrents">
  <tr><th>分类</th><th>标题</th><th>下载</th><th>大小</th><th>种子</th><th>下载者</th><th>完成</th></tr>
  <tr>
    <td>电影</td>
    <td><a href="details.php?id=100">Movie 2024 1080p</a><span class="free">Free!</span></td>
    <td><a href="download.php?id=100&amp;passkey=xx">下载</a></td>
    <td>4.3 GB</td>
    <td>12</td>
    <td>3</td>
    <td>500</td>
  </tr>
</table></body></html>`

func defForSearch() *SiteMetadata {
	return &SiteMetadata{
		Selectors: SiteSelectors{
			Rows: ElementQuery{Selector: "table.torrents tr"},
			Fields: map[string]ElementQuery{
				"title":     {Selector: "td:nth-child(2) a"},
				"detail":    {Selector: "td:nth-child(2) a", Attr: "href"},
				"free":      {Selector: ".free"},
				"download":  {Selector: "td:nth-child(3) a", Attr: "href"},
				"size":      {Selector: "td:nth-child(4)"},
				"seeders":   {Selector: "td:nth-child(5)"},
				"leechers":  {Selector: "td:nth-child(6)"},
				"completed": {Selector: "td:nth-child(7)"},
				"category":  {Selector: "td:nth-child(1)"},
			},
		},
	}
}

func TestParseSearch(t *testing.T) {
	doc := docFrom(t, searchFixture)
	ts := ParseSearch(doc, defForSearch(), "https://demo.pt")
	if len(ts) != 1 {
		t.Fatalf("expected 1 torrent (header row skipped), got %d", len(ts))
	}
	tt := ts[0]
	if tt.Title != "Movie 2024 1080p" {
		t.Errorf("title = %q", tt.Title)
	}
	if tt.Size != "4.3 GB" {
		t.Errorf("size = %q", tt.Size)
	}
	if tt.Seeders != 12 || tt.Leechers != 3 || tt.Completed != 500 {
		t.Errorf("seeders/leechers/completed = %d/%d/%d", tt.Seeders, tt.Leechers, tt.Completed)
	}
	if tt.DetailURL != "https://demo.pt/details.php?id=100" {
		t.Errorf("detail = %q", tt.DetailURL)
	}
	if tt.DownloadURL != "https://demo.pt/download.php?id=100&passkey=xx" {
		t.Errorf("download = %q", tt.DownloadURL)
	}
	if !tt.Free {
		t.Error("expected Free=true")
	}
}

func TestParseSearchRowsCompat(t *testing.T) {
	doc := docFrom(t, `<html><body>
<table class="torrents"><tbody><tr><td>old</td></tr></tbody></table>
<table class="torrents"><tbody>
  <tr><td><a href="details.php?id=2">Real row</a></td><td>7 GiB</td></tr>
</tbody></table>
</body></html>`)
	def := &SiteMetadata{
		Selectors: SiteSelectors{
			Rows: ElementQuery{Selectors: []string{"table.missing > tr", "table.torrents:last > tbody > tr"}},
			Fields: map[string]ElementQuery{
				"title":  {Selector: "> td:nth-child(1) a"},
				"detail": {Selector: "> td:nth-child(1) a", Attr: "href"},
				"size":   {Selector: "> td:nth-child(2)"},
			},
		},
	}
	ts := ParseSearch(doc, def, "https://demo.pt")
	if len(ts) != 1 {
		t.Fatalf("expected 1 torrent from fallback rows selector, got %d", len(ts))
	}
	if ts[0].Title != "Real row" || ts[0].Size != "7 GiB" {
		t.Fatalf("unexpected torrent: %+v", ts[0])
	}
}

func TestParseSearchWithMergedNexusPHPDefaults(t *testing.T) {
	doc := docFrom(t, `<html><body><table class="custom"><tbody>
<tr>
  <td class="cat">Movie</td>
  <td><a href="details.php?id=10">Merged default row</a></td>
  <td>comments</td>
  <td>2026-01-02</td>
  <td>1.25 GiB</td>
  <td><a href="details.php?id=10#seeders">9</a></td>
  <td><a href="details.php?id=10#leechers">2</a></td>
  <td><a href="viewsnatches.php?id=10">33</a></td>
  <td><a href="download.php?id=10">DL</a></td>
</tr>
</tbody></table></body></html>`)
	def := EnsureDefaults(&SiteMetadata{
		Schema: "nexusphp",
		Selectors: SiteSelectors{
			Rows:   ElementQuery{Selector: "table.custom > tbody > tr"},
			Fields: map[string]ElementQuery{"category": {Selector: "td.cat"}},
		},
	})
	ts := ParseSearch(doc, def, "https://demo.pt")
	if len(ts) != 1 {
		t.Fatalf("expected 1 torrent, got %d", len(ts))
	}
	got := ts[0]
	if got.Title != "Merged default row" || got.Category != "Movie" {
		t.Fatalf("title/category = %q/%q", got.Title, got.Category)
	}
	if got.Size != "1.25 GiB" || got.Seeders != 9 || got.Leechers != 2 || got.Completed != 33 {
		t.Fatalf("stats = size %q seeders %d leechers %d completed %d", got.Size, got.Seeders, got.Leechers, got.Completed)
	}
	if got.DetailURL != "https://demo.pt/details.php?id=10" {
		t.Fatalf("detail = %q", got.DetailURL)
	}
	if got.DownloadURL != "https://demo.pt/download.php?id=10" {
		t.Fatalf("download = %q", got.DownloadURL)
	}
}

func TestBuildSearchURL(t *testing.T) {
	def := &SiteMetadata{SearchEntry: "torrents.php", SearchParam: "search"}
	got := BuildSearchURL("https://demo.pt/", def, "hello 世界", "", 0)
	if got != "https://demo.pt/torrents.php?search=hello+%E4%B8%96%E7%95%8C" {
		t.Errorf("url = %q", got)
	}
}

func TestBuildSearchURLPreservesEntryQuery(t *testing.T) {
	def := &SiteMetadata{SearchEntry: "/search.php?do=process", SearchParam: "query"}
	got := BuildSearchURL("https://demo.pt", def, "batman", "", 2)
	if got != "https://demo.pt/search.php?do=process&page=2&query=batman" {
		t.Errorf("url = %q", got)
	}
}

func TestBuildSearchURLCategory(t *testing.T) {
	def := &SiteMetadata{
		SearchEntry: "torrents.php", SearchParam: "search",
		SearchCategories: map[string][]string{"movie": {"401", "408"}},
	}
	got := BuildSearchURL("https://demo.pt", def, "kw", "movie", 0)
	// 两个 cat[] 都应出现
	if !strings.Contains(got, "cat%5B%5D=401") || !strings.Contains(got, "cat%5B%5D=408") {
		t.Errorf("movie cats missing in url = %q", got)
	}
	// 无映射的类型不发 cat[]
	if g := BuildSearchURL("https://demo.pt", def, "kw", "music", 0); strings.Contains(g, "cat") {
		t.Errorf("music should not emit cat[]: %q", g)
	}
}

func TestExtractUIDAndNameSupportsUserPHP(t *testing.T) {
	doc := docFrom(t, `<html><body><a href="user.php?id=321"><span>hares-user</span></a></body></html>`)
	uid, name := ExtractUIDAndName(doc)
	if uid != "321" || name != "hares-user" {
		t.Fatalf("uid/name = %q/%q, want 321/hares-user", uid, name)
	}
	if got := ExtractUID(doc); got != "321" {
		t.Fatalf("ExtractUID = %q, want 321", got)
	}
}

func TestFilterByCategory(t *testing.T) {
	in := []Torrent{
		{Title: "电影A", Category: "电影"},
		{Title: "剧集B", Category: "剧集"},
		{Title: "音乐C", Category: "音乐"},
		{Title: "音乐D", Category: "Music Album FLAC"},
		{Title: "空分类E", Category: ""},
	}
	// 搜电影：丢两条音乐，留三条
	movie := FilterByCategory(in, "movie")
	if len(movie) != 3 {
		t.Fatalf("movie filter: got %d, want 3: %+v", len(movie), movie)
	}
	for _, x := range movie {
		if x.Title == "音乐C" || x.Title == "音乐D" {
			t.Errorf("music leaked into movie results: %q", x.Title)
		}
	}
	// 综合不过滤
	if all := FilterByCategory(in, ""); len(all) != len(in) {
		t.Errorf("empty category should not filter: got %d want %d", len(all), len(in))
	}
	// 音乐搜索也不过滤（用户主动搜音乐）
	if mus := FilterByCategory(in, "music"); len(mus) != len(in) {
		t.Errorf("music search should not filter: got %d want %d", len(mus), len(in))
	}
}

func TestIsLoggedOut(t *testing.T) {
	def := NoLoginDef{
		HttpStatus: []int{401},
		URLPattern: []string{"login.php"},
		Selector:   "form.login",
		Content:    []string{"请先登录"},
	}
	if !IsLoggedOut(401, "https://x/", nil, def) {
		t.Error("http status 401 should be logged out")
	}
	if !IsLoggedOut(200, "https://x/login.php", nil, def) {
		t.Error("redirect to login.php should be logged out")
	}
	if IsLoggedOut(200, "https://x/index.php", nil, def) {
		t.Error("normal page should NOT be logged out")
	}
	doc := docFrom(t, `<form class="login">请先登录</form>`)
	if !IsLoggedOut(200, "https://x/", doc, def) {
		t.Error("login form / content should be logged out")
	}
}

func TestIsVIP(t *testing.T) {
	def := &SiteMetadata{UserInfo: UserInfoDef{VIPLevels: []string{"VIP", "贵宾"}}}
	if !IsVIP(&UserInfo{Level: "VIP"}, def) {
		t.Error("VIP level should match")
	}
	if !IsVIP(&UserInfo{Level: " 贵宾 "}, def) {
		t.Error("贵宾 level should match (trim/case)")
	}
	if IsVIP(&UserInfo{Level: "Peasant"}, def) {
		t.Error("Peasant should not be VIP")
	}
}

func TestParseUserInfoFieldsExtendedTraffic(t *testing.T) {
	doc := docFrom(t, `<table>
<tr><td class="rowhead">传输</td><td>
上传量 5.00 TiB 下载量 1.00 TiB 分享率 5.0
实际上传量 2.00 TiB 实际下载量 512.00 GiB 实际分享率 4.0
做种积分 12,345.5
认领 3/8
</td></tr>
<tr><td class="rowhead">最近动向</td><td>2026-07-07 06:00 (刚刚)</td></tr>
</table>`)
	fields := map[string]ElementQuery{
		"true_uploaded":   {Selector: `td.rowhead:contains("传输") + td`, Filters: []Filter{{Name: "regexp", Args: `实际上传量\s*([\d.]+\s*TiB)`}, {Name: "parse_size"}}},
		"true_downloaded": {Selector: `td.rowhead:contains("传输") + td`, Filters: []Filter{{Name: "regexp", Args: `实际下载量\s*([\d.]+\s*GiB)`}, {Name: "parse_size"}}},
		"true_ratio":      {Selector: `td.rowhead:contains("传输") + td`, Filters: []Filter{{Name: "regexp", Args: `实际分享率\s*([\d.]+)`}}},
		"seeding_bonus":   {Selector: `td.rowhead:contains("传输") + td`, Filters: []Filter{{Name: "regexp", Args: `做种积分\s*([\d,.]+)`}, {Name: "parse_number"}}},
		"claim_count":     {Selector: `td.rowhead:contains("传输") + td`, Filters: []Filter{{Name: "claim_count"}}},
		"max_claim":       {Selector: `td.rowhead:contains("传输") + td`, Filters: []Filter{{Name: "claim_max"}}},
		"last_access_at":  {Selector: `td.rowhead:contains("最近动向") + td`, Filters: []Filter{{Name: "regexp", Args: `^([^(]+)`}, {Name: "trim"}}},
	}

	u := &UserInfo{}
	ParseUserInfoFields(doc, fields, u)

	if want := int64(2 * (1 << 40)); u.TrueUploaded != want {
		t.Fatalf("true_uploaded = %d, want %d", u.TrueUploaded, want)
	}
	if want := int64(512 * (1 << 30)); u.TrueDownloaded != want {
		t.Fatalf("true_downloaded = %d, want %d", u.TrueDownloaded, want)
	}
	if u.TrueRatio != 4 {
		t.Fatalf("true_ratio = %g, want 4", u.TrueRatio)
	}
	if u.SeedingBonus != 12345.5 {
		t.Fatalf("seeding_bonus = %g, want 12345.5", u.SeedingBonus)
	}
	if u.ClaimCount != 3 || u.MaxClaim != 8 {
		t.Fatalf("claims = %d/%d, want 3/8", u.ClaimCount, u.MaxClaim)
	}
	if u.LastAccessAt != "2026-07-07 06:00" {
		t.Fatalf("last_access_at = %q", u.LastAccessAt)
	}
}

func TestParseUserInfoFieldsProfileFields(t *testing.T) {
	doc := docFrom(t, `<html><body>
<h1>Alice <img src="pic/flag/donor.gif" alt="Donor"></h1>
<table>
  <tr><td class="rowhead">等级</td><td><img src="pic/class/6.gif" title="Veteran User"></td></tr>
  <tr><td class="rowhead">头像</td><td><img src="/avatars/alice.png"></td></tr>
  <tr><td class="rowhead">默认头像</td><td><img class="default" src="/pic/default_avatar.png"></td></tr>
</table>
<a href="invite.php?id=42">邀请</a>
<input name="avatar" value="https://cdn.example/alice-personal.png">
</body></html>`)
	fields := map[string]ElementQuery{
		"level_id":        {Selector: `td.rowhead:contains("等级") + td img`, Attr: "src", Filters: []Filter{{Name: "regexp", Args: `/class/(\d+)\.gif`}, {Name: "add", Args: "-1"}}},
		"numeric_user_id": {Selector: `a[href*="invite.php?id="]`, Attr: "href", Filters: []Filter{{Name: "queryparam", Args: "id"}}},
		"avatar":          {Selector: `td.rowhead:contains("头像") + td img`, Attr: "src", Filters: []Filter{{Name: "clean_avatar"}}},
		"personal_avatar": {Selector: `input[name="avatar"]`, Attr: "value", Filters: []Filter{{Name: "clean_avatar"}}},
		"is_donor":        {Selector: `h1 img[alt="Donor"]`, Attr: "alt", Filters: []Filter{{Name: "bool"}}},
	}

	u := &UserInfo{}
	ParseUserInfoFields(doc, fields, u)

	if u.LevelID != 5 || u.NumericUserID != 42 {
		t.Fatalf("ids = level_id %d numeric_user_id %d", u.LevelID, u.NumericUserID)
	}
	if u.Avatar != "/avatars/alice.png" || u.PersonalAvatar != "https://cdn.example/alice-personal.png" {
		t.Fatalf("avatars = %q / %q", u.Avatar, u.PersonalAvatar)
	}
	if !u.IsDonor {
		t.Fatal("is_donor should be true")
	}
}

func TestApplyFiltersProfileHelpers(t *testing.T) {
	if got := ApplyFilters("/pic/default_avatar.png", []Filter{{Name: "clean_avatar"}}); got != "" {
		t.Fatalf("clean_avatar default = %q, want empty", got)
	}
	if got := ApplyFilters("Donor", []Filter{{Name: "bool"}}); got != "true" {
		t.Fatalf("bool donor = %q, want true", got)
	}
	if got := ApplyFilters("class/6.gif", []Filter{{Name: "regexp", Args: `class/(\d+)\.gif`}, {Name: "add", Args: "-1"}}); got != "5" {
		t.Fatalf("level add = %q, want 5", got)
	}
}
