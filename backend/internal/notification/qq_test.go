// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package notification

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestQQSendMarkdownHappyPath：token 端点收 camelCase body；发送端点收 Markdown(msg_type=2) + QQBot 头。
func TestQQSendMarkdownHappyPath(t *testing.T) {
	var tokenBody, sendBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/getAppAccessToken"):
			tokenBody = string(b)
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok-123", "expires_in": 7200})
		case strings.Contains(r.URL.Path, "/messages"):
			sendBody = string(b)
			if r.Header.Get("Authorization") != "QQBot tok-123" {
				t.Errorf("auth header = %q, want QQBot tok-123", r.Header.Get("Authorization"))
			}
			if !strings.Contains(r.URL.Path, "/users/u1/messages") {
				t.Errorf("send path = %q, want c2c users/u1", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "message": "ok"})
		}
	}))
	defer srv.Close()

	q := NewQQ("app-id", "app-secret", "u1", "")
	q.tokenURL = srv.URL + "/getAppAccessToken"
	q.apiBase = srv.URL
	if err := q.Send(context.Background(), Message{Title: "标题", Body: "正文", Data: map[string]any{"url": "https://x"}}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.Contains(tokenBody, `"appId":"app-id"`) || !strings.Contains(tokenBody, `"clientSecret":"app-secret"`) {
		t.Errorf("token body not camelCase: %s", tokenBody)
	}
	if !strings.Contains(sendBody, `"msg_type":2`) || !strings.Contains(sendBody, `"markdown":{"content":`) {
		t.Errorf("send body not markdown: %s", sendBody)
	}
	if !strings.Contains(sendBody, `**`) || !strings.Contains(sendBody, `[查看详情](https://x)`) {
		t.Errorf("markdown content missing bold/link: %s", sendBody)
	}
}

// TestQQMarkdownFallbackToPlain：服务端对 Markdown 返回 11244 → 客户端回退纯文本(msg_type=0)。
func TestQQMarkdownFallbackToPlain(t *testing.T) {
	var calls []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(r.URL.Path, "/getAppAccessToken") {
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 7200})
			return
		}
		var m qqMessage
		_ = json.Unmarshal(b, &m)
		calls = append(calls, m.MsgType)
		if m.MsgType == 2 {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 11244, "message": "无 markdown 权限"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0})
	}))
	defer srv.Close()

	q := NewQQ("a", "s", "u1", "")
	q.tokenURL = srv.URL + "/getAppAccessToken"
	q.apiBase = srv.URL
	if err := q.Send(context.Background(), Message{Title: "T", Body: "B"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(calls) != 2 || calls[0] != 2 || calls[1] != 0 {
		t.Fatalf("expected markdown(2) then plain(0), got %v", calls)
	}
}

// TestQQResolveTarget：group 优先于 c2c；都空报错。
func TestQQResolveTarget(t *testing.T) {
	if _, isGroup := (&QQ{groupOpenID: "g1"}).resolveTarget(); !isGroup {
		t.Error("groupOpenID set → expect group")
	}
	if _, isGroup := (&QQ{openID: "u1"}).resolveTarget(); isGroup {
		t.Error("only openID → expect c2c")
	}
	if tg, _ := (&QQ{}).resolveTarget(); tg != "" {
		t.Error("neither set → empty target")
	}
	if err := (&QQ{}).Send(context.Background(), Message{Title: "x"}); err == nil {
		t.Error("no recipient → expect error")
	}
}

func TestQQEscapeMarkdown(t *testing.T) {
	in := "a*b_c[d]e`f"
	out := qqEscapeMarkdown(in)
	for _, c := range []string{`\*`, `\_`, `\[`, `\]`, "\\`"} {
		if !strings.Contains(out, c) {
			t.Errorf("escape %q missing in %q", c, out)
		}
	}
}

func TestQQNeedsPlainFallback(t *testing.T) {
	for _, s := range []string{"code 11244 failed", "markdown not allowed", "无权限"} {
		if !qqNeedsPlainFallback(errStr(s)) {
			t.Errorf("expected fallback for %q", s)
		}
	}
	if qqNeedsPlainFallback(errStr("network timeout")) {
		t.Error("unrelated error should not trigger fallback")
	}
}

type errStr string

func (e errStr) Error() string { return string(e) }
