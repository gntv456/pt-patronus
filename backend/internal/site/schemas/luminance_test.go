package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// TestLuminanceSearch：单层 #torrent_table（表头 + 1 行）→ 验列探测 + 字段抽取。
func TestLuminanceSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><table id="torrent_table">
		<tr>
		  <td>Cat</td><td>Name</td>
		  <td><a href="torrents.php?order_by=size">Size</a></td>
		  <td><a href="torrents.php?order_by=seeders">S</a></td>
		  <td><a href="torrents.php?order_by=leechers">L</a></td>
		  <td><a href="torrents.php?order_by=snatched">C</a></td>
		</tr>
		<tr>
		  <td class="cats_col"><div title="Movie"><a href="torrents.php?cat=1">cat</a></div></td>
		  <td><a href="torrents.php?id=100">My Torrent</a>
		      <a href="torrents.php?action=download&amp;id=100&amp;authkey=k">dl</a>
		      <span class="time" title="2024-01-01">t</span></td>
		  <td>1.5 GB</td><td>5</td><td>2</td><td>40</td>
		</tr>
		</table></body></html>`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	l := NewLuminance(hc)
	def := &site.SiteMetadata{ID: "lm", Schema: "luminance"}

	ts, err := l.Search(context.Background(), def, srv.URL, "", "kw", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 {
		t.Fatalf("expected 1 torrent, got %d", len(ts))
	}
	if ts[0].Title != "My Torrent" {
		t.Errorf("Title = %q", ts[0].Title)
	}
	if ts[0].Seeders != 5 || ts[0].Completed != 40 || ts[0].Size != "1.5 GB" {
		t.Errorf("columns: Seeders=%d Completed=%d Size=%q", ts[0].Seeders, ts[0].Completed, ts[0].Size)
	}
	if ts[0].Category == "" {
		t.Errorf("Category should be populated from cats_col")
	}
	if !strings.Contains(ts[0].DownloadURL, "action=download") {
		t.Errorf("DownloadURL = %q", ts[0].DownloadURL)
	}
}

// TestLuminanceUserInfo：/ 发现 uid → /user.php?id=N 解析 ul.stats。
func TestLuminanceUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><body><a class="username" href="user.php?id=7">u</a></body></html>`))
		case "/user.php":
			_, _ = w.Write([]byte(`<html><body>
			<a class="username" href="user.php?id=7">alice</a>
			<ul class="stats">
			  <li>Uploaded: 2.0 TB</li>
			  <li>Ratio: <span>3.5</span></li>
			  <li>Class: VIP</li>
			</ul>
			</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	l := NewLuminance(hc)
	def := &site.SiteMetadata{ID: "lm", Schema: "luminance"}

	ui, err := l.FetchUserInfo(context.Background(), def, srv.URL, "")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if ui.ID != "7" || ui.Name != "alice" {
		t.Errorf("ID/Name = %q/%q, want 7/alice", ui.ID, ui.Name)
	}
	if ui.Ratio != 3.5 {
		t.Errorf("Ratio = %v, want 3.5", ui.Ratio)
	}
	if ui.Uploaded <= 0 {
		t.Errorf("Uploaded = %d, want >0", ui.Uploaded)
	}
}

// TestLuminanceLoggedOut：cookie 失效 → 302 跳 login.php → ErrNotLoggedIn。
func TestLuminanceLoggedOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login.php", http.StatusFound)
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true, FollowRedirect: false})
	l := NewLuminance(hc)
	def := &site.SiteMetadata{ID: "lm", Schema: "luminance"}

	_, err := l.FetchUserInfo(context.Background(), def, srv.URL, "")
	if err != site.ErrNotLoggedIn {
		t.Errorf("expected ErrNotLoggedIn, got %v", err)
	}
}
