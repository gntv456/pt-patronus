package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

func TestNexusPHPSearchEndToEnd(t *testing.T) {
	var gotParam string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotParam = r.URL.Query().Get("search")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><table class="torrents">
		<tr><th>x</th></tr>
		<tr>
		  <td><a href="details.php?id=1">The Movie 2024</a></td>
		  <td><a href="download.php?id=1&amp;passkey=k">dl</a></td>
		  <td>1.5 GB</td><td>5</td><td>2</td><td>40</td>
		</tr>
		</table></body></html>`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	n := NewNexusPHP(hc)
	def := &site.SiteMetadata{
		SearchEntry: "torrents.php",
		SearchParam: "search",
		Selectors: site.SiteSelectors{
			Rows: site.ElementQuery{Selector: "table.torrents tr"},
			Fields: map[string]site.ElementQuery{
				"title":     {Selector: "td:nth-child(1) a"},
				"detail":    {Selector: "td:nth-child(1) a", Attr: "href"},
				"download":  {Selector: "td:nth-child(2) a", Attr: "href"},
				"size":      {Selector: "td:nth-child(3)"},
				"seeders":   {Selector: "td:nth-child(4)"},
				"leechers":  {Selector: "td:nth-child(5)"},
				"completed": {Selector: "td:nth-child(6)"},
			},
		},
	}

	ts, err := n.Search(context.Background(), def, srv.URL, "sess=abc", "hello", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if gotParam != "hello" {
		t.Errorf("backend received search param = %q", gotParam)
	}
	if len(ts) != 1 {
		t.Fatalf("expected 1 torrent, got %d", len(ts))
	}
	if ts[0].Title != "The Movie 2024" || ts[0].Seeders != 5 || ts[0].Completed != 40 {
		t.Errorf("unexpected torrent: %+v", ts[0])
	}
	if ts[0].DownloadURL != srv.URL+"/download.php?id=1&passkey=k" {
		t.Errorf("download url = %q", ts[0].DownloadURL)
	}
}

func TestNexusPHPDetectsLoggedOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/login.php", http.StatusFound)
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true, FollowRedirect: false})
	n := NewNexusPHP(hc)
	def := &site.SiteMetadata{
		NoLogin: site.NoLoginDef{URLPattern: []string{"login.php"}},
		Selectors: site.SiteSelectors{
			Rows: site.ElementQuery{Selector: "table tr"},
		},
	}
	_, err := n.Search(context.Background(), def, srv.URL, "", "kw", "", 0)
	if err != site.ErrNotLoggedIn {
		t.Errorf("expected ErrNotLoggedIn, got %v", err)
	}
}
