// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

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
// 先验存在性：GORM 的 Save 在「主键非零但库里没这条」时会 INSERT——
// PUT 一个不存在的任务号会凭空造出一条任务（实测 id=999999 落库后 GET 返回全零记录）。
func (o *BrushFlowOper) UpdateTask(t *models.BrushFlowTask) error {
	var n int64
	if err := o.db.Model(&models.BrushFlowTask{}).Where("id = ?", t.ID).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return gorm.ErrRecordNotFound
	}
	return o.db.Omit(
		"TotalAdded", "TotalDeleted", "TotalUpload",
		"CreatedAt", "LastSelectAt", "LastSweepAt", "LastError", "LastErrorFrom",
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

// SeedListLimit 种子历史单页上限：长期任务种得多了，老记录会被这道窗口挤出去。
const SeedListLimit = 500

func (o *BrushFlowOper) ListSeeds(taskID uint) ([]models.BrushFlowSeed, error) {
	var ss []models.BrushFlowSeed
	err := o.db.Where("task_id = ?", taskID).Order("id DESC").Limit(SeedListLimit).Find(&ss).Error
	return ss, err
}

// CountSeeds 该任务的种子历史总数（筛选与 ListSeeds 一致）。
// 只回被截断的一页时，界面就把 500 条画成「这个任务就加过这些种」，
// 更早加入、可能仍在做种或删除中的那些在界面上凭空消失。
func (o *BrushFlowOper) CountSeeds(taskID uint) (int64, error) {
	var n int64
	err := o.db.Model(&models.BrushFlowSeed{}).Where("task_id = ?", taskID).Count(&n).Error
	return n, err
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

// SetTaskError 记录任务运行失败。op（"select"/"sweep"）标记写入方：成功后各自只清同源错误
// （见 ClearTaskError），防止对方的成功运行擦掉本侧诊断。
func (o *BrushFlowOper) SetTaskError(id uint, op, msg string) error {
	return o.db.Model(&models.BrushFlowTask{}).Where("id = ?", id).
		Updates(map[string]any{"last_error": msg, "last_error_from": op}).Error
}

// ClearTaskError 运行成功时清除诊断：仅当错误来源为 op 或空（升级前遗留无来源标记）才清。
// 选种每 10 分钟、扫删每 5 分钟交错运行，无条件清空会让诊断在几分钟内被另一方擦掉。
func (o *BrushFlowOper) ClearTaskError(id uint, op string) error {
	return o.db.Model(&models.BrushFlowTask{}).
		Where("id = ? AND (last_error_from = ? OR last_error_from = '')", id, op).
		Updates(map[string]any{"last_error": "", "last_error_from": ""}).Error
}
