package oper

import (
	"encoding/json"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PluginOper persists plugin state and operational logs.
type PluginOper struct {
	db *gorm.DB
}

func NewPluginOper(db *gorm.DB) *PluginOper {
	return &PluginOper{db: db}
}

func (o *PluginOper) EnsureState(id string, config map[string]any) (*models.PluginState, error) {
	var st models.PluginState
	err := o.db.First(&st, "id = ?", id).Error
	if err == nil {
		return &st, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	raw, _ := json.Marshal(config)
	st = models.PluginState{
		ID:         id,
		Installed:  true,
		Enabled:    false,
		ConfigJSON: string(raw),
	}
	if err := o.db.Create(&st).Error; err != nil {
		return nil, err
	}
	return &st, nil
}

func (o *PluginOper) GetState(id string) (*models.PluginState, error) {
	var st models.PluginState
	if err := o.db.First(&st, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &st, nil
}

func (o *PluginOper) SaveState(st *models.PluginState) error {
	return o.db.Save(st).Error
}

func (o *PluginOper) SetEnabled(id string, enabled bool) error {
	return o.db.Model(&models.PluginState{}).Where("id = ?", id).Update("enabled", enabled).Error
}

func (o *PluginOper) MarkEvent(id string, eventAt time.Time, lastErr string) error {
	return o.db.Model(&models.PluginState{}).Where("id = ?", id).Updates(map[string]any{
		"last_event_at": eventAt,
		"last_error":    lastErr,
	}).Error
}

func (o *PluginOper) CreateLog(l *models.PluginLog) error {
	if l.CreatedAt.IsZero() {
		l.CreatedAt = time.Now()
	}
	return o.db.Create(l).Error
}

func (o *PluginOper) ListLogs(pluginID string, limit int) ([]models.PluginLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var logs []models.PluginLog
	q := o.db.Order("created_at DESC").Limit(limit)
	if pluginID != "" {
		q = q.Where("plugin_id = ?", pluginID)
	}
	err := q.Find(&logs).Error
	return logs, err
}

func (o *PluginOper) UpsertState(st *models.PluginState) error {
	return o.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"installed", "config_json", "updated_at"}),
	}).Create(st).Error
}
