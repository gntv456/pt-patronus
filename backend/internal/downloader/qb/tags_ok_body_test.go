package qb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newBodyClient 起一个只关心「写端点响应正文」的假 qB：登录恒 Ok.，
// addTags/removeTags 回 200 + 指定正文（真实 qB 就是这样恒 200、成败写在正文），
// 其余读端点回可用的空 JSON，免得断言被解析层提前拦下。
func newBodyClient(t *testing.T, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v2/auth/login":
			_, _ = w.Write([]byte("Ok."))
		case strings.HasSuffix(r.URL.Path, "/addTags"), strings.HasSuffix(r.URL.Path, "/removeTags"):
			w.WriteHeader(200)
			_, _ = w.Write([]byte(body))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(srv.Close)
	c, err := New(srv.URL, "u", "p")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// TestQBTagOpsHonorResponseBody 回归：addTags / removeTags 只看 HTTP 状态码。
// qB 这两个端点恒回 200，成败写在正文（"ok." / "fail."），sid 过期时甚至回 200 + 登录页 HTML——
// 于是「标签根本没改」被报成成功，而标签是刷流分流、自动删种与限流的选择依据。
func TestQBTagOpsHonorResponseBody(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{"ok. 是成功", "ok.", false},
		{"大写 Ok. 也是成功", "Ok.", false},
		{"空正文（部分版本无返回体）", "", false},
		{"fail. 必须算失败", "fail.", true},
		{"sid 过期回登录页 HTML", "<html><head><title/qBittorrent Web UI - Login", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newBodyClient(t, tc.body)
			errs := []error{
				c.AddTags(context.Background(), []string{"abcd0123"}, []string{"keep-mode"}),
				c.RemoveTags(context.Background(), []string{"abcd0123"}, []string{"keep-mode"}),
			}
			for i, err := range errs {
				op := []string{"addTags", "removeTags"}[i]
				if tc.wantErr && err == nil {
					t.Errorf("%s: 正文 %q 被当成功（下载器其实没改标签）", op, tc.body)
					continue
				}
				if !tc.wantErr && err != nil {
					t.Errorf("%s: 不应报错，got %v", op, err)
					continue
				}
				if err != nil && !strings.Contains(err.Error(), op) {
					t.Errorf("%s: 错误应指明是哪个操作，got %v", op, err)
				}
			}
		})
	}
}

// TestQBSetTagsPropagatesRefusal 覆盖式 SetTags 走 diff（先 remove 再 add），
// 任一侧被拒绝都要整体失败，不能让「旧标签没摘掉」伪装成设置完成。
func TestQBSetTagsPropagatesRefusal(t *testing.T) {
	c := newBodyClient(t, "fail.")
	err := c.SetTags(context.Background(), []string{"abcd0123"}, []string{"new-tag"})
	if err == nil {
		t.Fatal("下载器拒绝改标签时 SetTags 必须报错")
	}
	if !strings.Contains(err.Error(), "add") && !strings.Contains(err.Error(), "remove") {
		t.Errorf("错误应说明是哪个操作，got %v", err)
	}
}

// TestQBRejectsHTMLFromAuthPage qB 挂在需认证的反向代理后（或会话失效回登录页）时，
// API 会顶着 200 回一个 HTML 页面。一批写操作只看状态码、丢弃正文，
// 于是「下载器里啥也没改」被报成成功——移动保存目录/改文件优先级/加 Tracker 都是会
// 影响实际做种的写操作，必须整体拦下。
func TestQBRejectsHTMLFromAuthPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			_, _ = w.Write([]byte("Ok."))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<!DOCTYPE html>\n<html><head><title>Sign in</title></head><body>login</body></html>"))
	}))
	defer srv.Close()

	c, err := New(srv.URL, "u", "p")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	hash := "abcd0123"
	ops := map[string]func() error{
		"SetLocation":     func() error { return c.SetLocation(ctx, []string{hash}, "/downloads/moved") },
		"SetFilePriority": func() error { return c.SetFilePriority(ctx, hash, []int{0}, 1) },
		"AddTrackers":     func() error { return c.AddTrackers(ctx, hash, []string{"http://tracker/announce"}) },
		"AddTags":         func() error { return c.AddTags(ctx, []string{hash}, []string{"t"}) },
	}
	for name, fn := range ops {
		if err := fn(); err == nil {
			t.Errorf("%s: 200+HTML 被当成功（实际未生效）", name)
		} else if !strings.Contains(err.Error(), "HTML") {
			t.Errorf("%s: 错误应说明返回的是 HTML 页面，got %v", name, err)
		}
	}
}
