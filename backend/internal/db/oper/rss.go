package oper

import (
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// RssOper 封装 RssSubscription 表读写。
type RssOper struct {
	db *gorm.DB
}

func NewRssOper(db *gorm.DB) *RssOper { return &RssOper{db: db} }

// List 全部 RSS 订阅（按更新时间倒序）。
func (o *RssOper) List() ([]models.RssSubscription, error) {
	var rs []models.RssSubscription
	err := o.db.Order("updated_at DESC").Find(&rs).Error
	return rs, err
}

// ListEnabled 仅启用的订阅，供 cron 扫描。
func (o *RssOper) ListEnabled() ([]models.RssSubscription, error) {
	var rs []models.RssSubscription
	err := o.db.Where("enabled = ?", true).Find(&rs).Error
	return rs, err
}

// Get 单条。
func (o *RssOper) Get(id uint) (*models.RssSubscription, error) {
	var r models.RssSubscription
	if err := o.db.First(&r, id).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

// Create 新建。
func (o *RssOper) Create(r *models.RssSubscription) error {
	// 地址在写入面校验（同包 downloader.go / mediaserver.go 都调 validateURLScheme）：
	// 「忘了写 https://」这类笔误从前会原样入库，成为一条列表里看着正常、
	// 抓取却永远失败的订阅，失败要到 cron 那一刻才以英文尾巴露出来。
	if err := validateURLScheme(r.URL); err != nil {
		return err
	}
	// Create 会把 GORM default:true 回填进 r.Enabled，必须先保存原始值再判断，
	// 否则 Create 后 r.Enabled 已被改为 true，回写条件永假（同 SiteOper 用 input 的等价做法）。
	enabled := r.Enabled
	if err := o.db.Create(r).Error; err != nil {
		return err
	}
	// GORM default:true 会把 Enabled=false 当零值吞为 true → 显式写回（同 SiteOper.Create）。
	if !enabled {
		return o.db.Model(r).Update("enabled", false).Error
	}
	return nil
}

// Update 保存。
func (o *RssOper) Update(r *models.RssSubscription) error {
	// 与 Create 同一判据（一处校验，两个入口共用），编辑时也拒掉不可用地址。
	if err := validateURLScheme(r.URL); err != nil {
		return err
	}
	return o.db.Save(r).Error
}

// Delete 删除订阅及其条目。
func (o *RssOper) Delete(id uint) error {
	return o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("subscription_id = ?", id).Delete(&models.RssItem{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.RssSubscription{}, id).Error
	})
}

// MarkChecked 记录本次检查时间。
func (o *RssOper) MarkChecked(id uint) error {
	now := time.Now()
	return o.db.Model(&models.RssSubscription{}).Where("id = ?", id).
		Update("last_checked_at", now).Error
}

// RssItemOper 封装 RssItem 表读写（去重 + 列表）。
type RssItemOper struct {
	db *gorm.DB
}

func NewRssItemOper(db *gorm.DB) *RssItemOper { return &RssItemOper{db: db} }

// Exists 该订阅是否已记录此 item_key（去重）。
func (o *RssItemOper) Exists(subID uint, key string) (bool, error) {
	var count int64
	err := o.db.Model(&models.RssItem{}).
		Where("subscription_id = ? AND item_key = ?", subID, key).Count(&count).Error
	return count > 0, err
}

// Create 新建条目（联合唯一索引兜底）。
func (o *RssItemOper) Create(it *models.RssItem) error { return o.db.Create(it).Error }

// Delete 删除单条 RSS 条目（Add 失败回滚去重行用：让下轮 Check 重见重试）。
func (o *RssItemOper) Delete(id uint) error {
	return o.db.Delete(&models.RssItem{}, id).Error
}

// MarkDownloaded 标记已下载。
func (o *RssItemOper) MarkDownloaded(id uint) error {
	return o.db.Model(&models.RssItem{}).Where("id = ?", id).Update("downloaded", true).Error
}

// ListBySubscription 某订阅的条目（倒序，限 limit）。
func (o *RssItemOper) ListBySubscription(subID uint, limit int) ([]models.RssItem, error) {
	var its []models.RssItem
	q := o.db.Where("subscription_id = ?", subID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&its).Error
	return its, err
}

// CountBySubscription 某订阅的条目总数。列表有上限，界面要说「还有多少没显示」，
// 所以总数与清单必须同批给出（对齐 MonitorOper.ListAlerts 的 total/truncated 口径）。
func (o *RssItemOper) CountBySubscription(subID uint) (int64, error) {
	var n int64
	err := o.db.Model(&models.RssItem{}).Where("subscription_id = ?", subID).Count(&n).Error
	return n, err
}

// Prune 清理 before 之前创建的条目（保留已下载记录，供历史追溯）。
// RSS items 每 5 分钟抓取即落行，无 TTL 会无限膨胀（数据审计 P2）。
func (o *RssItemOper) Prune(before time.Time) (int64, error) {
	res := o.db.Where("created_at < ? AND downloaded = ?", before, false).
		Delete(&models.RssItem{})
	return res.RowsAffected, res.Error
}
