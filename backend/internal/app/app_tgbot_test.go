// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

import (
	"errors"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// 建会话失败（哪怕 Create 已回填 ID 的半成功态）必须报错且不缓存会话。
func TestTgChatSessionsResolveCreateErrorSurfaces(t *testing.T) {
	s := newTgChatSessions()
	_, err := s.resolve("tg:1", func(c *models.AgentConversation) error {
		c.ID = 7 // 半成功态：不能因为「有 ID」就把失败吞掉
		return errors.New("database is locked")
	})
	if !errors.Is(err, errTgConvCreate) {
		t.Fatalf("Create 失败必须返回 errTgConvCreate，实际: %v", err)
	}
	if _, ok := s.convs["tg:1"]; ok {
		t.Error("失败的会话不能进缓存（后续消息会复用坏会话）")
	}
}

// Create 报成功却未回填主键 → 按失败处理（否则消息会挂到会话 0 上）。
func TestTgChatSessionsResolveZeroIDIsError(t *testing.T) {
	s := newTgChatSessions()
	_, err := s.resolve("tg:1", func(*models.AgentConversation) error { return nil })
	if !errors.Is(err, errTgConvCreate) {
		t.Fatalf("ID==0 必须返回 errTgConvCreate，实际: %v", err)
	}
	if _, ok := s.convs["tg:1"]; ok {
		t.Error("ID==0 的会话不能进缓存")
	}
}

// 同一 chatKey 复用同一会话，不重复建会话。
func TestTgChatSessionsResolveReusesConversation(t *testing.T) {
	s := newTgChatSessions()
	calls := 0
	create := func(c *models.AgentConversation) error {
		calls++
		c.ID = uint(100 + calls)
		return nil
	}
	id1, err := s.resolve("tg:1", create)
	if err != nil {
		t.Fatalf("首次 resolve 失败: %v", err)
	}
	id2, err := s.resolve("tg:1", create)
	if err != nil || id1 != id2 || calls != 1 {
		t.Fatalf("同 chatKey 应复用: id1=%d id2=%d calls=%d err=%v", id1, id2, calls, err)
	}
}

// /clear 后同一 chatKey 重建会话。
func TestTgChatSessionsClearRebuilds(t *testing.T) {
	s := newTgChatSessions()
	calls := 0
	create := func(c *models.AgentConversation) error {
		calls++
		c.ID = uint(calls)
		return nil
	}
	if _, err := s.resolve("tg:1", create); err != nil {
		t.Fatal(err)
	}
	s.clear("tg:1")
	id, err := s.resolve("tg:1", create)
	if err != nil || calls != 2 || id != 2 {
		t.Fatalf("clear 后应重建: id=%d calls=%d err=%v", id, calls, err)
	}
}
