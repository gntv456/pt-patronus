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

func TestBuildSearchURL(t *testing.T) {
	def := &SiteMetadata{SearchEntry: "torrents.php", SearchParam: "search"}
	got := BuildSearchURL("https://demo.pt/", def, "hello 世界", "", 0)
	if got != "https://demo.pt/torrents.php?search=hello+%E4%B8%96%E7%95%8C" {
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
