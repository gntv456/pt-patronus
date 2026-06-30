package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// TestGazelleSearchGroupFlatten：canned torrent_table 含表头 + group 头 + 2 group_torrent 子 + 1 独立。
// 验证：group 扁平化（子行继承 group 标题）、表头列探测（size/seeders/completed 按 nth-child）、download 已 action=download。
func TestGazelleSearchGroupFlatten(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><table class="torrent_table">
		<tr class="colhead">
		  <td>Cat</td><td>Name</td>
		  <td><a href="torrents.php?order_by=size">Size</a></td>
		  <td><a href="torrents.php?order_by=seeders">S</a></td>
		  <td><a href="torrents.php?order_by=leechers">L</a></td>
		  <td><a href="torrents.php?order_by=snatched">C</a></td>
		</tr>
		<tr class="group">
		  <td><a href="filter_cat[1]=1"><img alt="Music"></a></td>
		  <td><a href="torrents.php?id=500">Group Title</a><div class="tags">tag1</div></td>
		  <td></td><td></td><td></td><td></td>
		</tr>
		<tr class="group_torrent groupid_500">
		  <td></td>
		  <td><a href="torrents.php?action=download&amp;id=10">DL</a></td>
		  <td>1.0 GB</td><td>5</td><td>2</td><td>40</td>
		</tr>
		<tr class="group_torrent groupid_500">
		  <td></td>
		  <td><a href="torrents.php?action=download&amp;id=11">DL</a></td>
		  <td>2.0 GB</td><td>3</td><td>1</td><td>20</td>
		</tr>
		<tr class="torrent">
		  <td><a href="filter_cat[2]=1"><img alt="App"></a></td>
		  <td><a href="torrents.php?id=600">Standalone</a><a href="torrents.php?action=download&amp;id=20">DL</a></td>
		  <td>500 MB</td><td>1</td><td>0</td><td>5</td>
		</tr>
		</table></body></html>`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelle(hc)
	def := &site.SiteMetadata{ID: "gz", Schema: "gazelle"} // 选择器由 Search 内 EnsureDefaults 注入

	ts, err := g.Search(context.Background(), def, srv.URL, "", "kw", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 3 {
		t.Fatalf("expected 3 torrents (2 group children + 1 standalone), got %d", len(ts))
	}
	// group 子行：继承 group 标题，download 已 action=download&id=
	if ts[0].Title != "Group Title" || ts[0].ID != "10" {
		t.Errorf("ts[0]: Title=%q ID=%q, want Group Title/10", ts[0].Title, ts[0].ID)
	}
	if ts[0].Seeders != 5 || ts[0].Completed != 40 || ts[0].Size != "1.0 GB" {
		t.Errorf("ts[0] columns: Seeders=%d Completed=%d Size=%q", ts[0].Seeders, ts[0].Completed, ts[0].Size)
	}
	if ts[1].Title != "Group Title" || ts[1].ID != "11" || ts[1].Seeders != 3 {
		t.Errorf("ts[1]: %+v", ts[1])
	}
	// 独立种子：自有标题/分类
	if ts[2].Title != "Standalone" || ts[2].ID != "20" || ts[2].Category != "App" {
		t.Errorf("ts[2]: Title=%q ID=%q Cat=%q", ts[2].Title, ts[2].ID, ts[2].Category)
	}
}

// TestGazelleUserInfoTwoStep：/ 发现 uid → /user.php?id=N 解析 ul.stats。
func TestGazelleUserInfoTwoStep(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><body><a class="username" href="user.php?id=7">u</a></body></html>`))
		case "/user.php":
			_, _ = w.Write([]byte(`<html><body>
			<a class="username" href="user.php?id=7">alice</a>
			<div>Stats</div><ul class="stats"><li>Uploaded: 2.0 TB</li><li>Ratio: 3.5</li></ul>
			<div>Personal</div><ul class="stats"><li>Class: VIP</li></ul>
			</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelle(hc)
	def := &site.SiteMetadata{ID: "gz", Schema: "gazelle"}

	ui, err := g.FetchUserInfo(context.Background(), def, srv.URL, "")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if ui.ID != "7" {
		t.Errorf("ID = %q, want 7", ui.ID)
	}
	if ui.Name != "alice" {
		t.Errorf("Name = %q, want alice", ui.Name)
	}
	if ui.Level != "VIP" {
		t.Errorf("Level = %q, want VIP", ui.Level)
	}
	if ui.Ratio != 3.5 {
		t.Errorf("Ratio = %v, want 3.5", ui.Ratio)
	}
	if ui.Uploaded <= 0 {
		t.Errorf("Uploaded = %d, want >0", ui.Uploaded)
	}
}

// TestGazelleLoggedOut：cookie 失效 → 302 跳 login.php → ErrNotLoggedIn。
func TestGazelleLoggedOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login.php", http.StatusFound)
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true, FollowRedirect: false})
	g := NewGazelle(hc)
	def := &site.SiteMetadata{ID: "gz", Schema: "gazelle"}

	_, err := g.FetchUserInfo(context.Background(), def, srv.URL, "")
	if err != site.ErrNotLoggedIn {
		t.Errorf("expected ErrNotLoggedIn, got %v", err)
	}
}
