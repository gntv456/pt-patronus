// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package httpclient

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// 注意：httptest 监听 127.0.0.1，是内网地址，因此 ssrf 必须传 false。

func TestGetSendsCookiesAndUA(t *testing.T) {
	var gotUA, gotCookie string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotCookie = r.Header.Get("Cookie")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c, err := New(Options{Timeout: 0})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(context.Background(), srv.URL, "session_id=abc; token=xyz", false)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if !strings.Contains(gotCookie, "session_id=abc") || !strings.Contains(gotCookie, "token=xyz") {
		t.Errorf("cookie not sent: %q", gotCookie)
	}
	if gotUA == "" || !strings.Contains(gotUA, "Mozilla") {
		t.Errorf("UA not set: %q", gotUA)
	}
}

func TestRetryOn5xx(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c, err := New(Options{MaxRetries: 3})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(context.Background(), srv.URL, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 after retry, got %d", resp.StatusCode)
	}
	if hits != 3 {
		t.Fatalf("expected 3 attempts, got %d", hits)
	}
}

func TestCloudflareDetection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "cloudflare")
		w.WriteHeader(503)
		_, _ = w.Write([]byte("just a moment..."))
	}))
	defer srv.Close()

	c, _ := New(Options{})
	resp, err := c.Get(context.Background(), srv.URL, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.IsCloudflare() {
		t.Error("expected cloudflare detection")
	}
}

func TestSSRFBlocksLocalhost(t *testing.T) {
	c, _ := New(Options{})
	_, err := c.Get(context.Background(), "http://127.0.0.1:9/server-status", "", true)
	if err == nil {
		t.Error("expected SSRF block for localhost")
	}
}

// TestDecodeBodyGBK GBK 页面应转码为 UTF-8（中文关键字判定依赖此）。
func TestDecodeBodyGBK(t *testing.T) {
	gbkBytes, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte("<html><title>成功删除</title>"))
	if err != nil {
		t.Fatal(err)
	}
	out := decodeBody(gbkBytes, "text/html; charset=gbk")
	if !strings.Contains(string(out), "成功删除") {
		t.Errorf("gbk not decoded to utf-8: %q", out)
	}
}

// TestDecodeBodyPassthrough UTF-8 / 空 / 非 GBK 族应原样返回（无回归）。
func TestDecodeBodyPassthrough(t *testing.T) {
	in := []byte("<html>正常 UTF-8 成功</html>")
	if out := decodeBody(in, "text/html; charset=utf-8"); !bytes.Equal(in, out) {
		t.Errorf("utf-8 with charset should pass through, got %q", out)
	}
	if out := decodeBody(in, ""); !bytes.Equal(in, out) {
		t.Errorf("utf-8 without content-type should pass through, got %q", out)
	}
	if out := decodeBody(nil, ""); out != nil {
		t.Errorf("nil should return nil, got %q", out)
	}
	// 显式声明的 latin-1 不在 GBK 族，不应转码。
	if out := decodeBody([]byte("caf\xe9"), "text/html; charset=iso-8859-1"); bytes.Equal(nil, out) {
		t.Errorf("latin-1 should pass through unchanged")
	}
}
