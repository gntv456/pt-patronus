// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package manager 管理多下载器实例：按 id 懒构建客户端并缓存，配置变更即失效重建。
// 独立成包以避免 downloader（接口/类型）↔ qb/tr（实现）之间的循环依赖。
package manager

import (
	"context"
	"fmt"
	"sync"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/downloader/ar"
	"github.com/hxpt/ptpatronus/internal/downloader/dl"
	"github.com/hxpt/ptpatronus/internal/downloader/fl"
	"github.com/hxpt/ptpatronus/internal/downloader/qb"
	"github.com/hxpt/ptpatronus/internal/downloader/rt"
	"github.com/hxpt/ptpatronus/internal/downloader/tr"
	"github.com/hxpt/ptpatronus/internal/downloader/ut"
)

// Manager 管理多下载器实例。
type Manager struct {
	op      *oper.DownloaderOper
	mu      sync.RWMutex
	clients map[uint]downloader.Downloader
}

// NewManager 构造管理器。
func NewManager(op *oper.DownloaderOper) *Manager {
	return &Manager{op: op, clients: map[uint]downloader.Downloader{}}
}

// Get 取/构建指定 id 的下载器客户端。
func (m *Manager) Get(ctx context.Context, id uint) (downloader.Downloader, error) {
	m.mu.RLock()
	c, ok := m.clients[id]
	m.mu.RUnlock()
	if ok {
		return c, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.clients[id]; ok {
		return c, nil
	}

	d, err := m.op.Get(id)
	if err != nil {
		return nil, err
	}
	pw, _ := m.op.Password(d)
	c, err = BuildClient(d, pw)
	if err != nil {
		return nil, err
	}
	m.clients[id] = c
	return c, nil
}

// Invalidate 失效某实例缓存（配置变更后调用，下次 Get 重建）。
func (m *Manager) Invalidate(id uint) {
	m.mu.Lock()
	delete(m.clients, id)
	m.mu.Unlock()
}

// BuildClient 按下载器记录与解密密码构造客户端。
func BuildClient(d *models.Downloader, password string) (downloader.Downloader, error) {
	switch d.Type {
	case "qb", "qbittorrent":
		return qb.New(d.URL, d.Username, password)
	case "tr", "transmission":
		c := tr.New(d.URL, d.Username, password)
		return c, nil
	case "ut", "utorrent":
		return ut.New(d.URL, d.Username, password)
	case "dl", "deluge":
		return dl.New(d.URL, d.Username, password), nil
	case "ar", "aria2":
		return ar.New(d.URL, d.Username, password), nil
	case "rt", "rutorrent":
		return rt.New(d.URL, d.Username, password), nil
	case "fl", "flood":
		return fl.New(d.URL, d.Username, password)
	default:
		return nil, fmt.Errorf("unknown downloader type: %s", d.Type)
	}
}
