// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

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
	"gorm.io/gorm"
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

// ErrPluginNotInstalled 让「卸载本机根本没装的插件」不再静默成功。
// 中文文案只在 errcode 一处写，这里只当分类载体。
var ErrPluginNotInstalled = errors.New("plugin not installed")

// 插件动作的三类失败此前没有哨兵，接口层只能一律报 400「请求参数有误」，
// 用户对着输入框反复试，而真实原因（没启用 / 本机没注册 / 设置读不出）只在 DEBUG 日志里。
var (
	// ErrPluginStateNotCleared 卸载已删掉目录、状态行却没落成未安装：
	// 面板显示「仍已安装」而调用报「插件未安装」，两份读数互相矛盾，不能当成功交付。
	ErrPluginStateNotCleared  = errors.New("plugin state not cleared after uninstall")
	ErrPluginDisabled         = errors.New("plugin is disabled")
	ErrPluginNotRegistered    = errors.New("plugin not registered")
	ErrPluginConfigUnreadable = errors.New("plugin config unreadable")
)

// 回滚是破坏性动作（进函数就先 stopPlugin，再把当前目录 rename 走），
// 从前这些失败在接口层一律 400「请求参数有误」：用户被支去改输入框、反复重点同一个按钮，
// 而每一次重试都会再停一次插件、再动一次目录。中文文案仍只在 errcode 一处写。
var (
	// ErrPluginRootUnconfigured 本机根本没配置文件目录，回滚无从落地——不是输入问题。
	ErrPluginRootUnconfigured = errors.New("plugin root is not configured")
	// ErrReleaseNotForPlugin 版本记录属于另一个插件（前端列表串了/参数拼错）。
	ErrReleaseNotForPlugin = errors.New("release does not belong to plugin")
	// ErrRollbackRestoreFailed 回滚中途失败、连原来的目录都没能放回去：
	// 磁盘上此刻既没有新版也没有旧版，绝不能只说「回滚失败，请重试」。
	ErrRollbackRestoreFailed = errors.New("rollback backup could not be restored")
	// ErrRollbackLedgerNotWritten 磁盘已经换成旧版本，但「当前版本」这条账面没写进去。
	ErrRollbackLedgerNotWritten = errors.New("active release could not be recorded after rollback")
)

// markPluginUninstalled 把插件状态行落成「未安装/停用」。判定只写这一处
// （卸载接口与开发目录监听两条路共用，抄第二遍就会漂移）：
//   - 没有状态行：库里不会留着假的「已安装」，视为完成（既有测试依赖这条幂等语义）；
//   - 读得出却写不进 / 读得出别的错：必须报失败——目录已经删了，状态行还标着已安装的话，
//     面板刷新后插件会「复活」成已安装且启用，指向一个不存在的目录。
//
// 重试是安全的：os.RemoveAll 对已不存在的目录回 nil，第二次只会重做状态行这一半。
func (m *Manager) markPluginUninstalled(id string) error {
	st, err := m.op.GetState(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("%w: 插件 %s 的目录已删除，但状态行读不出来（%v），数据库可能仍标记为已安装，请重试卸载",
			ErrPluginStateNotCleared, id, err)
	}
	st.Enabled = false
	st.Installed = false
	if err := m.op.SaveState(st); err != nil {
		return fmt.Errorf("%w: 插件 %s 的目录已删除，但状态行没能改成未安装（%v），面板刷新后它可能仍显示已安装，请重试卸载",
			ErrPluginStateNotCleared, id, err)
	}
	return nil
}

// configUnreadableError 让「设置读不出」这条**人话句子**既能被 errors.Is 分类，
// 又原样送到界面（api/v1 用 WithDetail 取 err.Error() 当 message）。
type configUnreadableError struct{ detail string }

func (e *configUnreadableError) Error() string { return e.detail }
func (e *configUnreadableError) Unwrap() error { return ErrPluginConfigUnreadable }

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
	// 从前这一步无条件成功：os.RemoveAll 对不存在的路径返回 nil，于是「本机压根没装这个插件」
	// 也被回成 200 {"uninstalled":id}，界面弹「已卸载」——多半是列表陈旧或点错了行。
	// 判据只用三条正向证据（内存加载着 / 磁盘目录在 / 状态行标已安装），任一成立就照常卸载；
	// 读不到状态行不算「没装」，免得主密钥轮换或库抖动把合法卸载也掐掉。
	if !ok {
		_, serr := os.Stat(dir)
		stateInstalled := false
		if st, gerr := m.op.GetState(id); gerr == nil {
			stateInstalled = st.Installed
		}
		if serr != nil && !stateInstalled {
			return ErrPluginNotInstalled
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := m.markPluginUninstalled(id); err != nil {
		return err
	}
	m.log(id, logInfo, "lifecycle", "plugin uninstalled", nil)
	return nil
}
