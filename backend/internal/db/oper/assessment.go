package oper

import (
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// AssessmentOper 封装 AssessmentTask 表（站点考核快照，per site+task_name 唯一）。
type AssessmentOper struct{ db *gorm.DB }

// NewAssessmentOper 构造。
func NewAssessmentOper(db *gorm.DB) *AssessmentOper { return &AssessmentOper{db: db} }

// FindBySite 取某站最新一批考核任务（按 fetched_at 降序）。
func (o *AssessmentOper) FindBySite(siteID uint) ([]models.AssessmentTask, error) {
	var out []models.AssessmentTask
	err := o.db.Where("site_id = ?", siteID).Order("fetched_at DESC").Find(&out).Error
	return out, err
}

// LatestFetchedAt 取某站最新抓取时间（DB 缓存 TTL 判定用）。
func (o *AssessmentOper) LatestFetchedAt(siteID uint) (time.Time, error) {
	var t models.AssessmentTask
	err := o.db.Where("site_id = ?", siteID).Order("fetched_at DESC").First(&t).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	return t.FetchedAt, nil
}

// BatchUpsert 事务删后插（保持「每次抓取一份新鲜快照」语义）。
func (o *AssessmentOper) BatchUpsert(siteID uint, tasks []models.AssessmentTask, fetchedAt time.Time) error {
	return o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("site_id = ?", siteID).Delete(&models.AssessmentTask{}).Error; err != nil {
			return err
		}
		for i := range tasks {
			tasks[i].SiteID = siteID
			tasks[i].FetchedAt = fetchedAt
			if err := tx.Create(&tasks[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// CleanupExpired 删除超过 maxAge 的快照，返回删除条数。
func (o *AssessmentOper) CleanupExpired(maxAge time.Duration) (int64, error) {
	res := o.db.Where("fetched_at < ?", time.Now().Add(-maxAge)).Delete(&models.AssessmentTask{})
	return res.RowsAffected, res.Error
}
