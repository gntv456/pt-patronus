// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Code split from models.go. Package models defines GORM models for the persistence layer.
package models

import "time"

// UserPreference — 用户界面偏好（跨浏览器同步）。对标 MSaber「偏好随账号同步」：
// 主题/壁纸/侧栏/卡片样式等 UI 偏好存服务端，登录即拉取，localStorage 仅作离线缓存。
// Value 为 JSON 文本（前端结构自由演进，后端只透传；上限 64KB 防滥用）。
type UserPreference struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"uniqueIndex:uk_user_pref,priority:1;not null" json:"user_id"`
	Key       string    `gorm:"size:64;uniqueIndex:uk_user_pref,priority:2" json:"key"` // 偏好域（如 ui、dashboard）；一域一行
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// User — 单用户实例。本地账号登录以 Username + PasswordHash 为准；
// HxptUID / Vip* 等字段为好学时代外部身份的历史兼容快照，保留列不删（老数据）。
type User struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Username      string     `gorm:"uniqueIndex;size:64;not null" json:"username"`
	PasswordHash  string     `gorm:"size:128" json:"-"` // bcrypt 本地登录密码；空=存量未迁移账号
	IsAdmin       bool       `gorm:"default:false" json:"is_admin"`
	Level         int        `gorm:"default:0" json:"level"`        // 与站点认证等级配合
	HxptUID       string     `gorm:"size:64;index" json:"hxpt_uid"` // 历史身份标识；本地账号为 "local:<username>"
	HxptUsername  string     `gorm:"size:64" json:"hxpt_username"`
	VipLevel      int        `gorm:"default:0" json:"vip_level"` // 历史快照，登录时写 1 兼容旧前端
	VipVerifiedAt *time.Time `json:"vip_verified_at"`            // 历史快照，不再作准入判定
	TokenVersion  uint       `gorm:"default:0" json:"-"`         // 登出 bump → 旧 JWT 失效
	HxptCookieEnc string     `gorm:"type:text" json:"-"`         // 历史列，VIP 重验已退役
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}
