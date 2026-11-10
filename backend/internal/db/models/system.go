// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Code split from models.go. Package models defines GORM models for the persistence layer.
package models

import "time"

// SystemConfig — 运行时可变配置（key/value）。
type SystemConfig struct {
	Key       string    `gorm:"primaryKey;size:128" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PluginState stores runtime state and user config for a registered plugin.
type PluginState struct {
	ID          string     `gorm:"primaryKey;size:96" json:"id"`
	Enabled     bool       `gorm:"default:false;index" json:"enabled"`
	Installed   bool       `gorm:"default:true;index" json:"installed"`
	ConfigJSON  string     `gorm:"type:text;column:config_json" json:"-"`
	LastError   string     `gorm:"type:text" json:"last_error,omitempty"`
	LastEventAt *time.Time `json:"last_event_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// PluginLog records plugin events, actions, and operational messages.
type PluginLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	PluginID  string    `gorm:"size:96;index;not null" json:"plugin_id"`
	Level     string    `gorm:"size:16;index" json:"level"`
	Event     string    `gorm:"size:96;index" json:"event"`
	Message   string    `gorm:"type:text" json:"message"`
	DataJSON  string    `gorm:"type:text;column:data_json" json:"-"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// PluginKV stores per-plugin key-value state (失败消息队列、调度锚点等)。
// 撬动多插件的持久化需求——插件 Runtime 通过 KVGet/KVSet/KVDelete 读写自己的运行时状态，
// 不必把运行时数据塞进用户可见的 ConfigJSON。
type PluginKV struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	PluginID  string    `gorm:"size:96;uniqueIndex:idx_plugin_kv,priority:1;not null" json:"plugin_id"`
	Key       string    `gorm:"size:128;uniqueIndex:idx_plugin_kv,priority:2;not null" json:"key"`
	ValueJSON string    `gorm:"type:text" json:"-"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PluginRelease stores install, upgrade, and rollback history for external plugins.
type PluginRelease struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	PluginID    string    `gorm:"size:96;index;not null" json:"plugin_id"`
	Version     string    `gorm:"size:64;index;not null" json:"version"`
	Action      string    `gorm:"size:16;index;not null" json:"action"`
	Source      string    `gorm:"size:512" json:"source,omitempty"`
	Archive     string    `gorm:"size:512" json:"archive,omitempty"`
	SHA256      string    `gorm:"size:128" json:"sha256,omitempty"`
	SnapshotDir string    `gorm:"size:512;not null" json:"snapshot_dir"`
	Active      bool      `gorm:"default:false;index" json:"active"`
	CreatedAt   time.Time `gorm:"index" json:"created_at"`
}

// PosterCache — 海报/元数据缓存（避免重复打 TMDB/豆瓣/MusicBrainz）。
type PosterCache struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CacheKey  string    `gorm:"uniqueIndex;size:128;not null" json:"cache_key"` // source:tmdbid / title|year|type 哈希
	Source    string    `gorm:"size:32;index" json:"source"`                    // tmdb | douban | musicbrainz
	SourceID  string    `gorm:"size:64" json:"source_id"`
	Title     string    `gorm:"size:255" json:"title"`
	Year      int       `json:"year"`
	Type      string    `gorm:"size:16" json:"type"` // movie | tv | music
	PosterURL string    `gorm:"size:512" json:"poster_url"`
	Overview  string    `gorm:"type:text" json:"overview"`
	RawJSON   string    `gorm:"type:text" json:"-"` // 原始元数据 JSON
	HitCount  int       `gorm:"default:0" json:"hit_count"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedAt time.Time `json:"created_at"`
}

// FileStorage — 网盘/存储后端配置。
type FileStorage struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:64;not null" json:"name"`
	Type        string    `gorm:"size:16;not null" json:"type"` // webdav | alist | smb | agent | local
	URL         string    `gorm:"size:255" json:"url"`
	Username    string    `gorm:"size:64" json:"username"`
	PasswordEnc string    `gorm:"type:text" json:"-"`
	BasePath    string    `gorm:"size:255" json:"base_path"`
	AgentID     uint      `gorm:"index" json:"agent_id"` // type=agent 时关联 Agent 表；其他类型忽略
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ActivationCode — legacy 激活码（好学 VIP 准入时代产物，端点保留但不再作门禁）。
type ActivationCode struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Code      string     `gorm:"uniqueIndex;size:128;not null" json:"code"`
	DeviceID  string     `gorm:"size:128;index" json:"device_id"` // 绑定设备（可空）
	VipLevel  int        `gorm:"default:1" json:"vip_level"`
	UsedAt    *time.Time `json:"used_at"`
	ExpiresAt *time.Time `json:"expires_at"`
	Revoked   bool       `gorm:"default:false" json:"revoked"`
	CreatedAt time.Time  `json:"created_at"`
}
