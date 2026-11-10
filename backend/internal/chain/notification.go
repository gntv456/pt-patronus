// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/notification"
)

// NotificationChain 统一通知入口：渲染 → 广播事件 → 多通道扇出。
// 借鉴 MoviePilot 的 post_message。
type NotificationChain struct {
	Base
	mgr *notification.Manager
}

// NewNotificationChain 构造。
func NewNotificationChain(b Base, mgr *notification.Manager) *NotificationChain {
	return &NotificationChain{Base: b, mgr: mgr}
}

// Notify 发送通知：先广播 NoticeMessage 事件（供链式处理器/插件改写），
// 再向所有通道并发扇出。
func (c *NotificationChain) Notify(ctx context.Context, msg notification.Message) []notification.SendResult {
	c.Emit(ctx, "NoticeMessage", map[string]any{
		"title": msg.Title,
		"body":  msg.Body,
		"level": msg.Level,
	})
	return c.mgr.Notify(ctx, msg)
}
