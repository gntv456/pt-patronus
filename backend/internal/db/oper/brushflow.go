package oper

import (
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BrushFlowOper 持久化刷流任务与种子加入历史。
type BrushFlowOper struct{ db *gorm.DB }

func NewBrushFlowOper(db *gorm.DB) *BrushFlowOper { return &BrushFlowOper{db: db} }

func (o *BrushFlowOper) ListTasks() ([]models.BrushFlowTask, error) {
	var ts []models.BrushFlowTask
	err := o.db.Order("id DESC").Find(&ts).Error
	return ts, err
}

func (o *BrushFlowOper) ListEnabledTasks() ([]models.BrushFlowTask, error) {
	var ts []models.BrushFlowTask
	err := o.db.Where("enabled = ?", true).Find(&ts).Error
	return ts, err
}

func (o *BrushFlowOper) GetTask(id uint) (*models.BrushFlowTask, error) {
	var t models.BrushFlowTask
	if err := o.db.First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (o *BrushFlowOper) CreateTask(t *models.BrushFlowTask) error { return o.db.Create(t).Error }

// UpdateTask 保存任务编辑。统计/时间戳/错误为服务端管控字段（cron 与 Select/Sweep 累加），
// 不随编辑覆盖：用 Omit 排除，避免客户端伪造或字段缺失把统计清零。
func (o *BrushFlowOper) UpdateTask(t *models.BrushFlowTask) error {
	return o.db.Omit(
		"TotalAdded", "TotalDeleted", "TotalUpload",
		"CreatedAt", "LastSelectAt", "LastSweepAt", "LastError",
	).Save(t).Error
}

// DeleteTask 删任务并联删种子历史。
func (o *BrushFlowOper) DeleteTask(id uint) error {
	return o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("task_id = ?", id).Delete(&models.BrushFlowSeed{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.BrushFlowTask{}, id).Error
	})
}

func (o *BrushFlowOper) ListSeeds(taskID uint) ([]models.BrushFlowSeed, error) {
	var ss []models.BrushFlowSeed
	err := o.db.Where("task_id = ?", taskID).Order("id DESC").Limit(500).Find(&ss).Error
	return ss, err
}

func (o *BrushFlowOper) SeedExistsBySourceKey(taskID uint, key string) (bool, error) {
	var n int64
	err := o.db.Model(&models.BrushFlowSeed{}).Where("task_id = ? AND source_key = ?", taskID, key).Count(&n).Error
	return n > 0, err
}

// AddSeed 写入种子历史（幂等：task_id+source_key 唯一索引，冲突静默跳过——
// 并发/重试窗口内同一种子只落一条记录，配合 chain 层 SeedExists 去重双保险）。
func (o *BrushFlowOper) AddSeed(s *models.BrushFlowSeed) error {
	return o.db.Clauses(clause.OnConflict{DoNothing: true}).Create(s).Error
}

// IncrTaskStats 原子累加任务统计（added/deleted/upload 传 0 跳过该项）。
func (o *BrushFlowOper) IncrTaskStats(id uint, added, deleted, upload int64) error {
	updates := map[string]any{}
	if added != 0 {
		updates["total_added"] = gorm.Expr("total_added + ?", added)
	}
	if deleted != 0 {
		updates["total_deleted"] = gorm.Expr("total_deleted + ?", deleted)
	}
	if upload != 0 {
		updates["total_upload"] = gorm.Expr("total_upload + ?", upload)
	}
	if len(updates) == 0 {
		return nil
	}
	return o.db.Model(&models.BrushFlowTask{}).Where("id = ?", id).Updates(updates).Error
}

func (o *BrushFlowOper) TouchTask(id uint, field string) error {
	return o.db.Model(&models.BrushFlowTask{}).Where("id = ?", id).Update(field, time.Now()).Error
}

func (o *BrushFlowOper) SetTaskError(id uint, msg string) error {
	return o.db.Model(&models.BrushFlowTask{}).Where("id = ?", id).Update("last_error", msg).Error
}
