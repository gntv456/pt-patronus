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

// TestTelegramSendCustomAPIURL：自定义 apiURL 生效；正文含 url 链接。
func TestTelegramSendCustomAPIURL(t *testing.T) {
	var gotPath, gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var m map[string]string
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &m)
		gotText = m["text"]
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer srv.Close()

	tg := NewTelegram("TG-TOKEN", "chat-1", srv.URL)
	if err := tg.Send(context.Background(), Message{Title: "标题", Body: "正文", Data: map[string]any{"url": "https://detail/x"}}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/botTG-TOKEN/sendMessage") {
		t.Errorf("path = %q, want .../botTG-TOKEN/sendMessage", gotPath)
	}
	if !strings.Contains(gotText, "标题") || !strings.Contains(gotText, "正文") || !strings.Contains(gotText, "https://detail/x") {
		t.Errorf("text missing parts: %q", gotText)
	}
}

// TestTelegramEmptyGuard：缺 token/chat_id 报错；空消息报错。
func TestTelegramEmptyGuard(t *testing.T) {
	if err := (&Telegram{}).Send(context.Background(), Message{Title: "x"}); err == nil {
		t.Error("no token/chat → expect error")
	}
	if err := (&Telegram{token: "t", chatID: "c"}).Send(context.Background(), Message{}); err == nil {
		t.Error("empty message → expect error")
	}
}
