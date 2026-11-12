// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// AgentConversationOper 封装 AI 助手会话的读写。
type AgentConversationOper struct{ db *gorm.DB }

// NewAgentConversationOper 构造。
func NewAgentConversationOper(db *gorm.DB) *AgentConversationOper {
	return &AgentConversationOper{db: db}
}

// AgentConvListLimit 会话清单单页上限。
const AgentConvListLimit = 200

// List 按更新时间倒序（最近活跃在前）。始终返回非 nil。
func (o *AgentConversationOper) List() ([]models.AgentConversation, error) {
	out := make([]models.AgentConversation, 0)
	return out, o.db.Order("updated_at DESC").Limit(AgentConvListLimit).Find(&out).Error
}

// Count 会话总数。清单有上限，不把总数带出去，侧栏就把「最近 200 个会话」
// 画成「我用过的全部会话」，更早的会话在界面上找不到也删不掉。
func (o *AgentConversationOper) Count() (int64, error) {
	var n int64
	err := o.db.Model(&models.AgentConversation{}).Count(&n).Error
	return n, err
}

// Get 单条。
func (o *AgentConversationOper) Get(id uint) (*models.AgentConversation, error) {
	var c models.AgentConversation
	if err := o.db.First(&c, id).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// Create 新建会话。
func (o *AgentConversationOper) Create(c *models.AgentConversation) error {
	return o.db.Create(c).Error
}

// SetTitle 更新标题（首条消息后）。
func (o *AgentConversationOper) SetTitle(id uint, title string) error {
	return o.db.Model(&models.AgentConversation{}).Where("id = ?", id).Updates(map[string]any{"title": title}).Error
}

// Touch 更新时间（活跃会话置顶）。
func (o *AgentConversationOper) Touch(id uint) error {
	return o.db.Model(&models.AgentConversation{}).Where("id = ?", id).Update("updated_at", gorm.Expr("CURRENT_TIMESTAMP")).Error
}

// Delete 删会话及其全部消息。
func (o *AgentConversationOper) Delete(id uint) error {
	return o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("conversation_id = ?", id).Delete(&models.AgentMessage{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.AgentConversation{}, id).Error
	})
}

// AgentMessageOper 封装对话消息的读写。
type AgentMessageOper struct{ db *gorm.DB }

// NewAgentMessageOper 构造。
func NewAgentMessageOper(db *gorm.DB) *AgentMessageOper {
	return &AgentMessageOper{db: db}
}

// ListByConv 按时间正序（对话顺序）。始终返回非 nil。
// AI 执行侧（agent 历史裁剪）需要全量；用户侧展示请用 ListByConvLimit。
func (o *AgentMessageOper) ListByConv(convID uint) ([]models.AgentMessage, error) {
	out := make([]models.AgentMessage, 0)
	return out, o.db.Where("conversation_id = ?", convID).Order("created_at ASC").Find(&out).Error
}

// ListByConvLimit 取会话最近 limit 条（仍按时间正序返回，保持对话顺序）。
// Messages 端点在用：长会话（工具循环多轮、含 reasoning/tool_calls 大文本列）
// 全量返回可达数 MB，前端渲染卡；limit<=0 视为不限。
func (o *AgentMessageOper) ListByConvLimit(
	convID uint, limit int,
) ([]models.AgentMessage, error) {
	out := make([]models.AgentMessage, 0)
	if limit <= 0 {
		return out, o.db.Where("conversation_id = ?", convID).
			Order("created_at ASC").Find(&out).Error
	}
	if err := o.db.Where("conversation_id = ?", convID).
		Order("created_at DESC").Limit(limit).Find(&out).Error; err != nil {
		return nil, err
	}
	// DESC 取最近 N 条后反转为正序。
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// Append 追加一条消息。
func (o *AgentMessageOper) Append(m *models.AgentMessage) error { return o.db.Create(m).Error }
