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

func TestRousiSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 旧版站无 PeerGo torrents 端点 → 先探测 404 再走 legacy search
		if r.URL.Path == "/api/v1/torrents" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer pk" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/api/v1/search" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("keyword"); got != "matrix" {
			t.Fatalf("keyword = %q", got)
		}
		if got := r.URL.Query().Get("category"); got != "movie" {
			t.Fatalf("category = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"data": {
				"torrents": [{
					"uuid": "abc-123",
					"title": "The Matrix",
					"subtitle": "Remux",
					"created_at": "2026-07-05T12:34:56Z",
					"size": 1073741824,
					"seeders": 8,
					"leechers": 2,
					"downloads": 99,
					"category_name": "电影",
					"promotion": {"is_active": true, "type": 4}
				}]
			}
		}`))
	}))
	defer srv.Close()

	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewRousi(hc).Search(context.Background(), &site.SiteMetadata{}, srv.URL, "Bearer pk", "matrix", "movie", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	tor := got[0]
	if tor.ID != "abc-123" || tor.Title != "The Matrix" || tor.Seeders != 8 || tor.Completed != 99 {
		t.Fatalf("torrent = %+v", tor)
	}
	if !tor.Free || len(tor.Tags) != 1 || tor.Tags[0] != "2xFree" {
		t.Fatalf("promotion not parsed: free=%v tags=%v", tor.Free, tor.Tags)
	}
	if !strings.Contains(tor.DownloadURL, "/api/torrent/abc-123/download/pk") {
		t.Fatalf("download url = %s", tor.DownloadURL)
	}
}

func TestRousiFetchUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 旧版站无 PeerGo me 端点 → 404 回退 legacy
		if r.URL.Path == "/api/v1/me/profile" || r.URL.Path == "/api/v1/me/economy" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer pk" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/profile":
			if got := r.URL.Query().Get("include_fields[user]"); got != "seeding_leeching_data" {
				t.Fatalf("include_fields[user] = %q", got)
			}
			_, _ = w.Write([]byte(`{
				"code": 0,
				"data": {
					"id": 42,
					"username": "alice",
					"email": "a@example.test",
					"level": 5,
					"level_text": "Lv.5",
					"registered_at": "2024-01-02T03:04:05Z",
					"last_active_at": "2026-07-06T07:08:09Z",
					"uploaded": 2048,
					"downloaded": 1024,
					"ratio": 2.0,
					"karma": 1234.5,
					"seeding_time": 3661,
					"seeding_leeching_data": {
						"seeding_count": 7,
						"seeding_size": 4096,
						"leeching_count": 1
					}
				}
			}`))
		case "/api/v1/seeding-reward":
			_, _ = w.Write([]byte(`{"code":0,"data":{"total_reward":12.5}}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewRousi(hc).FetchUserInfo(context.Background(), &site.SiteMetadata{}, srv.URL, "pk")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "42" || got.Name != "alice" || got.Level != "Lv.5" || got.JoinDate != "2024-01-02 03:04:05" {
		t.Fatalf("identity = %+v", got)
	}
	if got.LevelID != 5 || got.LastAccessAt != "2026-07-06 07:08:09" {
		t.Fatalf("profile extras = level_id %d last_access_at %q", got.LevelID, got.LastAccessAt)
	}
	if got.Uploaded != 2048 || got.Downloaded != 1024 || got.Ratio != 2 ||
		got.Seeding != 7 || got.Leeching != 1 || got.SeedingSize != 4096 ||
		got.SeedingTime != 3661 || got.Bonus != 1234.5 || got.BonusPerHour != 12.5 {
		t.Fatalf("stats = %+v", got)
	}
}

func TestRousiSearchPeerGo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/torrents" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("query"); got != "matrix" {
			t.Fatalf("query = %q", got)
		}
		if got := r.URL.Query().Get("category"); got != "movies" {
			t.Fatalf("category = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[
			{"id":8448,"name":"Monogatari S05","subtitle":"物语系列","size_bytes":1073741824,
			 "seeders":2,"leechers":4,"completed":2,"uploaded_at":"2026-07-05T12:34:56Z",
			 "promotion":"double_upload_free","category":{"id":"anime","name":"动漫"}}
		],"limit":100,"offset":0,"total":1}`))
	}))
	defer srv.Close()

	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewRousi(hc).Search(context.Background(), &site.SiteMetadata{}, srv.URL, "", "matrix", "movie", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	tor := got[0]
	if tor.ID != "8448" || tor.Title != "Monogatari S05" || tor.Seeders != 2 || tor.Completed != 2 {
		t.Fatalf("torrent = %+v", tor)
	}
	if !tor.Free || tor.DownloadVolumeFactor != 0 || tor.UploadVolumeFactor != 2 {
		t.Fatalf("promotion not parsed: %+v", tor)
	}
	if tor.Category != "动漫" || tor.Size == "" {
		t.Fatalf("category/size = %q/%q", tor.Category, tor.Size)
	}
}

func TestRousiFetchUserInfoPeerGo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/me/profile":
			_, _ = w.Write([]byte(`{"username":"gntv","registered_at":"2026-01-02T03:04:05Z","level":{"id":3,"name":"Power User"}}`))
		case "/api/v1/me/economy":
			_, _ = w.Write([]byte(`{"uploaded":"2048","downloaded":"1024","ratio":2.0,"bonus":8888.5,"seed_points":12.3}`))
		default:
			t.Fatalf("path = %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := NewRousi(hc).FetchUserInfo(context.Background(), &site.SiteMetadata{}, srv.URL, "session=abc")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "gntv" || got.Level != "Power User" || got.JoinDate != "2026-01-02 03:04:05" {
		t.Fatalf("user = %+v", got)
	}
	if got.Uploaded != 2048 || got.Downloaded != 1024 || got.Ratio != 2 || got.Bonus != 8888.5 {
		t.Fatalf("economy = %+v", got)
	}
}
