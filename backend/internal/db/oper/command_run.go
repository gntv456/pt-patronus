// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// CommandRunOper stores audited ops command executions.
type CommandRunOper struct {
	db *gorm.DB
}

func NewCommandRunOper(db *gorm.DB) *CommandRunOper {
	return &CommandRunOper{db: db}
}

func (o *CommandRunOper) Create(run *models.OpsCommandRun) error {
	return o.db.Create(run).Error
}

func (o *CommandRunOper) AddTargetResult(result *models.OpsCommandRunTarget) error {
	return o.db.Create(result).Error
}

func (o *CommandRunOper) Finish(id uint, status string, success, failure int, finished time.Time, durationMS int64) error {
	return o.db.Model(&models.OpsCommandRun{}).Where("id = ?", id).Updates(map[string]any{
		"status":        status,
		"success_count": success,
		"failure_count": failure,
		"finished_at":   finished,
		"duration_ms":   durationMS,
	}).Error
}

const (
	CommandRunListDefault = 30
	CommandRunListLimit   = 100
)

// NormalizeCommandRunLimit 留空/非法取默认，超上限取上限。
// 旧写法把 `limit>100` 一并退回 30：要 500 条的人只拿到 30 条还看不出来。
func NormalizeCommandRunLimit(n int) int {
	if n <= 0 {
		return CommandRunListDefault
	}
	if n > CommandRunListLimit {
		return CommandRunListLimit
	}
	return n
}

func (o *CommandRunOper) List(source string, limit int) ([]models.OpsCommandRun, error) {
	q := o.db.Order("id desc").Limit(NormalizeCommandRunLimit(limit))
	if source != "" {
		q = q.Where("source = ?", source)
	}
	var runs []models.OpsCommandRun
	err := q.Find(&runs).Error
	return runs, err
}

// Count 命令批次总数（筛选与 List 一致）。审计台账只回最近 30 条时，
// 「这一批是不是全在这里」必须能回答，否则翻不到的失败批次等于不存在。
func (o *CommandRunOper) Count(source string) (int64, error) {
	q := o.db.Model(&models.OpsCommandRun{})
	if source != "" {
		q = q.Where("source = ?", source)
	}
	var n int64
	err := q.Count(&n).Error
	return n, err
}

func (o *CommandRunOper) Get(id uint) (*models.OpsCommandRun, error) {
	var run models.OpsCommandRun
	err := o.db.Preload("Targets", func(db *gorm.DB) *gorm.DB {
		return db.Order("id asc")
	}).First(&run, id).Error
	if err != nil {
		return nil, err
	}
	return &run, nil
}
