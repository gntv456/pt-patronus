package schemas

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// TestFetchHRParsesList：GET H&R 页 → 解析出种子列表。
func TestFetchHRParsesList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/getusertorrentlist.php" || r.URL.Query().Get("type") != "hr" {
			t.Errorf("unexpected request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><table class="torrents">
		<tr><th>标题</th></tr>
		<tr>
		  <td><a href="details.php?id=7">HR Torrent</a></td>
		  <td>2 GB</td><td>0.8</td><td>5:00:00</td><td>监控中</td><td>3:00:00</td>
		</tr>
		</table></body></html>`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	n := NewNexusPHP(hc)
	def := &site.SiteMetadata{
		NoLogin: site.NoLoginDef{URLPattern: []string{"login.php"}},
		HR: site.HRDef{
			Enabled: true,
			Rows:    site.ElementQuery{Selector: "table.torrents tr"},
			Fields: map[string]site.ElementQuery{
				"title":    {Selector: "a"},
				"size":     {Selector: "td:nth-child(2)"},
				"ratio":    {Selector: "td:nth-child(3)"},
				"seedtime": {Selector: "td:nth-child(4)"},
				"status":   {Selector: "td:nth-child(5)"},
				"deadline": {Selector: "td:nth-child(6)"},
			},
		},
	}
	// EnsureDefaults 会补 HR.Path；这里显式覆盖指向测试服
	def.HR.Path = "getusertorrentlist.php?type=hr"

	list, err := n.FetchHR(context.Background(), def, srv.URL, "sess=abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 HR torrent, got %d", len(list))
	}
	if list[0].Title != "HR Torrent" || list[0].Status != "监控中" {
		t.Errorf("unexpected: %+v", list[0])
	}
}

// TestFetchHRDetectsLoggedOut：重定向到登录页 → ErrNotLoggedIn。
func TestFetchHRDetectsLoggedOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login.php", http.StatusFound)
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true, FollowRedirect: false})
	n := NewNexusPHP(hc)
	def := &site.SiteMetadata{
		NoLogin: site.NoLoginDef{URLPattern: []string{"login.php"}},
		HR:      site.HRDef{Enabled: true, Path: "getusertorrentlist.php?type=hr"},
	}
	_, err := n.FetchHR(context.Background(), def, srv.URL, "sess=abc")
	if !errors.Is(err, site.ErrNotLoggedIn) {
		t.Errorf("err = %v, want ErrNotLoggedIn", err)
	}
}
