package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestUTLSClientGETsHTTPS 验证 uTLS（Chrome 指纹）client 能完成 TLS 握手并发起请求：
// 连 httptest TLS server（127.0.0.1 自签），GET 200 并读到响应体。
// 覆盖 utlsDialContext 全链——TCP 拨号 + utls.UClient 握手 + HTTP 请求。
// AllowPrivate 放行内网 httptest；InsecureTLS 跳过自签校验（生产保持 false）。
func TestUTLSClientGETsHTTPS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "签到成功，获得 10 魔力")
	}))
	defer srv.Close()

	c, err := New(Options{AllowPrivate: true, InsecureTLS: true, UTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Get(context.Background(), srv.URL, "", false)
	if err != nil {
		t.Fatalf("uTLS GET failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if !strings.Contains(string(resp.Body), "签到成功") {
		t.Errorf("unexpected body: %q", resp.Body)
	}
}

// TestUTLSClientSSRFBlocksPrivate uTLS client 在非 AllowPrivate 模式下，
// SSRF 守卫仍生效——拨号内网目标被拒（utlsDialContext 复用 IsPublicIP）。
func TestUTLSClientSSRFBlocksPrivate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	c, err := New(Options{InsecureTLS: true, UTLS: true}) // AllowPrivate=false
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(context.Background(), srv.URL, "", false)
	if err == nil {
		t.Fatal("expected SSRF block for loopback under uTLS")
	}
	if !strings.Contains(err.Error(), "ssrf") && !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected ssrf block error, got %v", err)
	}
}
