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

// List 按更新时间倒序（最近活跃在前）。始终返回非 nil。
func (o *AgentConversationOper) List() ([]models.AgentConversation, error) {
	out := make([]models.AgentConversation, 0)
	return out, o.db.Order("updated_at DESC").Limit(200).Find(&out).Error
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
func (o *AgentConversationOper) Create(c *models.AgentConversation) error { return o.db.Create(c).Error }

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
func (o *AgentMessageOper) ListByConv(convID uint) ([]models.AgentMessage, error) {
	out := make([]models.AgentMessage, 0)
	return out, o.db.Where("conversation_id = ?", convID).Order("created_at ASC").Find(&out).Error
}

// Append 追加一条消息。
func (o *AgentMessageOper) Append(m *models.AgentMessage) error { return o.db.Create(m).Error }
