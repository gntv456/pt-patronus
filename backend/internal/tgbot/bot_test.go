package tgbot

import (
	"context"
	"errors"
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
	talk := func(ctx context.Context, chatKey, text string) (<-chan llm.Event, error) {
		ch := make(chan llm.Event, 2)
		ch <- llm.Event{Type: llm.EventText, Text: "已收到：" + text}
		ch <- llm.Event{Type: llm.EventDone}
		close(ch)
		return ch, nil
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
	talk := func(ctx context.Context, chatKey, text string) (<-chan llm.Event, error) {
		called = true
		ch := make(chan llm.Event)
		close(ch)
		return ch, nil
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
	b := New("T", srv.URL, nil, false, func(ctx context.Context, k, txt string) (<-chan llm.Event, error) {
		ch := make(chan llm.Event)
		close(ch)
		return ch, nil
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

// talk 报硬失败时必须以 ⚠️ 原样回报，不能落进「（已执行，无文本输出）」假成功。
func TestHandleUpdateTalkErrorNotFalseSuccess(t *testing.T) {
	srv, mu, sent := newFakeAPI(t)
	defer srv.Close()
	b := New("T", srv.URL, nil, false, func(ctx context.Context, k, txt string) (<-chan llm.Event, error) {
		return nil, errors.New("创建对话会话失败，请稍后重试")
	}, nil)
	b.handleUpdate(context.Background(), tgUpdate{
		Message: &tgMessage{Chat: tgChat{ID: 1, Type: "private"}, From: tgUser{ID: 1}, Text: "搜 Inception"},
	})
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(*sent, "创建对话会话失败") {
		t.Errorf("talk 失败必须回报错误，实际: %q", *sent)
	}
	if strings.Contains(*sent, "已执行") {
		t.Errorf("talk 失败不能被渲染成「已执行」假成功: %q", *sent)
	}
}

// 零事件流（agent 侧 crash 被 safego 静默 recover）不能渲染成「（已执行，无文本输出）」。
func TestHandleUpdateZeroEventStreamNotFalseSuccess(t *testing.T) {
	srv, mu, sent := newFakeAPI(t)
	defer srv.Close()
	b := New("T", srv.URL, nil, false, func(ctx context.Context, k, txt string) (<-chan llm.Event, error) {
		ch := make(chan llm.Event)
		close(ch)
		return ch, nil
	}, nil)
	b.handleUpdate(context.Background(), tgUpdate{
		Message: &tgMessage{Chat: tgChat{ID: 1, Type: "private"}, From: tgUser{ID: 1}, Text: "搜 Inception"},
	})
	mu.Lock()
	defer mu.Unlock()
	if strings.Contains(*sent, "已执行") {
		t.Errorf("零事件流不能被渲染成「已执行」假成功: %q", *sent)
	}
	if !strings.Contains(*sent, "没有返回任何结果") {
		t.Errorf("零事件流必须给出重试提示，实际: %q", *sent)
	}
}

// 有事件但无文本（done 正常收尾）保留「已执行，无文本输出」的既有提示。
func TestHandleUpdateDoneWithoutTextKeepsExecutedHint(t *testing.T) {
	srv, mu, sent := newFakeAPI(t)
	defer srv.Close()
	b := New("T", srv.URL, nil, false, func(ctx context.Context, k, txt string) (<-chan llm.Event, error) {
		ch := make(chan llm.Event, 1)
		ch <- llm.Event{Type: llm.EventDone}
		close(ch)
		return ch, nil
	}, nil)
	b.handleUpdate(context.Background(), tgUpdate{
		Message: &tgMessage{Chat: tgChat{ID: 1, Type: "private"}, From: tgUser{ID: 1}, Text: "暂停所有任务"},
	})
	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(*sent, "已执行，无文本输出") {
		t.Errorf("done 无文本应保留「已执行」提示，实际: %q", *sent)
	}
}
