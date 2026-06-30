package oper

import (
	"time"

	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// WebhookOper 封装 Webhook（进站接收端点）表的读写 + Secret 加解密。
type WebhookOper struct {
	db *gorm.DB
	c  *security.Crypto
}

func NewWebhookOper(db *gorm.DB, c *security.Crypto) *WebhookOper {
	return &WebhookOper{db: db, c: c}
}

// List 全部 webhook（按创建时间倒序）。
func (o *WebhookOper) List() ([]models.Webhook, error) {
	var ws []models.Webhook
	err := o.db.Order("created_at DESC").Find(&ws).Error
	for i := range ws {
		ws[i].HasSecret = ws[i].SecretEnc != ""
	}
	return ws, err
}

// Get 单条。
func (o *WebhookOper) Get(id uint) (*models.Webhook, error) {
	var w models.Webhook
	if err := o.db.First(&w, id).Error; err != nil {
		return nil, err
	}
	w.HasSecret = w.SecretEnc != ""
	return &w, nil
}

// GetByToken 按 URL token 查（公开接收端点用）。
func (o *WebhookOper) GetByToken(token string) (*models.Webhook, error) {
	var w models.Webhook
	if err := o.db.Where("token = ? AND enabled = ?", token, true).First(&w).Error; err != nil {
		return nil, err
	}
	return &w, nil
}

// Create 新建（token 由调用方生成；secret 明文经 c 加密入库）。
func (o *WebhookOper) Create(w *models.Webhook, plainSecret string) error {
	if plainSecret != "" {
		enc, err := o.c.Encrypt(plainSecret)
		if err != nil {
			return err
		}
		w.SecretEnc = enc
	}
	return o.db.Create(w).Error
}

// Update 编辑（secret 为空则保留原密文）。
func (o *WebhookOper) Update(w *models.Webhook, plainSecret string) error {
	if plainSecret != "" {
		enc, err := o.c.Encrypt(plainSecret)
		if err != nil {
			return err
		}
		w.SecretEnc = enc
	}
	return o.db.Save(w).Error
}

// Delete 删除。
func (o *WebhookOper) Delete(id uint) error {
	return o.db.Delete(&models.Webhook{}, id).Error
}

// DecryptedSecret 解密签名校验密钥；空或失败返回 ""。
func (o *WebhookOper) DecryptedSecret(w *models.Webhook) string {
	if w == nil || w.SecretEnc == "" {
		return ""
	}
	s, err := o.c.Decrypt(w.SecretEnc)
	if err != nil {
		return ""
	}
	return s
}

// MarkTriggered 记录一次触发：计数+1、时间、截断的 payload。
func (o *WebhookOper) MarkTriggered(id uint, payload string) error {
	if len(payload) > 2000 {
		payload = payload[:2000]
	}
	return o.db.Model(&models.Webhook{}).Where("id = ?", id).Updates(map[string]any{
		"trigger_count":     gorm.Expr("trigger_count + 1"),
		"last_triggered_at": time.Now(),
		"last_payload":      payload,
	}).Error
}
