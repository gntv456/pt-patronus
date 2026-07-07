package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

func TestGenericHTMLFileListDefinitionSearchAndUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/browse.php":
			_, _ = w.Write([]byte(`<html><body>
<div class="torrentrow">
  <div>
    <img alt="Filme HD">
    <a href="details.php?id=123" title="FileList Movie"><b>FileList Movie</b></a>
    <a href="download.php?id=123"><img src="/download.png"></a>
  </div>
  <div>c2</div><div>c3</div><div>c4</div><div>c5</div>
  <div>02-01-2024 03:04:05</div>
  <div>2.00 GiB</div>
  <div>21</div>
  <div>8</div>
  <div>1</div>
</div>
</body></html>`))
		case "/index.php":
			_, _ = w.Write([]byte(`<html><body>
<div class="statusbar">
  <a href="userdetails.php?id=42"><span>alice</span></a>
  <span><img src="/uploaded.png"> 2.00 TiB</span>
  <span><img src="/downloaded.png"> 512.00 GiB</span>
  <a href="shop.php">12,345.6 bonus</a>
</div>
</body></html>`))
		case "/userdetails.php":
			if r.URL.Query().Get("id") != "42" {
				t.Fatalf("id = %q, want 42", r.URL.Query().Get("id"))
			}
			_, _ = w.Write([]byte(`<html><body><table>
<tr><td>Class</td><td>Elite</td></tr>
<tr><td>Join date</td><td>2024-01-02 03:04:05</td></tr>
<tr><td>Last seen</td><td>2024-01-03 04:05:06 (1 day ago)</td></tr>
<tr><td>Seed bonus</td><td>
  <div>Seeding 7 torrents with a total seed size of 3.00 TiB.</div>
  <div>ignored</div>
  <div>12.5 points per hour</div>
</td></tr>
</table></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("filelist")
	if !ok {
		t.Fatal("filelist definition missing")
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericHTML(hc, "filelist")

	ts, err := g.Search(context.Background(), def, srv.URL, "cookie", "movie", "", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(ts) != 1 {
		t.Fatalf("Search returned %d torrents, want 1", len(ts))
	}
	if ts[0].Title != "FileList Movie" || ts[0].Category != "Filme HD" {
		t.Fatalf("torrent title/category = %q/%q", ts[0].Title, ts[0].Category)
	}
	if ts[0].DetailURL != srv.URL+"/details.php?id=123" || ts[0].DownloadURL != srv.URL+"/download.php?id=123" {
		t.Fatalf("torrent urls = detail %q download %q", ts[0].DetailURL, ts[0].DownloadURL)
	}
	if ts[0].Seeders != 8 || ts[0].Leechers != 1 || ts[0].Completed != 21 {
		t.Fatalf("torrent stats = seeders %d leechers %d completed %d", ts[0].Seeders, ts[0].Leechers, ts[0].Completed)
	}

	u, err := g.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "42" || u.Name != "alice" || u.Level != "Elite" || u.JoinDate != "2024-01-02" {
		t.Fatalf("identity = id %q name %q level %q join %q", u.ID, u.Name, u.Level, u.JoinDate)
	}
	if u.LastAccessAt != "2024-01-03" {
		t.Fatalf("last_access_at = %q, want 2024-01-03", u.LastAccessAt)
	}
	if u.Uploaded != 2*(1<<40) || u.Downloaded != 512*(1<<30) {
		t.Fatalf("traffic = uploaded %d downloaded %d", u.Uploaded, u.Downloaded)
	}
	if u.Bonus != 12345.6 || u.Seeding != 7 || u.SeedingSize != 3*(1<<40) || u.BonusPerHour != 12.5 {
		t.Fatalf("stats = bonus %g seeding %d seeding_size %d bph %g", u.Bonus, u.Seeding, u.SeedingSize, u.BonusPerHour)
	}
}
