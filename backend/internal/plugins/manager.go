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
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"gorm.io/gorm"
)

const (
	logInfo    = "info"
	logWarning = "warning"
	logError   = "error"
)

type Manager struct {
	mu            sync.RWMutex
	plugins       map[string]Plugin
	subscribed    map[string]map[string]bool
	op            *oper.PluginOper
	bus           *event.Bus
	notice        *notice.Service
	pluginRoot    string
	defaultMarket string
	started       bool
}

func NewManager(db *gorm.DB, bus *event.Bus, noticeSvc *notice.Service) *Manager {
	m := &Manager{
		plugins:    make(map[string]Plugin),
		subscribed: make(map[string]map[string]bool),
		op:         oper.NewPluginOper(db),
		bus:        bus,
		notice:     noticeSvc,
	}
	_ = m.Register(NewEventAuditPlugin())
	_ = m.Register(NewNoticeForwarderPlugin())
	return m
}

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
	if manifest.Runtime != RuntimeExternalHTTP {
		return nil, fmt.Errorf("unsupported plugin runtime %s", manifest.Runtime)
	}
	if err := m.register(NewExternalHTTPPlugin(manifest, dir), true); err != nil {
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
		m.subscribePlugin(p)
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
	slog.Info("plugin manager started", "plugins", len(plugins))
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
			m.subscribed[manifest.ID] = make(map[string]bool)
		}
		if m.subscribed[manifest.ID][eventType] {
			m.mu.Unlock()
			continue
		}
		m.subscribed[manifest.ID][eventType] = true
		m.mu.Unlock()
		pluginID := manifest.ID
		m.bus.Subscribe(eventType, 1000, func(ctx context.Context, e event.Event) {
			m.handleEvent(ctx, pluginID, Event{Type: e.Type, Data: e.Data})
		})
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
	if _, err := m.op.EnsureState(id, p.DefaultConfig()); err != nil {
		return err
	}
	if err := m.op.SetEnabled(id, enabled); err != nil {
		return err
	}
	m.log(id, logInfo, "lifecycle", fmt.Sprintf("plugin enabled=%v", enabled), nil)
	return nil
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
		return nil, errors.New("plugin is disabled")
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

func (m *Manager) plugin(id string) (Plugin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.plugins[id]
	if !ok {
		return nil, fmt.Errorf("plugin %s not found", id)
	}
	return p, nil
}

func (m *Manager) viewOf(p Plugin) (View, error) {
	st, cfg, err := m.stateAndConfig(p)
	if err != nil {
		return View{}, err
	}
	manifest := p.Manifest()
	return View{
		Manifest:    manifest,
		Enabled:     st.Enabled,
		Installed:   st.Installed,
		Config:      cfg,
		LastError:   st.LastError,
		LastEventAt: st.LastEventAt,
		CreatedAt:   st.CreatedAt,
		UpdatedAt:   st.UpdatedAt,
	}, nil
}

func (m *Manager) stateAndConfig(p Plugin) (*models.PluginState, map[string]any, error) {
	manifest := p.Manifest()
	st, err := m.op.EnsureState(manifest.ID, p.DefaultConfig())
	if err != nil {
		return nil, nil, err
	}
	cfg := p.DefaultConfig()
	if st.ConfigJSON != "" {
		var saved map[string]any
		if err := json.Unmarshal([]byte(st.ConfigJSON), &saved); err == nil {
			cfg = mergeConfig(cfg, saved)
		}
	}
	return st, cfg, nil
}

func (m *Manager) handleEvent(ctx context.Context, id string, e Event) {
	p, err := m.plugin(id)
	if err != nil {
		return
	}
	if !declaresEvent(p.Manifest(), e.Type) {
		return
	}
	st, cfg, err := m.stateAndConfig(p)
	if err != nil || !st.Enabled {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err = p.HandleEvent(cctx, m.runtime(id, cfg), e)
	now := time.Now()
	if err != nil {
		_ = m.op.MarkEvent(id, now, err.Error())
		m.log(id, logError, e.Type, err.Error(), e.Data)
		return
	}
	_ = m.op.MarkEvent(id, now, "")
	m.log(id, logInfo, e.Type, "event handled", nil)
}

func declaresEvent(manifest Manifest, eventType string) bool {
	for _, ev := range manifest.Events {
		if strings.TrimSpace(ev.Type) == eventType {
			return true
		}
	}
	return false
}

func stopPlugin(p Plugin) {
	if s, ok := p.(interface{ Stop() error }); ok {
		_ = s.Stop()
	}
}

func (m *Manager) runtime(id string, cfg map[string]any) Runtime {
	return &pluginRuntime{manager: m, pluginID: id, config: cfg}
}

func (m *Manager) log(pluginID, level, eventName, message string, data map[string]any) {
	raw, _ := json.Marshal(data)
	if len(raw) > 8000 {
		raw = raw[:8000]
	}
	_ = m.op.CreateLog(&models.PluginLog{
		PluginID: pluginID,
		Level:    level,
		Event:    eventName,
		Message:  message,
		DataJSON: string(raw),
	})
}

func mergeConfig(defaults, saved map[string]any) map[string]any {
	out := make(map[string]any, len(defaults)+len(saved))
	for k, v := range defaults {
		out[k] = v
	}
	for k, v := range saved {
		out[k] = v
	}
	return out
}

func validateConfig(schema []ConfigField, cfg map[string]any) error {
	for _, f := range schema {
		v, ok := cfg[f.Key]
		if f.Required && (!ok || v == nil || strings.TrimSpace(fmt.Sprint(v)) == "") {
			return fmt.Errorf("%s is required", f.Label)
		}
	}
	return nil
}

func logView(l models.PluginLog) LogView {
	var data map[string]any
	if l.DataJSON != "" && l.DataJSON != "null" {
		_ = json.Unmarshal([]byte(l.DataJSON), &data)
	}
	return LogView{
		ID:        l.ID,
		PluginID:  l.PluginID,
		Level:     l.Level,
		Event:     l.Event,
		Message:   l.Message,
		Data:      data,
		CreatedAt: l.CreatedAt,
	}
}
