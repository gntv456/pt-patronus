// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestMediaImageReferer：白名单匹配 + Referer 注入 + SSRF 拒绝。
func TestMediaImageReferer(t *testing.T) {
	cases := []struct {
		host    string
		wantOK  bool
		wantRef string
	}{
		{"img9.doubanio.com", true, "https://movie.douban.com/"},
		{"qnmob3.doubanio.com", true, "https://movie.douban.com/"},
		{"DOUBANIO.COM", true, "https://movie.douban.com/"}, // 大小写不敏感
		{"image.tmdb.org", true, ""},
		{"evil.com", false, ""},
		{"doubanio.com.attacker.io", false, ""}, // 后缀伪装不命中
		{"", false, ""},
	}
	for _, c := range cases {
		ref, ok := mediaImageReferer(c.host)
		if ok != c.wantOK || ref != c.wantRef {
			t.Errorf("mediaImageReferer(%q) = (%q,%v), want (%q,%v)", c.host, ref, ok, c.wantRef, c.wantOK)
		}
	}
}

func TestMediaClientForcesHTTP1AgainstHTTP2Server(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.Proto)
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	resp, err := newMediaClient(true, true).Get(srv.URL)
	if err != nil {
		t.Fatalf("media client request failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); got != "HTTP/1.1" {
		t.Fatalf("protocol = %q, want HTTP/1.1", got)
	}
}
