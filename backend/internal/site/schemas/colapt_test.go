// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package schemas

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// ── parseColapTCred：chain 打包 JSON / 裸 apiKey / 空 三种形态 ──

func TestParseColapTCredJSON(t *testing.T) {
	c := parseColapTCred(
		`{"api_key":"rcapi_abc","username":"gntv","password":"p@ss"}`)
	if c.APIKey != "rcapi_abc" || c.Username != "gntv" || c.Password != "p@ss" {
		t.Fatalf("unexpected cred: %+v", c)
	}
}

func TestParseColapTCredBareKey(t *testing.T) {
	// 裸字符串（老用户只配了 apiKey）→ 只拆出 apiKey，账密为空
	c := parseColapTCred("rcapi_abc")
	if c.APIKey != "rcapi_abc" || c.Username != "" || c.Password != "" {
		t.Fatalf("unexpected cred: %+v", c)
	}
}

func TestParseColapTCredEmpty(t *testing.T) {
	if c := parseColapTCred("  "); c.APIKey != "" {
		t.Fatalf("empty raw should give empty cred: %+v", c)
	}
}

// ── colaptWebBase：兼容面主机 → 原生面主机 ──

func TestColaptWebBase(t *testing.T) {
	cases := map[string]string{
		"https://api.colapt.com":    "https://www.colapt.com",
		"https://api.colapt.com/":   "https://www.colapt.com",
		"https://www.colapt.com":    "https://www.colapt.com",    // 已是原生面，原样
		"https://proxy.example.com": "https://proxy.example.com", // 自建反代不猜
		"not a url":                 "not a url",
	}
	for in, want := range cases {
		if got := colaptWebBase(in); got != want {
			t.Errorf("colaptWebBase(%q) = %q, want %q", in, got, want)
		}
	}
}

// ── 原生面公告/站免：httptest 假站点全链路（登录 → 会话缓存 → GET 解析）──

// mustClient 构造允许私网（httptest）的共享客户端。
func mustClient(t *testing.T) *httpclient.Client {
	t.Helper()
	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatalf("new httpclient: %v", err)
	}
	return hc
}

// sessOK 校验测试请求带了会话头（CSRF + cookie）。
func sessOK(r *http.Request) bool {
	return r.Header.Get("X-CSRF-Token") == "csrf456" &&
		strings.Contains(r.Header.Get("Cookie"), "reelcore_session=sess123")
}

// newColaptTestServer 起一个假 reelcore：POST /api/v1/sessions 签发 cookie+csrf，
// GET /api/v1/announcements、/api/v1/promotions 校验会话头后回 fixture。
func newColaptTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	login := func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["username"] != "gntv" || body["password"] != "p@ss" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"unauthorized"}}`))
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name: "reelcore_session", Value: "sess123", Path: "/api/v1",
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"csrf_token":"csrf456","status":"authenticated"}`))
	}
	ann := func(w http.ResponseWriter, r *http.Request) {
		if !sessOK(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"items":[
			{"id":13,"title":"公告A","body":"正文A",
			 "created_at":"2026-11-02T13:10:26Z",
			 "published_at":"2026-11-02T13:28:41Z",
			 "published":true,"severity":"info"},
			{"id":12,"title":"草稿","body":"不可见","published":false}
		]}`))
	}
	promo := func(w http.ResponseWriter, r *http.Request) {
		if !sessOK(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"execution_revision":12,"promotion":{
			"accounting_applied":false,
			"upload":{"num":1,"den":1},
			"download":{"num":0,"den":1},
			"ends_at":"2027-01-01T02:10:00Z"}}`))
	}
	mux.HandleFunc("POST /api/v1/sessions", login)
	mux.HandleFunc("GET /api/v1/announcements", ann)
	mux.HandleFunc("GET /api/v1/promotions", promo)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestColaptFetchAnnouncementNative(t *testing.T) {
	srv := newColaptTestServer(t)
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	nat := newColaptNative()
	cred := colapTCred{Username: "gntv", Password: "p@ss"}

	a, err := colaptFetchAnnouncement(context.Background(), nat, hc, srv.URL, cred)
	if err != nil {
		t.Fatalf("fetch announcement: %v", err)
	}
	if len(a.Items) != 1 { // 草稿（published=false）不收
		t.Fatalf("want 1 published item, got %d", len(a.Items))
	}
	if a.Items[0].Title != "公告A" || !strings.Contains(a.Items[0].Content, "正文A") {
		t.Fatalf("unexpected item: %+v", a.Items[0])
	}
	if a.Items[0].Date != "2026-11-02T13:28:41Z" { // 优先 published_at
		t.Fatalf("unexpected date: %s", a.Items[0].Date)
	}
}

func TestColaptFetchFreeleechNative(t *testing.T) {
	srv := newColaptTestServer(t)
	hc, _ := httpclient.New(httpclient.Options{AllowPrivate: true})
	nat := newColaptNative()
	cred := colapTCred{Username: "gntv", Password: "p@ss"}

	out, err := colaptFetchFreeleech(context.Background(), nat, hc, srv.URL, cred)
	if err != nil {
		t.Fatalf("fetch freeleech: %v", err)
	}
	if len(out) != 1 || out[0].Status != site.FreeleechActive {
		t.Fatalf("want active freeleech, got %+v", out)
	}
	if !strings.Contains(out[0].Content, "2027-01-01") {
		t.Fatalf("ends_at missing in content: %q", out[0].Content)
	}
}

func TestColaptFetchFreeleechNone(t *testing.T) {
	// promotion.download.num!=0（非 0 倍下载）→ 暂无站免
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/sessions",
		func(w http.ResponseWriter, _ *http.Request) {
			http.SetCookie(w, &http.Cookie{
				Name:  "reelcore_session",
				Value: "s",
				Path:  "/api/v1",
			})
			_, _ = w.Write([]byte(
				`{"csrf_token":"c","status":"authenticated"}`))
		})
	mux.HandleFunc("GET /api/v1/promotions",
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(
				`{"promotion":{"download":{"num":1,"den":1},` +
					`"upload":{"num":1,"den":1}}}`))
		})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, err := colaptFetchFreeleech(context.Background(),
		newColaptNative(), mustClient(t), srv.URL,
		colapTCred{Username: "gntv", Password: "p"})
	if err != nil {
		t.Fatalf("fetch freeleech: %v", err)
	}
	if len(out) != 1 || out[0].Status != site.FreeleechNone {
		t.Fatalf("want none freeleech, got %+v", out)
	}
}

func TestColaptNativeMissingPassword(t *testing.T) {
	// 只配 apiKey（账密空）→ 报可读指引而非空报错
	_, err := colaptFetchAnnouncement(context.Background(),
		newColaptNative(), mustClient(t),
		"https://www.colapt.com", colapTCred{APIKey: "rcapi_x"})
	if err == nil || !strings.Contains(err.Error(), "账号密码") {
		t.Fatalf("want readable guidance error, got %v", err)
	}
}

func TestColaptLoginBadCreds(t *testing.T) {
	srv := newColaptTestServer(t)
	nat := newColaptNative()
	_, err := nat.sessionFor(context.Background(),
		mustClient(t), srv.URL, "gntv", "wrong")
	if err == nil {
		t.Fatal("bad creds should fail")
	}
}
