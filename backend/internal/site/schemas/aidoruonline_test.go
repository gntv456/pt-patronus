package schemas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

func TestAidoruOnlineFetchUserInfoSeedingSizePages(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/forum/usercp.php":
			_, _ = w.Write([]byte(`<a href="/member.php?action=profile&uid=123">profile</a>`))
		case "/account.php":
			_, _ = w.Write([]byte(`
<html><head><title>Account</title></head><body>
<table>
  <tr><td class="prof-lbl">Username:</td><td>aidoru-user</td></tr>
  <tr><td class="prof-lbl">Joined:</td><td>2024-05-06 07:08:09</td></tr>
  <tr><td class="prof-lbl">Uploaded:</td><td>1.25 TiB</td></tr>
  <tr><td class="prof-lbl">Downloaded:</td><td>512 GiB</td></tr>
  <tr><td class="prof-lbl">User Class:</td><td>Sharer</td></tr>
  <tr><td class="prof-lbl">Ratio:</td><td>2.5</td></tr>
  <tr><td class="prof-lbl">Uploads:</td><td>55</td></tr>
</table>
<span id="seed-count">3</span>
<div id="seed-search-stat">Page 1 / 2</div>
</body></html>`))
		case "/get_account_peers.php":
			switch r.URL.Query().Get("p") {
			case "0":
				_, _ = w.Write([]byte(`<tr><td>A</td><td>x</td><td>x</td><td>1.50 GiB</td></tr><tr><td>B</td><td>x</td><td>x</td><td>512 MiB</td></tr>`))
			case "1":
				_, _ = w.Write([]byte(`<tr><td>C</td><td>x</td><td>x</td><td>2 GiB</td></tr>`))
			default:
				t.Fatalf("unexpected seed page p=%q", r.URL.Query().Get("p"))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	def, ok := reg.Definition("aidoruonline")
	if !ok {
		t.Fatal("aidoruonline definition missing")
	}
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGenericHTML(hc, "aidoruonline")
	u, err := g.FetchUserInfo(context.Background(), def, srv.URL, "cookie")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if u.ID != "123" || u.Name != "aidoru-user" || u.Level != "Sharer" {
		t.Fatalf("identity = id %q name %q level %q", u.ID, u.Name, u.Level)
	}
	if want := int64(1280 * (1 << 30)); u.Uploaded != want {
		t.Fatalf("uploaded = %d, want %d", u.Uploaded, want)
	}
	if want := int64(512 * (1 << 30)); u.Downloaded != want {
		t.Fatalf("downloaded = %d, want %d", u.Downloaded, want)
	}
	if u.Ratio != 2.5 || u.Seeding != 3 || u.Uploads != 55 || u.JoinDate != "2024-05-06" {
		t.Fatalf("stats = ratio %g seeding %d uploads %d join %q", u.Ratio, u.Seeding, u.Uploads, u.JoinDate)
	}
	if want := int64(4 * (1 << 30)); u.SeedingSize != want {
		t.Fatalf("seeding_size = %d, want %d", u.SeedingSize, want)
	}
}
