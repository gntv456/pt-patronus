// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Code split from models.go. Package models defines GORM models for the persistence layer.
package models

import "time"

// Downloader — 下载器连接配置。
type Downloader struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:64;not null" json:"name"`
	Type        string    `gorm:"size:16;not null" json:"type"` // qb | tr | ut | dl | ar | rt | fl
	URL         string    `gorm:"size:255" json:"url"`
	Username    string    `gorm:"size:64" json:"username"`
	PasswordEnc string    `gorm:"type:text" json:"-"`
	IsDefault   bool      `gorm:"default:false" json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
