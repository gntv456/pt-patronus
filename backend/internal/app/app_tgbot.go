// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

import (
	"errors"
	"log/slog"
	"sync"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// errTgConvCreate 是 TG 会话创建失败的统一用户可见文案（原始错误只进日志，避免英文原文进聊天）。
var errTgConvCreate = errors.New("创建对话会话失败，请稍后重试")

// tgChatSessions 维护 Telegram chatKey → AI 会话 ID 的进程内映射（重启后按需新建）。
type tgChatSessions struct {
	mu    sync.Mutex
	convs map[string]uint
}

func newTgChatSessions() *tgChatSessions { return &tgChatSessions{convs: map[string]uint{}} }

// resolve 取/建 chatKey 的会话 ID。创建失败必须显式返回错误并在调用方产出失败出口——
// 历史缺陷：失败时返回空事件流，tgbot 把「什么都没发生」渲染成「（已执行，无文本输出）」假成功。
func (s *tgChatSessions) resolve(chatKey string, create func(*models.AgentConversation) error) (uint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.convs[chatKey]; ok {
		return id, nil
	}
	conv := &models.AgentConversation{}
	if err := create(conv); err != nil {
		slog.Warn("tgbot: 创建对话会话失败", "chat", chatKey, "err", err)
		return 0, errTgConvCreate
	}
	if conv.ID == 0 {
		// Create 未报错却没回填主键：继续跑会把消息挂在会话 0 上（界面无处可见），按失败处理。
		slog.Warn("tgbot: 创建对话会话未返回 ID", "chat", chatKey)
		return 0, errTgConvCreate
	}
	s.convs[chatKey] = conv.ID
	return conv.ID, nil
}

// clear 丢弃 chatKey 的会话（/clear 命令；下次消息重新建会话）。
func (s *tgChatSessions) clear(chatKey string) {
	s.mu.Lock()
	delete(s.convs, chatKey)
	s.mu.Unlock()
}
