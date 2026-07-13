package tgbot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hxpt/ptpatronus/internal/llm"
)

// 伪 Bot API 服务器：记录 sendMessage 文本，所有请求回 {"ok":true}。
func newFakeAPI(t *testing.T) (*httptest.Server, *sync.Mutex, *string) {
	t.Helper()
	var mu sync.Mutex
	var sent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/sendMessage") {
			_ = r.ParseForm()
			mu.Lock()
			sent = r.FormValue("text")
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	return srv, &mu, &sent
}

func TestHandleUpdateTalkAndReply(t *testing.T) {
	srv, mu, sent := newFakeAPI(t)
	defer srv.Close()
	talk := func(ctx context.Context, chatKey, text string) <-chan llm.Event {
		ch := make(chan llm.Event, 2)
		ch <- llm.Event{Type: llm.EventText, Text: "已收到：" + text}
		ch <- llm.Event{Type: llm.EventDone}
		close(ch)
		return ch
	}
	b := New("FAKETOKEN", srv.URL, []int64{111}, false, talk, nil)
	b.handleUpdate(context.Background(), tgUpdate{
		UpdateID: 1,
		Message:  &tgMessage{MessageID: 10, Chat: tgChat{ID: 222, Type: "private"}, From: tgUser{ID: 111}, Text: "搜 Inception"},
	})
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(*sent, "已收到：搜 Inception") {
		t.Errorf("回帖文本不匹配: %q", *sent)
	}
}

func TestHandleUpdateAllowedFilter(t *testing.T) {
	called := false
	talk := func(ctx context.Context, chatKey, text string) <-chan llm.Event {
		called = true
		ch := make(chan llm.Event)
		close(ch)
		return ch
	}
	b := New("T", "", []int64{111}, false, talk, nil) // 仅允许 111
	b.handleUpdate(context.Background(), tgUpdate{
		Message: &tgMessage{Chat: tgChat{ID: 1, Type: "private"}, From: tgUser{ID: 999}, Text: "hi"},
	})
	if called {
		t.Error("非白名单用户不应触发 talk")
	}
}

func TestHandleUpdateHelpCommand(t *testing.T) {
	srv, mu, sent := newFakeAPI(t)
	defer srv.Close()
	b := New("T", srv.URL, nil, false, func(ctx context.Context, k, txt string) <-chan llm.Event {
		ch := make(chan llm.Event)
		close(ch)
		return ch
	}, nil)
	b.handleUpdate(context.Background(), tgUpdate{
		Message: &tgMessage{Chat: tgChat{ID: 1, Type: "private"}, From: tgUser{ID: 1}, Text: "/help"},
	})
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(*sent, "PTPatronus") {
		t.Errorf("/help 回帖不匹配: %q", *sent)
	}
}
