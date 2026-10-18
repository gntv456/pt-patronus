package oper

import (
	"errors"
	"fmt"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// PluginMarketOper 插件市场运营层：安装量榜（百鸟朝凤）/收藏（百宝袋）/多实例（分神）/排序（天序）。
type PluginMarketOper struct{ db *gorm.DB }

func NewPluginMarketOper(db *gorm.DB) *PluginMarketOper { return &PluginMarketOper{db: db} }

// ---- 安装量（老树洞） ----

// RecordInstall 记一次安装事件（百鸟朝凤数据源）。
func (o *PluginMarketOper) RecordInstall(pluginID, version, source string) error {
	if pluginID == "" {
		return errors.New("plugin id required")
	}
	return o.db.Create(&models.PluginInstall{PluginID: pluginID, Version: version, Source: source, CreatedAt: time.Now()}).Error
}

// InstallCount 单插件累计安装次数。
func (o *PluginMarketOper) InstallCount(pluginID string) (int64, error) {
	var n int64
	err := o.db.Model(&models.PluginInstall{}).Where("plugin_id = ?", pluginID).Count(&n).Error
	return n, err
}

// TopInstalled 老树洞：按累计安装次数降序 Top N（并列按最早安装在前）。
func (o *PluginMarketOper) TopInstalled(n int) ([]models.PluginInstallCountRow, error) {
	if n <= 0 || n > 100 {
		n = 30
	}
	var rows []models.PluginInstallCountRow
	err := o.db.Model(&models.PluginInstall{}).
		Select("plugin_id, COUNT(*) AS installs, MIN(created_at) AS first_installed").
		Group("plugin_id").Order("installs DESC, first_installed ASC").Limit(n).
		Scan(&rows).Error
	return rows, err
}

// RecentInstalls 蹦蹦新窝：最近安装事件（去重插件）按时间倒序 Top N。
// 与老树洞互补——新上架/刚被发现的插件即使总量低也能露脸。
// 两步查询：先取每插件最新事件（GROUP BY 取 MAX(id)），再回表 JOIN 出计数（SQLite 窗口函数支持不稳）。
func (o *PluginMarketOper) RecentInstalls(n int) ([]models.PluginInstallCountRow, error) {
	if n <= 0 || n > 100 {
		n = 30
	}
	var latest []models.PluginInstall
	err := o.db.Model(&models.PluginInstall{}).
		Where("id IN (SELECT MAX(id) FROM plugin_installs GROUP BY plugin_id)").
		Order("created_at DESC, id DESC").Limit(n).Find(&latest).Error
	if err != nil {
		return nil, err
	}
	counts := map[string]int64{}
	if len(latest) > 0 {
		ids := make([]string, 0, len(latest))
		for _, l := range latest {
			ids = append(ids, l.PluginID)
		}
		var cntRows []models.PluginInstallCountRow
		if err := o.db.Model(&models.PluginInstall{}).
			Select("plugin_id, COUNT(*) AS installs, '' AS first_installed").
			Where("plugin_id IN ?", ids).Group("plugin_id").Scan(&cntRows).Error; err != nil {
			return nil, err
		}
		for _, r := range cntRows {
			counts[r.PluginID] = r.Installs
		}
	}
	rows := make([]models.PluginInstallCountRow, 0, len(latest))
	for _, l := range latest {
		rows = append(rows, models.PluginInstallCountRow{
			PluginID: l.PluginID, Installs: counts[l.PluginID],
			FirstInstalled: l.CreatedAt.Format(time.RFC3339Nano),
		})
	}
	return rows, nil
}

// ---- 收藏（百宝袋） ----

// Favorite 收藏插件（幂等；已收藏返回 nil）。
func (o *PluginMarketOper) Favorite(userID uint, pluginID string) error {
	var exist models.PluginFavorite
	err := o.db.Where("user_id = ? AND plugin_id = ?", userID, pluginID).First(&exist).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return o.db.Create(&models.PluginFavorite{UserID: userID, PluginID: pluginID, CreatedAt: time.Now()}).Error
}

// Unfavorite 取消收藏（幂等）。
func (o *PluginMarketOper) Unfavorite(userID uint, pluginID string) error {
	return o.db.Where("user_id = ? AND plugin_id = ?", userID, pluginID).
		Delete(&models.PluginFavorite{}).Error
}

// Favorites 列出收藏的插件 id（按收藏时间倒序）。
func (o *PluginMarketOper) Favorites(userID uint) ([]string, error) {
	var favs []models.PluginFavorite
	err := o.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&favs).Error
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(favs))
	for _, f := range favs {
		ids = append(ids, f.PluginID)
	}
	return ids, nil
}

// IsFavorite 单插件是否已收藏。
func (o *PluginMarketOper) IsFavorite(userID uint, pluginID string) bool {
	var n int64
	o.db.Model(&models.PluginFavorite{}).Where("user_id = ? AND plugin_id = ?", userID, pluginID).Count(&n)
	return n > 0
}

// ---- 多实例（分神） ----

// CreateInstance 建分身实例（同 plugin 内 instance_key 查重；默认实例 key="default"）。
func (o *PluginMarketOper) CreateInstance(pluginID, instanceKey string, configJSON string) (*models.PluginInstance, error) {
	if pluginID == "" || instanceKey == "" {
		return nil, errors.New("plugin id / instance key required")
	}
	var n int64
	// 查重失败绝不能当「不存在」：idx_plugin_inst 只是普通索引（非唯一），
	// 库里没有任何东西兜住「同一 plugin 内 instance_key 唯一」这条不变量，
	// 一旦放行就落下同名实例，之后按 (plugin_id, instance_key) 取配置会随机命中其一。
	if err := o.db.Model(&models.PluginInstance{}).Where("plugin_id = ? AND instance_key = ?", pluginID, instanceKey).Count(&n).Error; err != nil {
		return nil, fmt.Errorf("查重失败（未创建，避免落下同名实例）: %w", err)
	}
	if n > 0 {
		return nil, errors.New("实例名已存在（同一插件内实例名唯一）")
	}
	// SortOrder 追加到末尾（max+1）。取最大值失败同样中止：静默当 0 会让新实例插到队首，
	// 把用户拖好的「天序」顺序打乱。
	var maxOrder int
	if err := o.db.Model(&models.PluginInstance{}).Where("plugin_id = ?", pluginID).
		Select("COALESCE(MAX(sort_order), 0)").Scan(&maxOrder).Error; err != nil {
		return nil, fmt.Errorf("取排序位失败（未创建）: %w", err)
	}
	inst := models.PluginInstance{PluginID: pluginID, InstanceKey: instanceKey,
		Enabled: true, ConfigJSON: configJSON, SortOrder: maxOrder + 1}
	if err := o.db.Create(&inst).Error; err != nil {
		return nil, err
	}
	return &inst, nil
}

// Instances 列出某插件全部实例（按天序 sort_order 升序）。
func (o *PluginMarketOper) Instances(pluginID string) ([]models.PluginInstance, error) {
	var insts []models.PluginInstance
	err := o.db.Where("plugin_id = ?", pluginID).Order("sort_order ASC, id ASC").Find(&insts).Error
	return insts, err
}

// DeleteInstance 删实例（幂等）。
func (o *PluginMarketOper) DeleteInstance(pluginID, instanceKey string) error {
	return o.db.Where("plugin_id = ? AND instance_key = ?", pluginID, instanceKey).
		Delete(&models.PluginInstance{}).Error
}

// GetInstance 按 (pluginID, instanceKey) 取单个分身实例（删除前的存在性判定用）。
// 为什么不改 DeleteInstance 的返回契约：它的「再删一次也回 nil」是被测语义（插件目录清理
// 与卸载级联会重复调用），存在性由调用方在这里查。
func (o *PluginMarketOper) GetInstance(pluginID, instanceKey string) (*models.PluginInstance, error) {
	var it models.PluginInstance
	if err := o.db.Where("plugin_id = ? AND instance_key = ?", pluginID, instanceKey).First(&it).Error; err != nil {
		return nil, err
	}
	return &it, nil
}

// SaveInstanceConfig 更新实例配置/启用态。
func (o *PluginMarketOper) SaveInstanceConfig(pluginID, instanceKey, configJSON string, enabled bool) error {
	return o.db.Model(&models.PluginInstance{}).
		Where("plugin_id = ? AND instance_key = ?", pluginID, instanceKey).
		Updates(map[string]any{"config_json": configJSON, "enabled": enabled, "updated_at": time.Now()}).Error
}

// ---- 天序（拖拽排序，服务端持久化 → 跨端同步） ----

// ReorderInstances 按给定实例 id 顺序重写 sort_order（前端拖拽后整表提交）。
func (o *PluginMarketOper) ReorderInstances(pluginID string, ids []uint) error {
	return o.db.Transaction(func(tx *gorm.DB) error {
		for i, id := range ids {
			if err := tx.Model(&models.PluginInstance{}).
				Where("id = ? AND plugin_id = ?", id, pluginID).
				Update("sort_order", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
