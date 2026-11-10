// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package oper

import (
	"errors"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"gorm.io/gorm"
)

// SSHHostKeyOper 封装 SSHHostKey 表的读写（TOFU：记录/查询/删除）。
// 结构上满足 monitor.HostKeyStore 接口（Lookup/Record），供 SSH 采集器注入，避免 monitor→oper 反向依赖。
type SSHHostKeyOper struct {
	db *gorm.DB
}

// NewSSHHostKeyOper 构造。
func NewSSHHostKeyOper(db *gorm.DB) *SSHHostKeyOper {
	return &SSHHostKeyOper{db: db}
}

// Lookup 返回 host:port 已记录的 host key（marshaled）；ok=false 表示未记录（首次连接）。
func (o *SSHHostKeyOper) Lookup(host string, port int) ([]byte, bool, error) {
	var k models.SSHHostKey
	err := o.db.Where("host = ? AND port = ?", host, port).First(&k).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return k.Key, true, nil
}

// Record 首次记录 host key（TOFU）。FirstOrCreate + Assign 处理并发首连竞态。
func (o *SSHHostKeyOper) Record(host string, port int, keyType string, marshaled []byte, fingerprint string) error {
	k := models.SSHHostKey{
		Host: host, Port: port, KeyType: keyType, Key: marshaled,
		Fingerprint: fingerprint, AcceptedAt: time.Now(),
	}
	return o.db.Where("host = ? AND port = ?", host, port).
		Assign(k).FirstOrCreate(&models.SSHHostKey{}).Error
}

// Delete 删除 host:port 的记录（合法换 key 后重置，下次连接重新 TOFU）。
func (o *SSHHostKeyOper) Delete(host string, port int) error {
	return o.db.Where("host = ? AND port = ?", host, port).
		Delete(&models.SSHHostKey{}).Error
}
