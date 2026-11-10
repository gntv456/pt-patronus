// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"errors"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// SystemConfigOper 读写运行时可变配置（SystemConfig key/value 表）。
// 用于 IYUU token 等用户经 UI 输入的设置（对齐 config 包「runtime settings live in DB」）。
type SystemConfigOper struct {
	db *gorm.DB
}

// NewSystemConfigOper 构造。
func NewSystemConfigOper(db *gorm.DB) *SystemConfigOper {
	return &SystemConfigOper{db: db}
}

// Get 取某 key 的值；不存在返回 ("", nil)。
func (o *SystemConfigOper) Get(key string) (string, error) {
	var sc models.SystemConfig
	err := o.db.Where("`key` = ?", key).First(&sc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return sc.Value, nil
}

// Set 写某 key 的值（存在则更新，不存在则插入）。
func (o *SystemConfigOper) Set(key, value string) error {
	var sc models.SystemConfig
	err := o.db.Where("`key` = ?", key).First(&sc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return o.db.Create(&models.SystemConfig{Key: key, Value: value}).Error
	}
	if err != nil {
		return err
	}
	return o.db.Model(&models.SystemConfig{}).Where("`key` = ?", key).Update("value", value).Error
}
