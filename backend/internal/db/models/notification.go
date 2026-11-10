// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Code split from models.go. Package models defines GORM models for the persistence layer.
package models

import "time"

// Notice — 全局通知中心收件箱。聚合各类业务事件（站庆/邮件/考核/公告/掉线/辅种/下载/备份/站免）。
// Source 标事件类型枚举；Key 用于去重（同 Source+Key 当天已发则跳过，避免周期 cron 刷屏；一次性事件留空）。
type Notice struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Source    string    `gorm:"size:32;index;uniqueIndex:uk_notice_src_key,priority:1" json:"source"`
	Key       string    `gorm:"size:160;index;uniqueIndex:uk_notice_src_key,priority:2" json:"key,omitempty"`
	Title     string    `gorm:"size:255" json:"title"`
	Body      string    `gorm:"type:text" json:"body,omitempty"`
	Level     string    `gorm:"size:16" json:"level"` // info | success | warning | danger
	Read      bool      `gorm:"default:false;index" json:"read"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}
