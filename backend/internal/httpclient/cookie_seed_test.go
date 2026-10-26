package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 站点 cookie 会随每次抓取重新 seed（RSS→抓种、消息→回帖…）。cookiejar 对
// Path 为空的 cookie 按 URL 目录推导存储路径（/dl/3 → "/dl/"），于是同一份
// cookie 在 "/" 与 "/dl/" 各存一份，请求头里每个 k=v 出现两遍。
// 值相同尚可苟过，值不同（站点换过 cookie）时服务端取哪一份就不可控了。
func TestSeedCookiesDoesNotDuplicatePerPath(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("Cookie"))
		w.WriteHeader(200)
	}))
	defer srv.Close()
	c, err := New(Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := c.Get(ctx, srv.URL+"/rss2", "uid=1; pass=abc", true); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, srv.URL+"/dl/3", "uid=1; pass=abc", true); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 requests, got %d", len(got))
	}
	for i, h := range got {
		if n := strings.Count(h, "uid=1"); n != 1 {
			t.Fatalf("request %d cookie 头重复了 %d 次: %q", i+1, n, h)
		}
		if !strings.Contains(h, "pass=abc") {
			t.Fatalf("request %d 丢了 cookie: %q", i+1, h)
		}
	}
}

// 正控：换一份新值 seed 后只能看到新值（旧值必须被替换，不能与新值同时发出）。
func TestSeedCookiesReplacesStaleValue(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("Cookie"))
		w.WriteHeader(200)
	}))
	defer srv.Close()
	c, err := New(Options{AllowPrivate: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// 根路径与子目录各 seed 一次（真实场景：RSS /rss2 → 抓种 /dl/3）。
	if _, err := c.Get(ctx, srv.URL+"/rss2", "uid=old", true); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(ctx, srv.URL+"/dl/3", "uid=new", true); err != nil {
		t.Fatal(err)
	}
	last := got[len(got)-1]
	if strings.Contains(last, "uid=old") || !strings.Contains(last, "uid=new") {
		t.Fatalf("旧 cookie 未被替换: %q", last)
	}
}
