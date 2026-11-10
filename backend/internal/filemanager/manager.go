// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package filemanager

import (
	"context"
	"fmt"
	"sync"

	"github.com/hxpt/ptpatronus/internal/agentrpc"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// Manager 管理多存储实例：按 id 懒构建后端并缓存，配置变更即失效。
type Manager struct {
	op       *oper.FileStorageOper
	agentHub agentrpc.AgentRPCHub // type=agent 后端用；SetAgentHub 后注入（app.go 装配）
	mu       sync.RWMutex
	clients  map[uint]StorageBackend
}

// NewManager 构造。
func NewManager(op *oper.FileStorageOper) *Manager {
	return &Manager{op: op, clients: map[uint]StorageBackend{}}
}

// SetAgentHub 注入 agent RPC hub（agent 文件管理后端用）。app.go 在 agentHub 构建后调用，
// 避免倒置装配顺序（fileMgr 早于 agentHub 构建）；Serve 前 set 即无并发风险。
func (m *Manager) SetAgentHub(hub agentrpc.AgentRPCHub) {
	m.agentHub = hub
}

// Get 取/构建指定 id 的存储后端。
func (m *Manager) Get(ctx context.Context, id uint) (StorageBackend, error) {
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
	f, err := m.op.Get(id)
	if err != nil {
		return nil, err
	}
	pw, _ := m.op.Password(f)
	c, err = m.buildBackend(f, pw)
	if err != nil {
		return nil, err
	}
	m.clients[id] = c
	return c, nil
}

// Invalidate 失效缓存。
func (m *Manager) Invalidate(id uint) {
	m.mu.Lock()
	delete(m.clients, id)
	m.mu.Unlock()
}

// buildBackend 按记录与密码构造后端（方法以访问 agentHub；agent 类型需 hub 注入）。
func (m *Manager) buildBackend(f *models.FileStorage, password string) (StorageBackend, error) {
	switch f.Type {
	case "webdav":
		return NewWebDAV(f.URL, f.Username, password, f.BasePath), nil
	case "alist":
		return NewAlist(f.URL, f.Username, password, f.BasePath), nil
	case "115", "quark", "123":
		// 网盘后端（只读浏览）：cookie/token 存于 password，浏览根存于 basePath。
		return NewCloudBackend(f.Type, password, f.BasePath)
	case "smb":
		return NewSMB(f.URL, f.Username, password, f.BasePath), nil
	case "rclone":
		// URL=二进制路径；Username=remote 名；password=已解密的 rclone.conf INI 原文；BasePath=remote 内根子路径。
		return NewRclone(f.URL, f.Username, password, f.BasePath)
	case "agent":
		if m.agentHub == nil {
			return nil, fmt.Errorf("agent 文件管理后端未就绪")
		}
		return NewAgentBackend(m.agentHub, f.AgentID, f.BasePath), nil
	case "local":
		// local 无需 URL/密码，根路径即本机物理目录；NewLocal 会校验目录存在
		return NewLocal(f.BasePath)
	default:
		return nil, fmt.Errorf("unknown storage type: %s", f.Type)
	}
}
