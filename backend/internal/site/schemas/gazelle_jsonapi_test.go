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

// gazelleAJAXHandler 按 action 参数返回 Gazelle ajax.php JSON。
func gazelleAJAXHandler(t *testing.T) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("action") {
		case "browse":
			_, _ = w.Write([]byte(`{"status":"success","response":{"results":[
			  {"groupId":1,"groupName":"Album","artist":"Bob","groupYear":2020,"releaseType":"Album","torrents":[
			    {"torrentId":10,"size":16106127360,"seeders":5,"leechers":2,"snatches":40,"isFreeleech":true},
			    {"torrentId":11,"size":8589934592,"seeders":3,"leechers":1,"snatches":20,"isFreeleech":false}
			  ]},
			  {"groupId":2,"groupName":"Single","torrentId":20,"size":1073741824,"seeders":1,"leechers":0,"snatches":5,"category":"music"}
			]}}`))
		case "index":
			_, _ = w.Write([]byte(`{"status":"success","response":{"username":"u","id":7,"authkey":"AK","passkey":"PK",
			  "userstats":{"uploaded":1000000000000,"downloaded":500000000000,"ratio":2,"class":"VIP","bonusPoints":100,"bonusPointsPerHour":5}}}`))
		case "user":
			_, _ = w.Write([]byte(`{"status":"success","response":{"stats":{"joinedDate":"2020-01-01"},
			  "community":{"seeding":8,"uploaded":3}}}`))
		default:
			http.NotFound(w, r)
		}
	})
}

// TestGazelleJSONAPISearch：group(2 子) + flat → 3 torrent；Free 标记；下载链含 authkey/torrent_pass。
func TestGazelleJSONAPISearch(t *testing.T) {
	srv := httptest.NewServer(gazelleAJAXHandler(t))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelleJSONAPI(hc)
	def := &site.SiteMetadata{ID: "red", Schema: "gazelle_jsonapi"}

	ts, err := g.Search(context.Background(), def, srv.URL, "", "kw", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 3 {
		t.Fatalf("expected 3 torrents (2 group + 1 flat), got %d", len(ts))
	}
	if !strings.Contains(ts[0].Title, "Album") || !strings.Contains(ts[0].Title, "Bob") {
		t.Errorf("ts[0].Title = %q, want group title 'Bob - Album ...'", ts[0].Title)
	}
	if !ts[0].Free {
		t.Errorf("ts[0] should be Free (isFreeleech)")
	}
	if ts[0].ID != "10" || ts[0].Seeders != 5 {
		t.Errorf("ts[0]: ID=%q Seeders=%d", ts[0].ID, ts[0].Seeders)
	}
	dl := ts[0].DownloadURL
	if !strings.Contains(dl, "id=10") || !strings.Contains(dl, "authkey=AK") || !strings.Contains(dl, "torrent_pass=PK") {
		t.Errorf("ts[0].DownloadURL = %q, want id=10 + authkey + torrent_pass", dl)
	}
	// flat torrent
	if ts[2].ID != "20" || ts[2].Title != "Single" {
		t.Errorf("ts[2] (flat): ID=%q Title=%q", ts[2].ID, ts[2].Title)
	}
}

// TestGazelleJSONAPIUserInfo：action=index（基础）+ action=user（扩展）。
func TestGazelleJSONAPIUserInfo(t *testing.T) {
	srv := httptest.NewServer(gazelleAJAXHandler(t))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelleJSONAPI(hc)
	def := &site.SiteMetadata{ID: "red", Schema: "gazelle_jsonapi"}

	ui, err := g.FetchUserInfo(context.Background(), def, srv.URL, "")
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if ui.Name != "u" || ui.Level != "VIP" {
		t.Errorf("Name/Level = %q/%q", ui.Name, ui.Level)
	}
	if ui.Uploaded != 1000000000000 {
		t.Errorf("Uploaded = %d", ui.Uploaded)
	}
	if ui.Bonus != 100 || ui.BonusPerHour != 5 {
		t.Errorf("Bonus/BonusPerHour = %v/%v", ui.Bonus, ui.BonusPerHour)
	}
	if !strings.Contains(ui.JoinDate, "2020") {
		t.Errorf("JoinDate = %q", ui.JoinDate)
	}
	if ui.Seeding != 8 || ui.Uploads != 3 {
		t.Errorf("Seeding/Uploads = %d/%d", ui.Seeding, ui.Uploads)
	}
}

// TestGazelleJSONAPIAuthFailure：status:failure + 鉴权类 error → ErrNotLoggedIn（改进 PT-depiler 静默返空）。
func TestGazelleJSONAPIAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"failure","error":"bad credentials"}`))
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	g := NewGazelleJSONAPI(hc)
	def := &site.SiteMetadata{ID: "red", Schema: "gazelle_jsonapi"}

	_, err := g.Search(context.Background(), def, srv.URL, "", "kw", "", 0)
	if err != site.ErrNotLoggedIn {
		t.Errorf("expected ErrNotLoggedIn, got %v", err)
	}
}
