package oper

import (
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// CheckinRecordOper 签到历史记录 CRUD + 查询。
type CheckinRecordOper struct {
	db *gorm.DB
}

// NewCheckinRecordOper 构造。
func NewCheckinRecordOper(db *gorm.DB) *CheckinRecordOper {
	return &CheckinRecordOper{db: db}
}

// Create 新建一条签到记录。
func (o *CheckinRecordOper) Create(r *models.CheckinRecord) error {
	return o.db.Create(r).Error
}

// List 列签到记录（按时间倒序）。siteID=0 不过滤；limit<=0 默认 100。
func (o *CheckinRecordOper) List(siteID uint, limit, offset int) ([]models.CheckinRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	q := o.db.Order("created_at desc").Limit(limit).Offset(offset)
	if siteID > 0 {
		q = q.Where("site_id = ?", siteID)
	}
	var rs []models.CheckinRecord
	return rs, q.Find(&rs).Error
}

// Cleanup 清理 before 之前的签到记录（控制表体积；保留近期供回看）。
func (o *CheckinRecordOper) Cleanup(before time.Time) (int64, error) {
	res := o.db.Where("created_at < ?", before).Delete(&models.CheckinRecord{})
	return res.RowsAffected, res.Error
}
