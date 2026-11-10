// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (m *Manager) List() ([]View, error) {
	m.mu.RLock()
	plugins := make([]Plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		plugins = append(plugins, p)
	}
	m.mu.RUnlock()

	out := make([]View, 0, len(plugins))
	for _, p := range plugins {
		v, err := m.viewOf(p)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (m *Manager) Get(id string) (*View, error) {
	p, err := m.plugin(id)
	if err != nil {
		return nil, err
	}
	v, err := m.viewOf(p)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (m *Manager) SetEnabled(id string, enabled bool) error {
	p, err := m.plugin(id)
	if err != nil {
		return err
	}
	manifest := p.Manifest()
	normalizeManifest(&manifest)
	if enabled {
		if _, ok, reason := m.evaluateDependencies(manifest.Dependencies, true); !ok {
			if strings.TrimSpace(reason) == "" {
				reason = "plugin dependencies are not satisfied"
			}
			return fmt.Errorf("%w：需要先满足该插件声明的依赖（%s）", ErrPluginStateRejected, reason)
		}
	} else {
		if dependents := m.requiredDependents(id, true); len(dependents) > 0 {
			names := make([]string, 0, len(dependents))
			for _, dependent := range dependents {
				names = append(names, dependent.ID)
			}
			return fmt.Errorf("%w：%s 正被已启用的插件依赖（%s），请先停用这些插件", ErrPluginStateRejected, id, strings.Join(names, ", "))
		}
	}
	if _, err := m.op.EnsureState(id, p.DefaultConfig()); err != nil {
		return err
	}
	if err := m.op.SetEnabled(id, enabled); err != nil {
		return err
	}
	if !enabled {
		stopPlugin(p)
	}
	m.log(id, logInfo, "lifecycle", fmt.Sprintf("plugin enabled=%v", enabled), nil)
	m.refreshSchedules(id)
	return nil
}

func (m *Manager) pluginEnabled(id string) bool {
	st, err := m.op.GetState(id)
	return err == nil && st.Enabled
}

func (m *Manager) SaveConfig(id string, cfg map[string]any) (*View, error) {
	p, err := m.plugin(id)
	if err != nil {
		return nil, err
	}
	manifest := p.Manifest()
	if err := validateConfig(manifest.ConfigSchema, cfg); err != nil {
		return nil, err
	}
	st, err := m.op.EnsureState(id, p.DefaultConfig())
	if err != nil {
		return nil, err
	}
	merged := mergeConfig(p.DefaultConfig(), cfg)
	raw, _ := json.Marshal(merged)
	st.ConfigJSON = string(raw)
	if err := m.op.SaveState(st); err != nil {
		return nil, err
	}
	m.log(id, logInfo, "config", "plugin config saved", nil)
	m.refreshSchedules(id)
	return m.Get(id)
}

func (m *Manager) ExecuteAction(ctx context.Context, id, action string, input map[string]any) (map[string]any, error) {
	p, err := m.plugin(id)
	if err != nil {
		return nil, err
	}
	st, cfg, err := m.stateAndConfig(p)
	if err != nil {
		return nil, err
	}
	if !st.Enabled {
		return nil, ErrPluginDisabled
	}
	rt := m.runtime(id, cfg)
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := p.ExecuteAction(cctx, rt, action, input)
	if err != nil {
		_ = m.op.MarkEvent(id, time.Now(), err.Error())
		m.log(id, logError, action, err.Error(), input)
		return nil, err
	}
	m.log(id, logInfo, action, "plugin action executed", input)
	return out, nil
}

func (m *Manager) PublishEvent(ctx context.Context, eventType string, data map[string]any) {
	if m.bus == nil || eventType == "" {
		return
	}
	m.bus.Publish(ctx, eventType, data)
}

// CountLogs 日志总数（与 Logs 同筛选），供 handler 给账面。
func (m *Manager) CountLogs(pluginID string) (int64, error) {
	return m.op.CountLogs(pluginID)
}

func (m *Manager) Logs(pluginID string, limit int) ([]LogView, error) {
	logs, err := m.op.ListLogs(pluginID, limit)
	if err != nil {
		return nil, err
	}
	out := make([]LogView, 0, len(logs))
	for _, l := range logs {
		out = append(out, logView(l))
	}
	return out, nil
}

func (m *Manager) ReadAsset(id, name string) (*Asset, error) {
	p, err := m.plugin(id)
	if err != nil {
		return nil, err
	}
	if p.Manifest().Builtin {
		return nil, errors.New("built-in plugin does not expose web assets")
	}
	root := m.PluginRoot()
	if root == "" {
		return nil, errors.New("plugin root is not configured")
	}
	name = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(name)), "/")
	if name == "" {
		name = "index.html"
	}
	name = strings.TrimPrefix(name, "web/")
	assetRoot := filepath.Join(root, id, "web")
	target := filepath.Join(assetRoot, filepath.FromSlash(name))
	if !isSubpath(assetRoot, target) {
		return nil, errors.New("invalid plugin asset path")
	}
	st, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		target = filepath.Join(target, "index.html")
		if !isSubpath(assetRoot, target) {
			return nil, errors.New("invalid plugin asset path")
		}
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}
	contentType := mime.TypeByExtension(filepath.Ext(target))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return &Asset{Data: data, ContentType: contentType}, nil
}
