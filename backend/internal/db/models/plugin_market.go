// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package models

import "time"

// ---- 插件市场运营层 ----

// PluginInstall 安装事件（百鸟朝凤"老树洞"= 累计安装量 Top30；"蹦蹦新窝"= 最近上架/新安装）。
// 每次 Install 成功记一条；老树洞计数 = 该插件累计安装事件数（跨用户社区榜在中心仓库侧，
// 本地表支持单机多装/重装计数与"本站安装量"榜）。
type PluginInstall struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	PluginID  string    `gorm:"size:96;index:idx_plugin_install,priority:1;not null" json:"plugin_id"`
	Version   string    `gorm:"size:64" json:"version,omitempty"`
	Source    string    `gorm:"size:512" json:"source,omitempty"`
	CreatedAt time.Time `gorm:"index:idx_plugin_install,priority:2" json:"created_at"`
}

// PluginFavorite 插件收藏（百宝袋）。UserID=0 视为单用户模式（当前部署形态）。
type PluginFavorite struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	PluginID  string    `gorm:"size:96;index:idx_plugin_fav,priority:2;not null" json:"plugin_id"`
	UserID    uint      `gorm:"index:idx_plugin_fav,priority:1;default:0" json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

// PluginInstance 插件实例（多实例「分神」）：同一插件 ID 的多份独立配置。
// InstanceKey 形如 "主实例"（默认，plugin_id 原值语义）/ "分身-签到2"。
// 运行时侧单实例插件（系统备份等）由 Manifest.SingleInstance 声明禁止建分身。
type PluginInstance struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	PluginID    string    `gorm:"size:96;index:idx_plugin_inst,priority:1;not null" json:"plugin_id"`
	InstanceKey string    `gorm:"size:96;index:idx_plugin_inst,priority:2;not null" json:"instance_key"` // 展示名（同一 plugin 内唯一）
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	ConfigJSON  string    `gorm:"type:text;column:config_json" json:"-"`
	SortOrder   int       `gorm:"index" json:"sort_order"` // 拖拽排序（天序）；小值在前
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PluginInstallCountRow 百鸟朝凤行（老树洞/蹦蹦新窝共用）。
// FirstInstalled 以字符串承载（SQLite MIN(created_at) 聚合返回字符串，避免驱动 Scan 时间类型差异）。
type PluginInstallCountRow struct {
	PluginID       string `json:"plugin_id"`
	Installs       int64  `json:"installs"`
	FirstInstalled string `json:"first_installed"` // 老树洞=最早安装；蹦蹦新窝=最近一次安装（ISO 文本）
}

// ChartRow 百鸟朝凤 API 行。
type ChartRow struct {
	PluginID      string    `json:"plugin_id"`
	Installs      int64     `json:"installs"`
	LastInstalled time.Time `json:"last_installed,omitempty"`
}

// MarketChartsView 百鸟朝凤视图（老树洞 + 蹦蹦新窝）。
type MarketChartsView struct {
	Disciples []ChartRow `json:"disciples"` // 老树洞：累计安装量 Top30
	Newcomers []ChartRow `json:"newcomers"` // 蹦蹦新窝：最近安装 Top30
}
