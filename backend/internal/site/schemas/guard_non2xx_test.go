// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

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

// 站点故障（5xx / CF 盾）被解析成「空数据」的同族：这些空数据在业务上等于「没问题」。
// H&R 空列表 = 无封号风险；搜索空结果 = 该站没片；保活成功 = cookie 有效。
// 三者的共同前提是「页面真的读到了」，所以非 2xx 必须先报错。

func serverError5xx() string {
	return `<html><head><title>502 Bad Gateway</title></head><body><h1>nginx</h1></body></html>`
}

// TestFetchHRRejectsServerErrorPage H&R 页 5xx 必须报错。
// 回归前只判 CF：502 错误页被解析成「一行 H&R 都没有」，考核与告警把它当「无风险」。
func TestFetchHRRejectsServerErrorPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(serverError5xx()))
	}))
	defer srv.Close()

	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	def := siblingDef(t, "1ptba")
	list, err := NewNexusPHP(hc).FetchHR(context.Background(), def, srv.URL, "sess=abc")
	if err == nil {
		t.Fatalf("5xx 必须报错，got %d 条 H&R（=无风险）", len(list))
	}
	if !strings.Contains(err.Error(), "HTTP 502") {
		t.Errorf("错误应带状态码，got %v", err)
	}
	if list != nil {
		t.Errorf("报错时不应返回任何条目，got %+v", list)
	}
}

// TestKeepAliveRejectsServerErrorPage 保活页 5xx 必须报错，而不是报「还在登录态」。
func TestKeepAliveRejectsServerErrorPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(serverError5xx()))
	}))
	defer srv.Close()

	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	res, err := NewNexusPHP(hc).KeepAlive(context.Background(), siblingDef(t, "1ptba"), srv.URL, "sess=abc")
	if err == nil {
		t.Fatalf("5xx 必须报错，got LoggedIn=%v（=cookie 有效）", res.LoggedIn)
	}
	if res != nil {
		t.Errorf("报错时不应返回保活结果，got %+v", res)
	}
}

// TestSearchRejectsServerErrorPage 三个 HTML 搜索入口（NexusPHP/Gazelle/Luminance）
// 对 5xx 都要报错。回归前它们只判 CF，故障站在跨站搜索里被渲染成「该站无匹配」。
func TestSearchRejectsServerErrorPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = w.Write([]byte(serverError5xx()))
	}))
	defer srv.Close()

	hc, err := httpclient.New(httpclient.Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		schema string
		id     string
		search func(def *site.SiteMetadata) ([]site.Torrent, error)
	}{
		{"nexusphp", "1ptba", func(def *site.SiteMetadata) ([]site.Torrent, error) {
			return NewNexusPHP(hc).Search(context.Background(), def, srv.URL, "sess=abc", "test", "", 1)
		}},
		{"gazelle", "passthepopcorn", func(def *site.SiteMetadata) ([]site.Torrent, error) {
			return NewGazelle(hc).Search(context.Background(), def, srv.URL, "sess=abc", "test", "", 1)
		}},
		{"luminance", "cgpeers", func(def *site.SiteMetadata) ([]site.Torrent, error) {
			return NewLuminance(hc).Search(context.Background(), def, srv.URL, "sess=abc", "test", "", 1)
		}},
	}
	for _, tc := range cases {
		def := siblingDef(t, tc.id)
		got, err := tc.search(def)
		if err == nil {
			t.Errorf("%s: 5xx 必须报错，got %d 条结果（=该站无匹配）", tc.schema, len(got))
			continue
		}
		if !strings.Contains(err.Error(), "HTTP 504") {
			t.Errorf("%s: 错误应带状态码，got %v", tc.schema, err)
		}
	}
}
