// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// WorkflowOper 封装 Workflow 表的读写（CRUD + 状态机）。
type WorkflowOper struct {
	db *gorm.DB
}

func NewWorkflowOper(db *gorm.DB) *WorkflowOper { return &WorkflowOper{db: db} }

// List 全部工作流（按更新时间倒序）。
func (o *WorkflowOper) List() ([]models.Workflow, error) {
	var ws []models.Workflow
	err := o.db.Order("updated_at DESC").Find(&ws).Error
	return ws, err
}

// ListEnabled 仅启用（Enabled=true）的工作流，供启动时注册触发器。
func (o *WorkflowOper) ListEnabled() ([]models.Workflow, error) {
	var ws []models.Workflow
	err := o.db.Where("enabled = ?", true).Find(&ws).Error
	return ws, err
}

// Get 单条。
func (o *WorkflowOper) Get(id uint) (*models.Workflow, error) {
	var w models.Workflow
	if err := o.db.First(&w, id).Error; err != nil {
		return nil, err
	}
	return &w, nil
}

// Create 新建。
func (o *WorkflowOper) Create(w *models.Workflow) error { return o.db.Create(w).Error }

// Update 保存。
func (o *WorkflowOper) Update(w *models.Workflow) error { return o.db.Save(w).Error }

// Delete 删除工作流及其运行记录（触发器注销由 chain 负责）。
func (o *WorkflowOper) Delete(id uint) error {
	return o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("workflow_id = ?", id).Delete(&models.WorkflowRun{}).Error; err != nil {
			return err
		}
		return tx.Delete(&models.Workflow{}, id).Error
	})
}

// SetState 更新执行状态（idle/running/success/failed）。用 map 形式避免零值写坑。
func (o *WorkflowOper) SetState(id uint, state string) error {
	return o.db.Model(&models.Workflow{}).Where("id = ?", id).
		Update("state", state).Error
}

// ResetStuckRunning 把所有 state=running 的工作流重置为 failed（启动恢复：
// 进程崩溃/重启前未写终态的，会被重入守卫永久挡死，需人工 Reset）。
func (o *WorkflowOper) ResetStuckRunning() error {
	return o.db.Model(&models.Workflow{}).Where("state = ?", "running").
		Update("state", "failed").Error
}

func (o *WorkflowOper) ResetState(id uint) error {
	res := o.db.Model(&models.Workflow{}).Where("id = ?", id).Updates(map[string]any{
		"state": "idle", "last_result": "",
	})
	if res.Error != nil {
		return res.Error
	}
	// 「重置」对一条不存在的工作流是空操作：报 404，别让人以为卡住的状态已被清掉。
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// MarkRun 记录一次执行结果：状态、运行数+1、上次时间、上次结果。
func (o *WorkflowOper) MarkRun(id uint, state, result string) error {
	now := time.Now()
	return o.db.Model(&models.Workflow{}).Where("id = ?", id).
		Updates(map[string]any{
			"state":       state,
			"run_count":   gorm.Expr("run_count + 1"),
			"last_run_at": now,
			"last_result": result,
		}).Error
}

// WorkflowRunOper 封装 WorkflowRun 表读写。
type WorkflowRunOper struct {
	db *gorm.DB
}

func NewWorkflowRunOper(db *gorm.DB) *WorkflowRunOper { return &WorkflowRunOper{db: db} }

// Create 新建运行记录。
func (o *WorkflowRunOper) Create(r *models.WorkflowRun) error { return o.db.Create(r).Error }

// Update 保存运行记录。
func (o *WorkflowRunOper) Update(r *models.WorkflowRun) error { return o.db.Save(r).Error }

// Get 单条运行记录。
func (o *WorkflowRunOper) Get(id uint) (*models.WorkflowRun, error) {
	var r models.WorkflowRun
	if err := o.db.First(&r, id).Error; err != nil {
		return nil, err
	}
	return &r, nil
}

// ListByWorkflow 某工作流的运行历史（倒序，限 limit）。
func (o *WorkflowRunOper) ListByWorkflow(workflowID uint, limit int) ([]models.WorkflowRun, error) {
	var rs []models.WorkflowRun
	q := o.db.Where("workflow_id = ?", workflowID).Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&rs).Error
	return rs, err
}

const (
	RunListDefault = 20
	RunListLimit   = 500
)

// NormalizeRunLimit 留空/非法取默认，超上限取上限（handler 回显生效值，
// 否则 ?limit=999999 的调用方以为自己拿到了全部历史）。
func NormalizeRunLimit(n int) int {
	if n <= 0 {
		return RunListDefault
	}
	if n > RunListLimit {
		return RunListLimit
	}
	return n
}

// CountByWorkflow 某工作流的运行总数。筛选与 ListByWorkflow 完全一致
// （含 workflowID=0 时同样按等值匹配），否则 total 与清单不同源、截断判断会失真。
func (o *WorkflowRunOper) CountByWorkflow(workflowID uint) (int64, error) {
	var n int64
	err := o.db.Model(&models.WorkflowRun{}).Where("workflow_id = ?", workflowID).Count(&n).Error
	return n, err
}

// ListRecent 全局最近运行（跨工作流，倒序，供统计看板）。
func (o *WorkflowRunOper) ListRecent(limit int) ([]models.WorkflowRun, error) {
	var rs []models.WorkflowRun
	q := o.db.Order("created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&rs).Error
	return rs, err
}

// Cleanup 删除指定时间前的运行记录（返回删除数）。
func (o *WorkflowRunOper) Cleanup(before time.Time) (int64, error) {
	res := o.db.Where("created_at < ?", before).Delete(&models.WorkflowRun{})
	return res.RowsAffected, res.Error
}
