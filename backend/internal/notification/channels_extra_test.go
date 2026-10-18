package notification

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// captured 记录测试服务器收到的最近一次请求。
type captured struct {
	method string
	path   string
	body   string
}

// newCapturingServer 启动一个测试服务器，把每次请求写入 *captured；返回 server + 关闭。
// body 是回给渠道的上游响应体（可省：留空即「200 + 空体」）。
// #232 之后空体不再算送达，所以打业务码信封的渠道必须显式给一份成功信封。
func newCapturingServer(t *testing.T, status int, cap *captured, body ...string) *httptest.Server {
	t.Helper()
	resp := ""
	if len(body) > 0 {
		resp = body[0]
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		cap.method = r.Method
		cap.path = r.URL.Path
		cap.body = string(b)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
}

func msgOf(title, body, url string) Message {
	m := Message{Title: title, Body: body}
	if url != "" {
		m.Data = map[string]any{"url": url}
	}
	return m
}

// TestExtraChannelsSend 覆盖 10 个 HTTP 通道：成功路径下服务器收到含标题/正文/链接的 POST。
func TestExtraChannelsSend(t *testing.T) {
	ctx := context.Background()
	title, body, link := "PTPatronus", "消息正文", "https://example.com/x"

	cases := []struct {
		name    string
		build   func(base string) Channel
		wantSub string // 请求体里必须出现的子串
		resp    string // 上游 200 的成功信封（各家的「成功码」不一样）
	}{
		{"pushplus", func(b string) Channel { return NewPushPlus(b, "TOK") }, title, `{"code":200}`},
		{"pushdeer", func(b string) Channel { return NewPushDeer(b, "PK") }, title, `{"code":0}`},
		{"ntfy", func(b string) Channel { return NewNtfy(b, "topic-x", "TOK") }, body, ""},
		{"gotify", func(b string) Channel { return NewGotify(b, "TOK") }, title, ""},
		{"apprise", func(b string) Channel { return NewApprise(b, "AK") }, title, ""},
		{"workwechat", func(u string) Channel { return NewWorkWechat(u) }, title, `{"errcode":0}`},
		{"wxpusher", func(b string) Channel { return NewWxPusher(b, "APP", "UID", "") }, title, `{"code":1000}`},
		{"lark", func(u string) Channel { return NewLark(u) }, title, `{"code":0}`},
		{"discord", func(u string) Channel { return NewDiscord(u) }, title, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var cap captured
			srv := newCapturingServer(t, http.StatusOK, &cap, tc.resp)
			defer srv.Close()

			ch := tc.build(srv.URL)
			if err := ch.Send(ctx, msgOf(title, body, link)); err != nil {
				t.Fatalf("Send error: %v", err)
			}
			if cap.method != http.MethodPost {
				t.Errorf("method = %q, want POST", cap.method)
			}
			if !strings.Contains(cap.body, tc.wantSub) {
				t.Errorf("body missing %q; got: %s", tc.wantSub, cap.body)
			}
			if !strings.Contains(cap.body, body) {
				t.Errorf("body missing message body %q; got: %s", body, cap.body)
			}
			if !strings.Contains(cap.body, link) {
				t.Errorf("body missing link %q; got: %s", link, cap.body)
			}
		})
	}
}

// TestServerChanForm Server酱 走表单编码，解析回查 title/desp。
func TestServerChanForm(t *testing.T) {
	ctx := context.Background()
	var cap captured
	srv := newCapturingServer(t, http.StatusOK, &cap, `{"code":0}`)
	defer srv.Close()
	if err := NewServerChan(srv.URL, "SCTKEY").Send(ctx, msgOf("PTPatronus", "消息正文", "https://example.com/x")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	form, err := url.ParseQuery(cap.body)
	if err != nil {
		t.Fatalf("parse form: %v", err)
	}
	if got := form.Get("title"); got != "PTPatronus" {
		t.Errorf("title = %q", got)
	}
	desp := form.Get("desp")
	if !strings.Contains(desp, "消息正文") || !strings.Contains(desp, "https://example.com/x") {
		t.Errorf("desp = %q", desp)
	}
	if !strings.HasSuffix(cap.path, "/SCTKEY.send") {
		t.Errorf("path = %q, want suffix /SCTKEY.send", cap.path)
	}
}

// TestExtraChannelsError 验证 5xx 响应返回错误；空凭据返回错误。
func TestExtraChannelsError(t *testing.T) {
	ctx := context.Background()
	var cap captured
	srv := newCapturingServer(t, http.StatusInternalServerError, &cap)
	defer srv.Close()

	if err := NewPushPlus(srv.URL, "T").Send(ctx, msgOf("t", "b", "")); err == nil {
		t.Error("pushplus 500: expected error")
	}
	if err := NewGotify(srv.URL, "T").Send(ctx, msgOf("t", "b", "")); err == nil {
		t.Error("gotify 500: expected error")
	}
	if err := NewServerChan(srv.URL, "K").Send(ctx, msgOf("t", "b", "")); err == nil {
		t.Error("serverchan 500: expected error")
	}

	// 缺少必填凭据/字段。
	emptyErrs := []Channel{
		NewPushPlus("", ""),
		NewPushDeer("", ""),
		NewGotify("", ""),
		NewApprise("", ""),
		NewNtfy("", "", ""),
		NewWorkWechat(""),
		NewWxPusher("", "", "", ""),
		NewLark(""),
		NewDiscord(""),
		NewServerChan("", ""),
	}
	for _, ch := range emptyErrs {
		if err := ch.Send(ctx, msgOf("t", "b", "")); err == nil {
			t.Errorf("%s: empty creds expected error", ch.Name())
		}
	}
}

// TestDiscordTruncation 验证超过 2000 字符的 content 被截断（Discord 限制）。
func TestDiscordTruncation(t *testing.T) {
	ctx := context.Background()
	var cap captured
	srv := newCapturingServer(t, http.StatusNoContent, &cap)
	defer srv.Close()
	long := strings.Repeat("a", 3000)
	if err := NewDiscord(srv.URL).Send(ctx, msgOf("", long, "")); err != nil {
		t.Fatalf("Send error: %v", err)
	}
	if strings.Contains(cap.body, "aaaaaaaaaaa") && len(cap.body) > 2100 {
		t.Errorf("discord body not truncated; len=%d", len(cap.body))
	}
}

// TestNtfyPriority 验证级别 → 优先级头映射。
func TestNtfyPriority(t *testing.T) {
	if got := ntfyPriority("danger"); got != "5" {
		t.Errorf("danger priority = %q, want 5", got)
	}
	if got := ntfyPriority("warning"); got != "4" {
		t.Errorf("warning priority = %q, want 4", got)
	}
	if got := ntfyPriority("info"); got != "3" {
		t.Errorf("info priority = %q, want 3", got)
	}
}

// TestAppriseType 验证级别 → Apprise type 映射。
func TestAppriseType(t *testing.T) {
	if got := appriseType("danger"); got != "failure" {
		t.Errorf("danger type = %q, want failure", got)
	}
	if got := appriseType("celebration"); got != "success" {
		t.Errorf("celebration type = %q, want success", got)
	}
}
