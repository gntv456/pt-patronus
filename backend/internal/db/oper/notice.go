// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

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
	q := o.db.Order("created_at desc").Limit(NormalizeNoticeLimit(limit))
	if unreadOnly {
		q = q.Where("read = ?", false)
	}
	var ns []models.Notice
	return ns, q.Find(&ns).Error
}

const (
	NoticeListDefault = 200
	NoticeListLimit   = 1000
)

// NormalizeNoticeLimit 留空/非法取默认，超上限取上限（不再悄悄退回默认值，
// 否则 ?limit=999999 的调用方会以为自己拿到了全部）。
func NormalizeNoticeLimit(n int) int {
	if n <= 0 {
		return NoticeListDefault
	}
	if n > NoticeListLimit { // 上限封顶，防 ?limit=999999999 全表灌内存（P1 DoS）
		return NoticeListLimit
	}
	return n
}

// Count 通知条数（与 List 同一套筛选）。列表有上限，不把总数带出去，
// 收件箱就把「最近 200 条」读成「全部」，超出部分的未读永远看不见。
func (o *NoticeOper) Count(unreadOnly bool) (int64, error) {
	q := o.db.Model(&models.Notice{})
	if unreadOnly {
		q = q.Where("read = ?", false)
	}
	var cnt int64
	err := q.Count(&cnt).Error
	return cnt, err
}

// MarkRead 标单条已读。
func (o *NoticeOper) MarkRead(id uint) error {
	res := o.db.Model(&models.Notice{}).Where("id = ?", id).Update("read", true)
	if res.Error != nil {
		return res.Error
	}
	// 通知已被删除/不存在时报「已读」是假账：前端会把它从未读列表里划掉，
	// 用户以为处理过了，实际那条根本不在库里。
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// MarkAllRead 全部已读。
func (o *NoticeOper) MarkAllRead() error {
	return o.db.Model(&models.Notice{}).Where("read = ?", false).Update("read", true).Error
}

// Delete 删单条。
func (o *NoticeOper) Delete(id uint) error {
	res := o.db.Delete(&models.Notice{}, id)
	if res.Error != nil {
		return res.Error
	}
	// 与同文件 MarkRead 同口径：GORM 删未命中主键不报错，不判行数就分不清
	// 「删掉了」与「本来就没有」，收件箱会弹「已删除」而那条早不在了。
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
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
	return o.Count(true)
}

// Cleanup 清理 before 之前已读通知（保留未读）。
func (o *NoticeOper) Cleanup(before time.Time) (int64, error) {
	res := o.db.Where("created_at < ? AND read = ?", before, true).Delete(&models.Notice{})
	return res.RowsAffected, res.Error
}
