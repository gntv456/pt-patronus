package oper

import (
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// NoticeOper 站内通知 CRUD + 去重查询（全局通知中心收件箱）。
type NoticeOper struct {
	db *gorm.DB
}

// NewNoticeOper 构造。
func NewNoticeOper(db *gorm.DB) *NoticeOper {
	return &NoticeOper{db: db}
}

// Create 新建通知记录。
func (o *NoticeOper) Create(n *models.Notice) error {
	// OnConflict DoNothing：(source,key) 唯一索引兜底，cron 重叠/并发时重复通知静默跳过。
	return o.db.Clauses(clause.OnConflict{DoNothing: true}).Create(n).Error
}

// List 列通知（按创建时间倒序）。unreadOnly=true 仅未读；limit<=0 默认 200。
func (o *NoticeOper) List(unreadOnly bool, limit int) ([]models.Notice, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 { // 上限封顶，防 ?limit=999999999 全表灌内存（P1 DoS）
		limit = 1000
	}
	q := o.db.Order("created_at desc").Limit(limit)
	if unreadOnly {
		q = q.Where("read = ?", false)
	}
	var ns []models.Notice
	return ns, q.Find(&ns).Error
}

// MarkRead 标单条已读。
func (o *NoticeOper) MarkRead(id uint) error {
	return o.db.Model(&models.Notice{}).Where("id = ?", id).Update("read", true).Error
}

// MarkAllRead 全部已读。
func (o *NoticeOper) MarkAllRead() error {
	return o.db.Model(&models.Notice{}).Where("read = ?", false).Update("read", true).Error
}

// Delete 删单条。
func (o *NoticeOper) Delete(id uint) error {
	return o.db.Delete(&models.Notice{}, id).Error
}

// ClearAll 一键清空全部通知（含未读）。返回删除条数。
func (o *NoticeOper) ClearAll() (int64, error) {
	res := o.db.Where("1 = 1").Delete(&models.Notice{})
	return res.RowsAffected, res.Error
}

// ExistsSince 是否存在 source(+key) 且 created_at>=since 的记录（去重：同事件当天已发则跳过）。
func (o *NoticeOper) ExistsSince(source, key string, since time.Time) (bool, error) {
	if source == "" {
		return false, nil
	}
	q := o.db.Model(&models.Notice{}).Where("source = ? AND created_at >= ?", source, since)
	if key != "" {
		q = q.Where("key = ?", key)
	}
	var cnt int64
	err := q.Count(&cnt).Error
	return cnt > 0, err
}

// CountUnread 未读数（铃铛角标）。
func (o *NoticeOper) CountUnread() (int64, error) {
	var cnt int64
	err := o.db.Model(&models.Notice{}).Where("read = ?", false).Count(&cnt).Error
	return cnt, err
}

// Cleanup 清理 before 之前已读通知（保留未读）。
func (o *NoticeOper) Cleanup(before time.Time) (int64, error) {
	res := o.db.Where("created_at < ? AND read = ?", before, true).Delete(&models.Notice{})
	return res.RowsAffected, res.Error
}
