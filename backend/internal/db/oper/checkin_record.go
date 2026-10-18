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
// Count 签到记录总数（siteID=0 不过滤，与 List 同筛选）。
// List 有单页上限且支持 offset，只回被切过的一页时，界面分不清「翻完了」和「到这页为止」。
func (o *CheckinRecordOper) Count(siteID uint) (int64, error) {
	q := o.db.Model(&models.CheckinRecord{})
	if siteID > 0 {
		q = q.Where("site_id = ?", siteID)
	}
	var n int64
	err := q.Count(&n).Error
	return n, err
}

func (o *CheckinRecordOper) Create(r *models.CheckinRecord) error {
	return o.db.Create(r).Error
}

// List 列签到记录（按时间倒序）。siteID=0 不过滤；limit<=0 默认 100。
const (
	CheckinListDefault = 100
	CheckinListLimit   = 1000
)

func NormalizeCheckinLimit(n int) int {
	if n <= 0 {
		return CheckinListDefault
	}
	if n > CheckinListLimit {
		return CheckinListLimit
	}
	return n
}

func (o *CheckinRecordOper) List(siteID uint, limit, offset int) ([]models.CheckinRecord, error) {
	limit = NormalizeCheckinLimit(limit)
	if offset < 0 { // 负 offset 传 GORM 触发 MySQL 500，钳到 0
		offset = 0
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
