package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// TestUnit3DSearch：canned /torrents 页（1 真 torrent 行 + 1 表头行）→ 验行过滤（丢表头）、
// 字段抽取、download 链接 /download_check/ → /download/ 重写、EnsureDefaults 注入的默认选择器。
func TestUnit3DSearch(t *testing.T) {
	var gotName, gotPerPage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotName = r.URL.Query().Get("name")
		gotPerPage = r.URL.Query().Get("perPage")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>
		<div class="torrent-search--list__results"><table><tbody>
		<tr><th>名称</th><th>大小</th><th>做种</th><th>下载</th></tr>
		<tr>
		  <td><a class="view-torrent" href="/torrents/123">My Movie 2024</a></td>
		  <td class="torrent-search--list__size">2.5 GB</td>
		  <td class="torrent-search--list__seeders">7</td>
		  <td class="torrent-search--list__leechers">2</td>
		  <td class="torrent-search--list__completed">40</td>
		  <td><a href="/torrents/download_check/123">dl</a></td>
		</tr>
		</tbody></table></div>
		</body></html>`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u := NewUnit3D(hc)
	def := &site.SiteMetadata{ID: "u3d", Schema: "unit3d"} // 选择器由 Search 内 EnsureDefaults 注入

	ts, err := u.Search(context.Background(), def, srv.URL, "", "hello", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if gotName != "hello" {
		t.Errorf("backend received name param = %q, want hello", gotName)
	}
	if gotPerPage != "100" {
		t.Errorf("perPage = %q, want 100", gotPerPage)
	}
	if len(ts) != 1 {
		t.Fatalf("expected 1 torrent (header row filtered), got %d", len(ts))
	}
	if ts[0].Title != "My Movie 2024" {
		t.Errorf("Title = %q", ts[0].Title)
	}
	if ts[0].Seeders != 7 || ts[0].Completed != 40 {
		t.Errorf("Seeders/Completed = %d/%d", ts[0].Seeders, ts[0].Completed)
	}
	if ts[0].DetailURL != srv.URL+"/torrents/123" {
		t.Errorf("DetailURL = %q", ts[0].DetailURL)
	}
	// /download_check/ → /download/ 重写
	if ts[0].DownloadURL != srv.URL+"/torrents/download/123" {
		t.Errorf("DownloadURL = %q (want download_check rewritten to download)", ts[0].DownloadURL)
	}
}

// TestUnit3DFetchUserInfo：三步流程。server 按路径切换：
//   - /  → 顶部 settings 链接（发现用户名 bob）
//   - /users/bob → ratio-bar uploaded + profile__registration
//   - /users/bob/earnings → .panelV2 dl.key-value 第 3 个 dd（bonusPerHour）
func TestUnit3DFetchUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><body><a href="/users/bob/settings">Bob</a></body></html>`))
		case "/users/bob":
			_, _ = w.Write([]byte(`<html><body>
			<ul class="ratio-bar">
			  <li class="ratio-bar__uploaded"><a href="/users/bob/uploads"><i class="fa fa-arrow-up"></i> 10 GB</a></li>
			</ul>
			<time class="profile__registration" datetime="2022-03-04">2022</time>
			</body></html>`))
		case "/users/bob/earnings":
			_, _ = w.Write([]byte(`<html><body><div class="panelV2"><dl class="key-value">
			  <dd>seedbonus</dd><dd>total</dd><dd>123.45</dd>
			</dl></div></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	u := NewUnit3D(hc)
	def := &site.SiteMetadata{ID: "u3d", Schema: "unit3d"}

	ui, err := u.FetchUserInfo(context.Background(), def, srv.URL, "")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if ui.Name != "bob" {
		t.Errorf("Name = %q, want bob", ui.Name)
	}
	if ui.Uploaded != 10*1024*1024*1024 {
		t.Errorf("Uploaded = %d, want 10 GiB", ui.Uploaded)
	}
	if ui.BonusPerHour != 123.45 {
		t.Errorf("BonusPerHour = %v, want 123.45", ui.BonusPerHour)
	}
}

// TestUnit3DDetectsLoggedOut：cookie 失效 → 302 跳 /login → ErrNotLoggedIn。
func TestUnit3DDetectsLoggedOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login", http.StatusFound)
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true, FollowRedirect: false})
	u := NewUnit3D(hc)
	def := &site.SiteMetadata{ID: "u3d", Schema: "unit3d"}

	_, err := u.FetchUserInfo(context.Background(), def, srv.URL, "")
	if err != site.ErrNotLoggedIn {
		t.Errorf("expected ErrNotLoggedIn, got %v", err)
	}
}
