// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hxpt/ptpatronus/internal/notice"
	"gorm.io/gorm"
)

type pluginRuntime struct {
	manager  *Manager
	pluginID string
	config   map[string]any
}

// CallHostTool 内置插件直调宿主工具（与外部插件 POST /tools/{name} 同一 Dispatch 路径，
// 含权限校验与审计）。返回原始结果（多为 map/[]map）。
func (r *pluginRuntime) CallHostTool(ctx context.Context, name string, input map[string]any) (any, error) {
	if r.manager == nil || r.manager.hostTools == nil {
		return nil, fmt.Errorf("host tools unavailable")
	}
	p, err := r.manager.plugin(r.pluginID)
	if err != nil {
		return nil, err
	}
	out, perm, notFound, derr := r.manager.hostTools.Dispatch(ctx, p.Manifest().Permissions, name, input)
	if notFound {
		return nil, fmt.Errorf("tool %q not found", name)
	}
	if derr != nil {
		r.manager.log(r.pluginID, logWarning, "host.tool", fmt.Sprintf("%s failed: %v", name, derr), map[string]any{"input": input})
		return nil, derr
	}
	if isWritePermission(perm) {
		r.manager.log(r.pluginID, logInfo, "host.tool", "tool called: "+name, map[string]any{"input": input})
	}
	return out, nil
}

func (r *pluginRuntime) Config() map[string]any {
	out := make(map[string]any, len(r.config))
	for k, v := range r.config {
		out[k] = v
	}
	return out
}

func (r *pluginRuntime) Log(level, eventName, message string, data map[string]any) {
	r.manager.log(r.pluginID, level, eventName, message, data)
}

func (r *pluginRuntime) Notice(ctx context.Context, in NoticeInput) error {
	if r.manager.notice == nil {
		return nil
	}
	return r.manager.notice.Notice(ctx, notice.Input{
		Source: "plugin",
		Key:    fmt.Sprintf("%s:%d", r.pluginID, time.Now().UnixNano()),
		Title:  in.Title,
		Body:   in.Body,
		Level:  in.Level,
	}, false)
}

func (r *pluginRuntime) Publish(ctx context.Context, eventType string, data map[string]any) {
	r.manager.PublishEvent(ctx, eventType, data)
}

// KVGet/KVSet/KVDelete 委托 manager.kv。测试环境 kv 为 nil 时 KVGet 返回 ErrRecordNotFound（便于插件走默认值分支）。
func (r *pluginRuntime) KVGet(key string, out any) error {
	if r.manager.kv == nil {
		return gorm.ErrRecordNotFound
	}
	return r.manager.kv.KVGet(r.pluginID, key, out)
}

func (r *pluginRuntime) KVSet(key string, val any) error {
	if r.manager.kv == nil {
		return errors.New("plugin KV store unavailable")
	}
	return r.manager.kv.KVSet(r.pluginID, key, val)
}

func (r *pluginRuntime) KVDelete(key string) error {
	if r.manager.kv == nil {
		return nil
	}
	return r.manager.kv.KVDelete(r.pluginID, key)
}

// SiteList 返回所有已配置站点（只读视图，不含密钥）。sites 为 nil（测试环境）返回 nil。
func (r *pluginRuntime) SiteList() []SiteRef {
	if r.manager.sites == nil {
		return nil
	}
	ss, err := r.manager.sites.List()
	if err != nil {
		return nil
	}
	out := make([]SiteRef, 0, len(ss))
	for _, s := range ss {
		out = append(out, SiteRef{ID: s.ID, Name: s.Name, BaseURL: s.BaseURL, Username: s.Username})
	}
	return out
}

// SiteCookie 解密返回某站点 cookie（复用 SiteOper 的 AES-GCM 解密）。
func (r *pluginRuntime) SiteCookie(id uint) (string, error) {
	if r.manager.sites == nil {
		return "", errors.New("site store unavailable")
	}
	s, err := r.manager.sites.Get(id)
	if err != nil {
		return "", err
	}
	return r.manager.sites.Cookie(s)
}

// SetRuntimeConfig 合并写回 ConfigJSON 的单个键并触发调度重排（动态 cron 用）。
func (r *pluginRuntime) SetRuntimeConfig(key string, val any) error {
	return r.manager.saveRuntimeConfig(r.pluginID, key, val)
}
