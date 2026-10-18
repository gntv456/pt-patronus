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

// CountLogs 插件日志总数（筛选与 ListLogs 一致）。单页封顶 500：只回被切过的一页时，
// 「这插件到底跑了多少行日志」在界面上无法回答，排障时被挤掉的老日志等于不存在。
func (o *PluginOper) CountLogs(pluginID string) (int64, error) {
	q := o.db.Model(&models.PluginLog{})
	if pluginID != "" {
		q = q.Where("plugin_id = ?", pluginID)
	}
	var n int64
	err := q.Count(&n).Error
	return n, err
}

const (
	PluginLogListDefault = 100
	PluginLogListLimit   = 500
)

// NormalizePluginLogLimit 留空/非法取默认，超上限取上限（旧写法把超限折回默认值）。
func NormalizePluginLogLimit(n int) int {
	if n <= 0 {
		return PluginLogListDefault
	}
	if n > PluginLogListLimit {
		return PluginLogListLimit
	}
	return n
}

func (o *PluginOper) ListLogs(pluginID string, limit int) ([]models.PluginLog, error) {
	limit = NormalizePluginLogLimit(limit)
	var logs []models.PluginLog
	q := o.db.Order("created_at DESC").Limit(limit)
	if pluginID != "" {
		q = q.Where("plugin_id = ?", pluginID)
	}
	err := q.Find(&logs).Error
	return logs, err
}

// PruneLogs 清理 before 之前的插件日志（每次调度写一行，无 TTL 会无限膨胀——数据审计 P2）。
func (o *PluginOper) PruneLogs(before time.Time) (int64, error) {
	res := o.db.Where("created_at < ?", before).Delete(&models.PluginLog{})
	return res.RowsAffected, res.Error
}

func (o *PluginOper) UpsertState(st *models.PluginState) error {
	return o.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"installed", "config_json", "updated_at"}),
	}).Create(st).Error
}

func (o *PluginOper) CreateRelease(rel *models.PluginRelease) error {
	if rel.CreatedAt.IsZero() {
		rel.CreatedAt = time.Now()
	}
	if rel.Active {
		if err := o.db.Model(&models.PluginRelease{}).Where("plugin_id = ?", rel.PluginID).Update("active", false).Error; err != nil {
			return err
		}
	}
	return o.db.Create(rel).Error
}

func (o *PluginOper) ListReleases(pluginID string, limit int) ([]models.PluginRelease, error) {
	if limit <= 0 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}
	var releases []models.PluginRelease
	err := o.db.Where("plugin_id = ?", pluginID).Order("created_at DESC").Limit(limit).Find(&releases).Error
	return releases, err
}

func (o *PluginOper) GetRelease(id uint) (*models.PluginRelease, error) {
	var rel models.PluginRelease
	if err := o.db.First(&rel, id).Error; err != nil {
		return nil, err
	}
	return &rel, nil
}

func (o *PluginOper) SetActiveRelease(pluginID string, releaseID uint) error {
	return o.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.PluginRelease{}).Where("plugin_id = ?", pluginID).Update("active", false).Error; err != nil {
			return err
		}
		return tx.Model(&models.PluginRelease{}).Where("id = ? AND plugin_id = ?", releaseID, pluginID).Update("active", true).Error
	})
}

func (o *PluginOper) ActiveRelease(pluginID string) (*models.PluginRelease, error) {
	var rel models.PluginRelease
	if err := o.db.Where("plugin_id = ? AND active = ?", pluginID, true).Order("created_at DESC").First(&rel).Error; err != nil {
		return nil, err
	}
	return &rel, nil
}

// PluginKVOper persists per-plugin key-value runtime state.
type PluginKVOper struct {
	db *gorm.DB
}

func NewPluginKVOper(db *gorm.DB) *PluginKVOper {
	return &PluginKVOper{db: db}
}

// KVGet reads a key into out (JSON-decoded). Returns gorm.ErrRecordNotFound if absent.
func (o *PluginKVOper) KVGet(pluginID, key string, out any) error {
	var kv models.PluginKV
	err := o.db.Where("plugin_id = ? AND key = ?", pluginID, key).First(&kv).Error
	if err != nil {
		return err
	}
	if out == nil || kv.ValueJSON == "" {
		return nil
	}
	return json.Unmarshal([]byte(kv.ValueJSON), out)
}

// KVSet upserts a key with JSON-encoded val.
func (o *PluginKVOper) KVSet(pluginID, key string, val any) error {
	raw, err := json.Marshal(val)
	if err != nil {
		return err
	}
	kv := models.PluginKV{PluginID: pluginID, Key: key, ValueJSON: string(raw), UpdatedAt: time.Now()}
	return o.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "plugin_id"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value_json", "updated_at"}),
	}).Create(&kv).Error
}

// KVDelete removes a key. Idempotent.
func (o *PluginKVOper) KVDelete(pluginID, key string) error {
	return o.db.Where("plugin_id = ? AND key = ?", pluginID, key).Delete(&models.PluginKV{}).Error
}
