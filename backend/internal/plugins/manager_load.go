package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/scheduler"
)

func (m *Manager) LoadExternal(root string) error {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	m.SetPluginRoot(root)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if _, err := m.LoadExternalDir(dir); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) LoadExternalDir(dir string) (*View, error) {
	manifestPath := filepath.Join(dir, "plugin.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("load plugin manifest %s: %w", manifestPath, err)
	}
	normalizeManifest(&manifest)
	if err := validateExternalManifest(manifest); err != nil {
		return nil, fmt.Errorf("validate plugin manifest %s: %w", manifestPath, err)
	}
	if manifest.Runtime != RuntimeExternalHTTP {
		return nil, fmt.Errorf("unsupported plugin runtime %s", manifest.Runtime)
	}
	if err := m.register(NewExternalHTTPPlugin(manifest, dir, m), true); err != nil {
		return nil, err
	}
	return m.Get(manifest.ID)
}

func (m *Manager) SetPluginRoot(root string) {
	root = filepath.Clean(root)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pluginRoot = root
	m.defaultMarket = filepath.Join(filepath.Dir(root), "plugin-market.json")
}

func (m *Manager) PluginRoot() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pluginRoot
}

func (m *Manager) DefaultMarketSource() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultMarket
}

// SetCfgOp 注入系统配置读写器（市场源等 UI 持久化配置）。应在 Start 前调用。
func (m *Manager) SetCfgOp(cfgOp *oper.SystemConfigOper) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfgOp = cfgOp
}

// SetMarketOper 注入市场运营层（安装量榜/收藏/多实例/天序）。应在 Start 前调用。
func (m *Manager) SetMarketOper(op *oper.PluginMarketOper) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.marketOp = op
}

func (m *Manager) Register(p Plugin) error {
	return m.register(p, false)
}

func (m *Manager) register(p Plugin, replace bool) error {
	if p == nil {
		return errors.New("plugin is nil")
	}
	manifest := p.Manifest()
	if manifest.ID == "" {
		return errors.New("plugin id is required")
	}
	m.mu.Lock()
	old, ok := m.plugins[manifest.ID]
	if ok && !replace {
		m.mu.Unlock()
		return fmt.Errorf("plugin %s already registered", manifest.ID)
	}
	if ok && old.Manifest().Builtin {
		m.mu.Unlock()
		return fmt.Errorf("plugin %s is built-in and cannot be replaced", manifest.ID)
	}
	if ok {
		stopPlugin(old)
	}
	m.plugins[manifest.ID] = p
	started := m.started
	m.mu.Unlock()
	if started {
		if ok {
			m.unsubscribePlugin(manifest.ID) // 替换：先摘掉旧实例的总线订阅，再订阅新实例
		}
		m.subscribePlugin(p)
		m.refreshSchedules(manifest.ID)
	}
	return nil
}

func (m *Manager) Start(ctx context.Context) {
	if m.bus == nil {
		return
	}
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	plugins := make([]Plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		plugins = append(plugins, p)
	}
	m.mu.Unlock()

	for _, p := range plugins {
		m.subscribePlugin(p)
	}
	// initialize plugin cron scheduler
	m.schedMu.Lock()
	m.sched = scheduler.New()
	m.sched.Start()
	m.schedMu.Unlock()
	for _, p := range plugins {
		m.refreshSchedules(p.Manifest().ID)
	}
	slog.Info("plugin manager started", "plugins", len(plugins))
}

// Stop shuts down plugin background services.
func (m *Manager) Stop() {
	m.stopDevWatcher()
	// 停止运行中的外部插件子进程与 host 回调服务（内置插件未实现 Stop，stopPlugin 为 no-op）。
	m.mu.RLock()
	plugins := make([]Plugin, 0, len(m.plugins))
	for _, p := range m.plugins {
		plugins = append(plugins, p)
	}
	m.mu.RUnlock()
	for _, p := range plugins {
		stopPlugin(p)
		m.unsubscribePlugin(p.Manifest().ID) // 停机：摘除全部总线订阅，避免 handler 泄漏
	}
	m.schedMu.Lock()
	sched := m.sched
	m.sched = nil
	m.schedMu.Unlock()
	if sched != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		select {
		case <-sched.Stop().Done():
		case <-ctx.Done():
		}
	}
	slog.Info("plugin manager stopped")
}

func (m *Manager) subscribePlugin(p Plugin) {
	if m.bus == nil || p == nil {
		return
	}
	manifest := p.Manifest()
	for _, ev := range manifest.Events {
		eventType := strings.TrimSpace(ev.Type)
		if eventType == "" {
			continue
		}
		m.mu.Lock()
		if m.subscribed[manifest.ID] == nil {
			m.subscribed[manifest.ID] = make(map[string]int)
		}
		if _, dup := m.subscribed[manifest.ID][eventType]; dup {
			m.mu.Unlock()
			continue
		}
		m.mu.Unlock()
		pluginID := manifest.ID
		// 不在持有 m.mu 时调 bus.Subscribe，避免与总线锁的顺序耦合。
		handle := m.bus.Subscribe(eventType, 1000, func(ctx context.Context, e event.Event) {
			m.handleEvent(ctx, pluginID, Event{Type: e.Type, Data: e.Data})
		})
		m.mu.Lock()
		m.subscribed[manifest.ID][eventType] = handle
		m.mu.Unlock()
	}
}

// unsubscribePlugin 移除某插件在事件总线上的全部订阅。旧实现只存 bool 且从不
// Unsubscribe：dev-watch 频繁重载时 handler 单调增长，且重载后事件仍路由到已 stop
// 的旧插件实例（subscribePlugin 的 dedup 会让新实例跳过订阅）。需在 bus 上调用，不持有 m.mu。
func (m *Manager) unsubscribePlugin(id string) {
	if m.bus == nil {
		return
	}
	m.mu.Lock()
	handles := m.subscribed[id]
	delete(m.subscribed, id)
	m.mu.Unlock()
	for eventType, handle := range handles {
		m.bus.Unsubscribe(eventType, handle)
	}
}

func (m *Manager) ReloadExternal(id string) (*View, error) {
	root := m.PluginRoot()
	if root == "" {
		return nil, errors.New("plugin root is not configured")
	}
	dir := filepath.Join(root, id)
	if !isSubpath(root, dir) {
		return nil, errors.New("invalid plugin path")
	}
	return m.LoadExternalDir(dir)
}

func (m *Manager) Uninstall(id string) error {
	root := m.PluginRoot()
	if root == "" {
		return errors.New("plugin root is not configured")
	}
	if dependents := m.requiredDependents(id, false); len(dependents) > 0 {
		names := make([]string, 0, len(dependents))
		for _, dependent := range dependents {
			names = append(names, dependent.ID)
		}
		return fmt.Errorf("plugin %s is required by installed plugins %s", id, strings.Join(names, ", "))
	}
	m.mu.Lock()
	p, ok := m.plugins[id]
	if ok && p.Manifest().Builtin {
		m.mu.Unlock()
		return errors.New("built-in plugin cannot be uninstalled")
	}
	if ok {
		stopPlugin(p)
		delete(m.plugins, id)
	}
	m.mu.Unlock()
	m.removeSchedules(id)
	m.unsubscribePlugin(id) // 卸载：摘除总线订阅，避免旧闭包残留
	dir := filepath.Join(root, id)
	if !isSubpath(root, dir) {
		return errors.New("invalid plugin path")
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if st, err := m.op.GetState(id); err == nil {
		st.Enabled = false
		st.Installed = false
		_ = m.op.SaveState(st)
	}
	m.log(id, logInfo, "lifecycle", "plugin uninstalled", nil)
	return nil
}
