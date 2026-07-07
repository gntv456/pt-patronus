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

// TestAvistazSearchWithReauth：首次 torrents 401 → 失效缓存+重 auth+重试 → 解析 data[]。
func TestAvistazSearchWithReauth(t *testing.T) {
	var authCalls, torrentCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/jackett/auth":
			authCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"JWT1","expiry":3600}`))
		case "/api/v1/jackett/torrents":
			torrentCalls++
			if r.Header.Get("Authorization") != "Bearer JWT1" {
				t.Errorf("缺 Bearer 头")
			}
			if torrentCalls == 1 {
				w.WriteHeader(401) // 触发重 auth 重试
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":1,"file_name":"Movie","file_size":16106127360,
				"url":"/torrent/1","download":"/download/1","category":{"1":"Movie"},
				"created_at":"2024-01-01","seed":5,"leech":2,"completed":40}]}`))
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	a := NewAvistaz(hc)
	cred := `{"username":"u","password":"p","pid":"PID"}`
	ts, err := a.Search(context.Background(), &site.SiteMetadata{Schema: "avistaz"}, srv.URL, cred, "kw", "", 1)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if torrentCalls != 2 {
		t.Errorf("torrents 应被调 2 次（首次401重试），got %d", torrentCalls)
	}
	if authCalls != 2 {
		t.Errorf("auth 应被调 2 次（重试），got %d", authCalls)
	}
	if len(ts) != 1 {
		t.Fatalf("len=%d", len(ts))
	}
	if ts[0].Title != "Movie" || ts[0].Seeders != 5 || ts[0].Category != "Movie" {
		t.Errorf("解析错: %+v", ts[0])
	}
	if ts[0].DownloadURL != srv.URL+"/download/1" {
		t.Errorf("download=%q", ts[0].DownloadURL)
	}
}

// TestAvistazUserInfoStub：只返 {Name}，绝不访问 /profile（站规禁用户数据）。
func TestAvistazUserInfoStub(t *testing.T) {
	var profileHit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "profile") {
			profileHit = true
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	a := NewAvistaz(hc)
	cred := `{"username":"alice","password":"p","pid":"PID"}`
	ui, err := a.FetchUserInfo(context.Background(), &site.SiteMetadata{Schema: "avistaz"}, srv.URL, cred)
	if err != nil {
		t.Errorf("不应报错: %v", err)
	}
	if ui.Name != "alice" {
		t.Errorf("Name=%q want alice", ui.Name)
	}
	if profileHit {
		t.Errorf("不应访问 /profile（站规禁用户数据）")
	}
}

func TestAvistazDefinitionsSearchAndUserInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/jackett/auth":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"JWT1","expiry":3600}`))
		case "/api/v1/jackett/torrents":
			if r.Header.Get("Authorization") != "Bearer JWT1" {
				t.Errorf("Authorization=%q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":1,"file_name":"Network Movie","file_size":16106127360,
				"url":"/torrent/1","download":"/download/1","category":{"1":"Movie"},
				"created_at":"2024-01-01","seed":5,"leech":2,"completed":40}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	reg := site.NewRegistry()
	if err := reg.LoadDefinitions("../../../definitions"); err != nil {
		t.Fatalf("load definitions: %v", err)
	}
	cred := `{"username":"alice","password":"p","pid":"PID"}`
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	for _, id := range []string{"avistaz", "cinemaz", "exoticaz", "privatehd"} {
		t.Run(id, func(t *testing.T) {
			def, ok := reg.Definition(id)
			if !ok {
				t.Fatalf("%s definition missing", id)
			}
			a := NewAvistaz(hc)
			ts, err := a.Search(context.Background(), def, srv.URL, cred, "kw", "", 1)
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if len(ts) != 1 || ts[0].Title != "Network Movie" || ts[0].Seeders != 5 || ts[0].DownloadURL != srv.URL+"/download/1" {
				t.Fatalf("torrents = %+v", ts)
			}
			ui, err := a.FetchUserInfo(context.Background(), def, srv.URL, cred)
			if err != nil {
				t.Fatalf("FetchUserInfo: %v", err)
			}
			if ui.Name != "alice" || ui.Uploaded != 0 || ui.SeedingSize != 0 {
				t.Fatalf("user info = %+v", ui)
			}
		})
	}
}

func TestAnimeZFetchUserInfoWithCookie(t *testing.T) {
	var profileHit, activeHit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Cookie"), "sid=ok") {
			t.Errorf("Cookie=%q", r.Header.Get("Cookie"))
		}
		switch r.URL.Path {
		case "/profile/alice":
			profileHit = true
			_, _ = w.Write([]byte(`<html><head><title>Alice - AnimeZ</title></head><body>
<main><div><div><div><div><div><div><div>
<div>Alice</div><div><span class="badge badge-sm">Power User</span></div>
</div></div></div></div></div></div></div></main>
<div class="datagrid-item"><div class="datagrid-title">Uploaded</div><div class="datagrid-content">1.5 TiB</div></div>
<div class="datagrid-item"><div class="datagrid-title">Downloaded</div><div class="datagrid-content">512 GiB</div></div>
<div class="datagrid-item"><div class="datagrid-title">Ratio</div><div class="datagrid-content">3.0</div></div>
<div class="datagrid-item"><div class="datagrid-title">Bonus Points</div><div class="datagrid-content">12,345</div></div>
<div class="datagrid-item"><div class="datagrid-title">Uploads</div><div class="datagrid-content">42</div></div>
<div class="datagrid-item"><div class="datagrid-title">Seeding</div><div class="datagrid-content">2</div></div>
<div class="datagrid-item"><div class="datagrid-title">Leeching</div><div class="datagrid-content">1</div></div>
<div class="datagrid-item"><div class="datagrid-title">Member Since</div><div class="datagrid-content">January 02, 2024</div></div>
</body></html>`))
		case "/profile/alice/active":
			activeHit = true
			_, _ = w.Write([]byte(`<table>
<tr><td>Seeding</td><td><span class="text-yellow">10 GiB</span></td></tr>
<tr><td>Leeching</td><td><span class="text-yellow">99 GiB</span></td></tr>
</table>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	ui, err := NewAvistaz(hc).FetchUserInfo(context.Background(),
		&site.SiteMetadata{ID: "animez", Schema: "avistaz"}, srv.URL,
		`{"username":"alice","password":"p","pid":"PID","cookie":"sid=ok"}`)
	if err != nil {
		t.Fatalf("FetchUserInfo: %v", err)
	}
	if !profileHit || !activeHit {
		t.Fatalf("profileHit=%v activeHit=%v", profileHit, activeHit)
	}
	if ui.Name != "Alice" || ui.Level != "Power User" || ui.Uploaded == 0 || ui.Downloaded == 0 ||
		ui.Ratio != 3 || ui.Bonus != 12345 || ui.Uploads != 42 || ui.Seeding != 2 ||
		ui.Leeching != 1 || ui.JoinDate != "2024-01-02" || ui.SeedingSize != 10*1024*1024*1024 {
		t.Fatalf("ui = %+v", ui)
	}
}

// TestAvistazEmpty：torrents 400 → 空结果不报错。
func TestAvistazEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/jackett/auth":
			_, _ = w.Write([]byte(`{"token":"JWT1","expiry":3600}`))
		case "/api/v1/jackett/torrents":
			w.WriteHeader(400) // 空结果
		}
	}))
	defer srv.Close()

	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	a := NewAvistaz(hc)
	ts, err := a.Search(context.Background(), &site.SiteMetadata{Schema: "avistaz"}, srv.URL,
		`{"username":"u","password":"p","pid":"x"}`, "kw", "", 1)
	if err != nil {
		t.Errorf("400 应为空结果不报错，got %v", err)
	}
	if len(ts) != 0 {
		t.Errorf("应空，got %d", len(ts))
	}
}
