package oper

import (
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SubscriptionOper 封装订阅与命中记录的读写。
type SubscriptionOper struct {
	db *gorm.DB
}

func NewSubscriptionOper(db *gorm.DB) *SubscriptionOper {
	return &SubscriptionOper{db: db}
}

// List 全部订阅（按创建时间倒序）。
func (o *SubscriptionOper) List() ([]models.Subscription, error) {
	var ss []models.Subscription
	err := o.db.Order("created_at DESC").Find(&ss).Error
	return ss, err
}

// ListActive 仅订阅中（status=subscribed）的订阅，供 cron 遍历。
func (o *SubscriptionOper) ListActive() ([]models.Subscription, error) {
	var ss []models.Subscription
	err := o.db.Where("status = ?", "subscribed").Order("last_checked_at NULLS FIRST").Find(&ss).Error
	return ss, err
}

// Get 单条。
func (o *SubscriptionOper) Get(id uint) (*models.Subscription, error) {
	var s models.Subscription
	if err := o.db.First(&s, id).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// FindActiveBySource 按源+source_id 找订阅中的订阅（创建时去重用）。
func (o *SubscriptionOper) FindActiveBySource(source, sourceID string) (*models.Subscription, error) {
	var s models.Subscription
	err := o.db.Where("source = ? AND source_id = ? AND status = ?", source, sourceID, "subscribed").First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Create 新建。
func (o *SubscriptionOper) Create(s *models.Subscription) error {
	return o.db.Create(s).Error
}

// Update 保存（resolution/downloader/status 等编辑）。
func (o *SubscriptionOper) Update(s *models.Subscription) error {
	return o.db.Save(s).Error
}

// Delete 删除订阅及其命中记录。
func (o *SubscriptionOper) Delete(id uint) error {
	return o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("subscription_id = ?", id).Delete(&models.SubscriptionHit{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Subscription{}, id).Error
	})
}

// MarkChecked 记录本次检查时间。
func (o *SubscriptionOper) MarkChecked(id uint) error {
	now := time.Now()
	return o.db.Model(&models.Subscription{}).Where("id = ?", id).Update("last_checked_at", now).Error
}

// SetStatus 更新状态（subscribed/completed/paused）。
func (o *SubscriptionOper) SetStatus(id uint, status string) error {
	return o.db.Model(&models.Subscription{}).Where("id = ?", id).Update("status", status).Error
}

// HitExists 该订阅是否已推送过此 torrent_key（去重）。
func (o *SubscriptionOper) HitExists(subID uint, key string) (bool, error) {
	var count int64
	err := o.db.Model(&models.SubscriptionHit{}).
		Where("subscription_id = ? AND torrent_key = ?", subID, key).Count(&count).Error
	return count > 0, err
}

// AddHit 记录一次命中。
func (o *SubscriptionOper) AddHit(h *models.SubscriptionHit) error {
	// OnConflict DoNothing：(subscription_id,torrent_key) 唯一索引兜底，并发/重叠重复命中静默跳过。
	return o.db.Clauses(clause.OnConflict{DoNothing: true}).Create(h).Error
}
