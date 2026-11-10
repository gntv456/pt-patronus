// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package llmgateway

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/llm"
)

// 网关是给别人家的客户端（Claude Code / OpenAI SDK / 各类 Agent）用的：
// 一次上游失败如果被写成「正常结束的空回答」，客户端不会重试、不会报错，
// 用户端只看到「模型没回话」。这几条用例钉的就是「失败必须有失败的样子」。

func eventsOf(evs ...llm.Event) chan llm.Event {
	ch := make(chan llm.Event, len(evs))
	for _, e := range evs {
		ch <- e
	}
	close(ch)
	return ch
}

func TestStreamOpenAISurfaceFailure(t *testing.T) {
	t.Run("error 事件 ⇒ error 帧 + finish_reason=error", func(t *testing.T) {
		ev := eventsOf(llm.Event{Type: llm.EventError, Error: "上游 429：配额已用尽"})
		rec := httptest.NewRecorder()
		err := StreamOpenAI(rec, "m", ev)
		if err == nil || !strings.Contains(err.Error(), "配额已用尽") {
			t.Fatalf("要把原因回给调用方（日志要写），got %v", err)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `"type":"server_error"`) || !strings.Contains(body, "配额已用尽") {
			t.Errorf("缺 OpenAI 兼容的流内 error 帧，body=%s", body)
		}
		if !strings.Contains(body, `"finish_reason":"error"`) {
			t.Errorf("终结块不许再写 stop，body=%s", body)
		}
		if !strings.HasSuffix(strings.TrimSpace(body), "data: [DONE]") {
			t.Error("仍要 [DONE] 收尾（旧契约）")
		}
	})

	t.Run("正控：正常回答只写 stop，不出 error 帧", func(t *testing.T) {
		ev := eventsOf(
			llm.Event{Type: llm.EventText, Text: "你好"},
			llm.Event{Type: llm.EventDone},
		)
		rec := httptest.NewRecorder()
		if err := StreamOpenAI(rec, "m", ev); err != nil {
			t.Fatalf("干净一轮不该报错: %v", err)
		}
		body := rec.Body.String()
		if strings.Contains(body, "server_error") || strings.Contains(body, `"finish_reason":"error"`) {
			t.Errorf("成功轮次被标成了失败，body=%s", body)
		}
		if !strings.Contains(body, `"finish_reason":"stop"`) {
			t.Errorf("正常收尾应有 stop，body=%s", body)
		}
	})

	t.Run("流被提前关闭（没有 done）⇒ 不算正常结束", func(t *testing.T) {
		rec := httptest.NewRecorder()
		err := StreamOpenAI(rec, "m", eventsOf())
		if err == nil {
			t.Fatal("没有 done 的流必须报错，不能回 nil")
		}
		body := rec.Body.String()
		if !strings.Contains(body, `"finish_reason":"error"`) {
			t.Errorf("应写 error 而不是静默 stop，body=%s", body)
		}
	})

	t.Run("错误原因是空串 ⇒ 也要给一句非空的话", func(t *testing.T) {
		ev := eventsOf(llm.Event{Type: llm.EventError}) // provider 回 503 且响应体为空
		rec := httptest.NewRecorder()
		err := StreamOpenAI(rec, "m", ev)
		if err == nil || strings.TrimSpace(err.Error()) == "" {
			t.Fatalf("空原因不能一路空到底，got %v", err)
		}
		if !strings.Contains(rec.Body.String(), "流被提前中断") {
			t.Errorf("应有兜底文案，body=%s", rec.Body.String())
		}
	})
}

func TestStreamAnthropicSurfaceFailure(t *testing.T) {
	t.Run("error 事件 ⇒ event: error，且不宣告 end_turn", func(t *testing.T) {
		ev := eventsOf(llm.Event{Type: llm.EventError, Error: "bad key"})
		rec := httptest.NewRecorder()
		if err := StreamAnthropic(rec, "m", ev); err == nil {
			t.Fatal("want error")
		}
		body := rec.Body.String()
		if !strings.Contains(body, "event: error") {
			t.Errorf("缺 Anthropic 的 error 事件，body=%s", body)
		}
		if strings.Contains(body, `"stop_reason":"end_turn"`) {
			t.Errorf("失败轮次不许写 end_turn（那是「答完了」），body=%s", body)
		}
		if !strings.Contains(body, "event: message_stop") {
			t.Error("仍要有 message_stop 收尾（旧契约：别把流吊着）")
		}
	})

	t.Run("流被提前关闭 ⇒ error 事件，而不是 end_turn", func(t *testing.T) {
		rec := httptest.NewRecorder()
		if err := StreamAnthropic(rec, "m", eventsOf()); err == nil {
			t.Fatal("没有 done 的流必须报错")
		}
		body := rec.Body.String()
		if !strings.Contains(body, "event: error") || strings.Contains(body, `"stop_reason":"end_turn"`) {
			t.Errorf("应报 error 而不是假装结束，body=%s", body)
		}
	})

	t.Run("正控：正常回答仍走 end_turn，不出 error 事件", func(t *testing.T) {
		ev := eventsOf(
			llm.Event{Type: llm.EventText, Text: "hi"},
			llm.Event{Type: llm.EventDone},
		)
		rec := httptest.NewRecorder()
		if err := StreamAnthropic(rec, "m", ev); err != nil {
			t.Fatalf("干净一轮不该报错: %v", err)
		}
		body := rec.Body.String()
		if strings.Contains(body, "event: error") {
			t.Errorf("成功轮次被标成失败，body=%s", body)
		}
		if !strings.Contains(body, `"stop_reason":"end_turn"`) {
			t.Errorf("正常收尾应有 end_turn，body=%s", body)
		}
	})
}
