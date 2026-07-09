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

func (o *CommandRunOper) List(source string, limit int) ([]models.OpsCommandRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	q := o.db.Order("id desc").Limit(limit)
	if source != "" {
		q = q.Where("source = ?", source)
	}
	var runs []models.OpsCommandRun
	err := q.Find(&runs).Error
	return runs, err
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
