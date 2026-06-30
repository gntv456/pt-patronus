package oper

import (
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// MedalOper 封装 Medal 表（站点勋章快照，per site+medal_id 唯一）。
// 照 AssessmentOper 范式：3 级缓存的 DB 层，BatchUpsert 为「删后插」保持每次抓取一份新鲜快照。
type MedalOper struct{ db *gorm.DB }

// NewMedalOper 构造。
func NewMedalOper(db *gorm.DB) *MedalOper { return &MedalOper{db: db} }

// FindBySite 取某站最新一批勋章（按 fetched_at 降序）。
func (o *MedalOper) FindBySite(siteID uint) ([]models.Medal, error) {
	var out []models.Medal
	err := o.db.Where("site_id = ?", siteID).Order("fetched_at DESC").Find(&out).Error
	return out, err
}

// LatestFetchedAt 取某站最新抓取时间（DB 缓存 TTL 判定用）。
func (o *MedalOper) LatestFetchedAt(siteID uint) (time.Time, error) {
	var m models.Medal
	err := o.db.Where("site_id = ?", siteID).Order("fetched_at DESC").First(&m).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	return m.FetchedAt, nil
}

// BatchUpsert 事务删后插（保持「每次抓取一份新鲜快照」语义）。
func (o *MedalOper) BatchUpsert(siteID uint, medals []models.Medal, fetchedAt time.Time) error {
	return o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("site_id = ?", siteID).Delete(&models.Medal{}).Error; err != nil {
			return err
		}
		for i := range medals {
			medals[i].SiteID = siteID
			medals[i].FetchedAt = fetchedAt
			if err := tx.Create(&medals[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// CleanupExpired 删除超过 maxAge 的快照，返回删除条数。
func (o *MedalOper) CleanupExpired(maxAge time.Duration) (int64, error) {
	res := o.db.Where("fetched_at < ?", time.Now().Add(-maxAge)).Delete(&models.Medal{})
	return res.RowsAffected, res.Error
}
