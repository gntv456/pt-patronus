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
