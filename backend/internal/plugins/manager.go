package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/scheduler"
	"gorm.io/gorm"
)

const (
	logInfo    = "info"
	logWarning = "warning"
	logError   = "error"
)

// 插件签名信任模型（P1 安全修复）：
// 旧实现用市场索引 idx.PublicKey 验证同源的 idx 签名——攻击者篡改市场源可同时换公钥+签名绕过。
// 新模型：信任锚独立于被验证数据——内置官方公钥（默认）+ 用户配置 plugin.trusted_public_keys，
// 未签名插件需 plugin.allow_unsigned=true 显式放行。详见 market.go verifySignatureTrusted。

// OfficialMarketPublicKey 是 PTPatronus 官方插件市场的 Ed25519 公钥（base64），内置作默认信任锚。
// 取自官方市场仓 examples/plugin-market/plugin-market.json，官方签名插件自动受信。
const OfficialMarketPublicKey = "cbK2okPdJoXOUPlW3vL1KEKWM6Owm4rO6Mw3+kICRfs="

const (
	cfgKeyTrustedPublicKeys = "plugin.trusted_public_keys" // JSON []string(base64)：用户追加的受信公钥
	cfgKeyAllowUnsigned     = "plugin.allow_unsigned"      // "true"/"false"，默认 false（严格）
)

// TrustedPublicKeys 返回受信公钥集：内置官方公钥 + 用户配置（plugin.trusted_public_keys）。
func (m *Manager) TrustedPublicKeys() []string {
	keys := []string{OfficialMarketPublicKey}
	if m.cfgOp != nil {
		if raw, err := m.cfgOp.Get(cfgKeyTrustedPublicKeys); err == nil && strings.TrimSpace(raw) != "" {
			var extra []string
			if json.Unmarshal([]byte(raw), &extra) == nil {
				keys = append(keys, extra...)
			}
		}
	}
	return keys
}

// AllowUnsigned 是否允许安装未签名插件（默认 false=严格）。生产建议保持 false。
func (m *Manager) AllowUnsigned() bool {
	if m.cfgOp != nil {
		if raw, err := m.cfgOp.Get(cfgKeyAllowUnsigned); err == nil {
			return strings.EqualFold(strings.TrimSpace(raw), "true")
		}
	}
	return false
}

// TrustConfig 是插件签名信任模型的用户可配置部分（GET/PUT /plugins/trust-config）。
type TrustConfig struct {
	OfficialKey       string   `json:"official_key"`        // 内置官方公钥（只读，始终受信）
	TrustedPublicKeys []string `json:"trusted_public_keys"` // 用户追加的受信公钥（base64）
	AllowUnsigned     bool     `json:"allow_unsigned"`      // 是否允许未签名插件安装
}

// GetTrustConfig 返回信任配置（官方只读 + 用户公钥 + allow_unsigned）。
func (m *Manager) GetTrustConfig() TrustConfig {
	cfg := TrustConfig{OfficialKey: OfficialMarketPublicKey, AllowUnsigned: m.AllowUnsigned(), TrustedPublicKeys: []string{}}
	if m.cfgOp != nil {
		if raw, err := m.cfgOp.Get(cfgKeyTrustedPublicKeys); err == nil && strings.TrimSpace(raw) != "" {
			_ = json.Unmarshal([]byte(raw), &cfg.TrustedPublicKeys)
		}
	}
	return cfg
}

// SetTrustConfig 持久化用户受信公钥 + allow_unsigned（官方公钥不可改）。
func (m *Manager) SetTrustConfig(trustedKeys []string, allowUnsigned bool) error {
	if m.cfgOp == nil {
		return errors.New("config store unavailable")
	}
	keysJSON, _ := json.Marshal(trustedKeys)
	if err := m.cfgOp.Set(cfgKeyTrustedPublicKeys, string(keysJSON)); err != nil {
		return err
	}
	val := "false"
	if allowUnsigned {
		val = "true"
	}
	return m.cfgOp.Set(cfgKeyAllowUnsigned, val)
}

type Manager struct {
	mu            sync.RWMutex
	plugins       map[string]Plugin
	subscribed    map[string]map[string]bool
	op            *oper.PluginOper
	kv            *oper.PluginKVOper
	sites         *oper.SiteOper
	bus           *event.Bus
	notice        *notice.Service
	pluginRoot    string
	defaultMarket string
	cfgOp         *oper.SystemConfigOper // UI 设置的市场源等运行时配置（SystemConfig）；nil 时 FetchMarket 回退本地 plugin-market.json
	started       bool
	sched         cronScheduler
	schedJobs     map[string][]string
	schedMu       sync.Mutex
	watchMu       sync.Mutex
	watchCancel   context.CancelFunc
	watchWG       sync.WaitGroup
	mediaSearcher MediaSearcher // 由 app.go 注入（MediaChain 适配器），nil 时 /media/search 返回空
}

type Asset struct {
	Data        []byte
	ContentType string
}

// cronScheduler abstracts the scheduler methods used by the plugin manager.
type cronScheduler interface {
	AddFunc(name, spec string, fn func()) error
	Remove(name string)
	Start()
	Stop() context.Context
}

func NewManager(db *gorm.DB, bus *event.Bus, noticeSvc *notice.Service, sites *oper.SiteOper, kv *oper.PluginKVOper) *Manager {
	m := &Manager{
		plugins:    make(map[string]Plugin),
		subscribed: make(map[string]map[string]bool),
		op:         oper.NewPluginOper(db),
		kv:         kv,
		sites:      sites,
		bus:        bus,
		notice:     noticeSvc,
		schedJobs:  make(map[string][]string),
	}
	_ = m.Register(NewEventAuditPlugin())
	_ = m.Register(NewNoticeForwarderPlugin())
	_ = m.Register(NewHTTPProbePlugin())
	_ = m.Register(NewJSONToolboxPlugin())
	_ = m.Register(NewTextToolboxPlugin())
	_ = m.Register(NewWebhookTransformerPlugin())
	_ = m.Register(NewKeywordAlertPlugin())
	_ = m.Register(NewCronHelperPlugin())
	_ = m.Register(NewCloudflareSubscribePlugin())
	_ = m.Register(NewGroupChatZonePlugin())
	_ = m.Register(NewTwoFAHelperPlugin())
	_ = m.Register(NewXiaomiRouterPlugin())
	_ = m.Register(NewAutoSpeedPlugin())
	_ = m.Register(NewDramaScrapePlugin())
	_ = m.Register(NewPlayletCategoryPlugin())
	return m
}

// SetMediaSearcher 注入媒体检索适配器（app.go 用 MediaChain 包装），供 Host API /media/search 使用。
func (m *Manager) SetMediaSearcher(s MediaSearcher) { m.mediaSearcher = s }

// MediaSearch 供 externalHostServer 的 /media/search 端点调用；未注入时返回空。
func (m *Manager) MediaSearch(ctx context.Context, keyword string, year int) ([]MediaHit, error) {
	if m.mediaSearcher == nil {
		return []MediaHit{}, nil
	}
	return m.mediaSearcher.Search(ctx, keyword, year)
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

// pluginMarketSourcesKey 是 SystemConfig 中存储插件市场源列表的 key（JSON []string）。
// pluginMarketSourceKey 为旧版单源 key，仅在迁移读取时回退使用。
const (
	pluginMarketSourcesKey = "plugin.market_sources"
	pluginMarketSourceKey  = "plugin.market_source"
)

// normalizeSources 对市场源列表做 trim、去空、按串去重（保持首次出现顺序）。
func normalizeSources(sources []string) []string {
	seen := make(map[string]struct{}, len(sources))
	out := make([]string, 0, len(sources))
	for _, s := range sources {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// GetMarketSources 返回 UI 持久化的插件市场源列表（GitHub 仓库地址或任意 URL）；
// 未设置或未注入 cfgOp 返回 nil。新 key 为空时回退读旧版单源 key（一次性迁移语义）。
func (m *Manager) GetMarketSources() []string {
	m.mu.RLock()
	cfgOp := m.cfgOp
	m.mu.RUnlock()
	if cfgOp == nil {
		return nil
	}
	var sources []string
	if raw, err := cfgOp.Get(pluginMarketSourcesKey); err == nil && strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &sources); err != nil {
			slog.Warn("parse plugin market sources", "err", err, "raw", raw)
			sources = nil
		}
	}
	if len(sources) == 0 {
		// 迁移：新版列表为空时回退读旧版单源 key。
		if old, err := cfgOp.Get(pluginMarketSourceKey); err == nil && strings.TrimSpace(old) != "" {
			sources = []string{old}
		}
	}
	return normalizeSources(sources)
}

// SetMarketSources 持久化插件市场源列表（normalize 后写入；空列表即清除，回退本地 plugin-market.json）。
// 同时清空旧版单源 key 完成迁移。
func (m *Manager) SetMarketSources(sources []string) error {
	m.mu.RLock()
	cfgOp := m.cfgOp
	m.mu.RUnlock()
	if cfgOp == nil {
		return errors.New("system config is not available")
	}
	normalized := normalizeSources(sources)
	raw, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	if err := cfgOp.Set(pluginMarketSourcesKey, string(raw)); err != nil {
		return err
	}
	// 清除旧版单源 key，避免迁移回退读到残留旧值。
	if err := cfgOp.Set(pluginMarketSourceKey, ""); err != nil {
		slog.Warn("clear legacy plugin market source key", "err", err)
	}
	return nil
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
	manifest := p.Manifest()
	normalizeManifest(&manifest)
	if enabled {
		if _, ok, reason := m.evaluateDependencies(manifest.Dependencies, true); !ok {
			if strings.TrimSpace(reason) == "" {
				reason = "plugin dependencies are not satisfied"
			}
			return errors.New(reason)
		}
	} else {
		if dependents := m.requiredDependents(id, true); len(dependents) > 0 {
			names := make([]string, 0, len(dependents))
			for _, dependent := range dependents {
				names = append(names, dependent.ID)
			}
			return fmt.Errorf("plugin %s is required by enabled plugins %s", id, strings.Join(names, ", "))
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
	if strings.HasPrefix(name, "web/") {
		name = strings.TrimPrefix(name, "web/")
	}
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

func (m *Manager) Bridge(ctx context.Context, id, method string, params map[string]any) (map[string]any, error) {
	if params == nil {
		params = map[string]any{}
	}
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
	manifest := p.Manifest()
	rt := m.runtime(id, cfg)
	require := func(permission string) error {
		if hasPluginPermission(manifest.Permissions, permission) {
			return nil
		}
		return fmt.Errorf("permission %s is required", permission)
	}
	switch strings.TrimSpace(method) {
	case "capabilities":
		return map[string]any{
			"api_version":       CurrentPluginAPIVersion,
			"plugin_id":         id,
			"permissions":       manifest.Permissions,
			"host_capabilities": supportedHostCapabilities(),
			"methods": []string{
				"capabilities",
				"config.get",
				"log.write",
				"kv.get",
				"kv.set",
				"kv.delete",
				"event.publish",
				"notice.send",
				"sites.list",
				"action.invoke",
			},
		}, nil
	case "config.get":
		if err := require("config:read"); err != nil {
			return nil, err
		}
		return map[string]any{"config": rt.Config()}, nil
	case "log.write":
		if err := require("log:write"); err != nil {
			return nil, err
		}
		level := bridgeString(params, "level", logInfo)
		eventName := bridgeString(params, "event", "ui.bridge")
		message := bridgeString(params, "message", "")
		rt.Log(level, eventName, message, bridgeMap(params, "data"))
		return map[string]any{"ok": true}, nil
	case "kv.get":
		if err := require("kv:read"); err != nil {
			return nil, err
		}
		key := bridgeString(params, "key", "")
		if key == "" {
			return nil, errors.New("key is required")
		}
		var value any
		err := rt.KVGet(key, &value)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return map[string]any{"found": false}, nil
		}
		if err != nil {
			return nil, err
		}
		return map[string]any{"found": true, "value": value}, nil
	case "kv.set":
		if err := require("kv:write"); err != nil {
			return nil, err
		}
		key := bridgeString(params, "key", "")
		if key == "" {
			return nil, errors.New("key is required")
		}
		if err := rt.KVSet(key, params["value"]); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "kv.delete":
		if err := require("kv:delete"); err != nil {
			return nil, err
		}
		key := bridgeString(params, "key", "")
		if key == "" {
			return nil, errors.New("key is required")
		}
		if err := rt.KVDelete(key); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "event.publish":
		if err := require("event:publish"); err != nil {
			return nil, err
		}
		eventType := bridgeString(params, "type", "")
		if eventType == "" {
			return nil, errors.New("type is required")
		}
		rt.Publish(ctx, eventType, bridgeMap(params, "data"))
		return map[string]any{"published": true, "type": eventType}, nil
	case "notice.send":
		if err := require("notice:write"); err != nil {
			return nil, err
		}
		if err := rt.Notice(ctx, NoticeInput{
			Title: bridgeString(params, "title", ""),
			Body:  bridgeString(params, "body", ""),
			Level: bridgeString(params, "level", logInfo),
		}); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "sites.list":
		if err := require("site:read"); err != nil {
			return nil, err
		}
		return map[string]any{"sites": rt.SiteList()}, nil
	case "action.invoke":
		// 让插件视图（iframe dashboard）能调用插件声明的 action 拿实时数据，
		// 例如 CloudDrive2 助手面板调 info 取指标。action 在插件进程内执行，无需宿主权限门。
		action := bridgeString(params, "action", "")
		if action == "" {
			return nil, errors.New("action is required")
		}
		input, _ := params["input"].(map[string]any)
		return m.ExecuteAction(ctx, id, action, input)
	default:
		return nil, fmt.Errorf("unsupported bridge method %q", method)
	}
}

func bridgeString(params map[string]any, key, fallback string) string {
	value := strings.TrimSpace(fmt.Sprint(params[key]))
	if value == "" || value == "<nil>" {
		return fallback
	}
	return value
}

func bridgeMap(params map[string]any, key string) map[string]any {
	if v, ok := params[key].(map[string]any); ok {
		return v
	}
	return nil
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
	normalizeManifest(&manifest)
	availableOptional, missingOptional := evaluateOptionalHostCapabilities(manifest.OptionalHostCapabilities, supportedHostCapabilities())
	dependenciesStatus, canEnable, canEnableReason := m.evaluateDependencies(manifest.Dependencies, true)
	var runtimeStatus *RuntimeStatus
	if rs, ok := p.(interface{ RuntimeStatus() RuntimeStatus }); ok {
		status := rs.RuntimeStatus()
		runtimeStatus = &status
	} else if manifest.Builtin {
		runtimeStatus = &RuntimeStatus{State: RuntimeBuiltin, Managed: true}
	}
	return View{
		Manifest:                          manifest,
		AvailableOptionalHostCapabilities: availableOptional,
		MissingOptionalHostCapabilities:   missingOptional,
		DependenciesStatus:                dependenciesStatus,
		CanEnable:                         canEnable,
		CanEnableReason:                   canEnableReason,
		Enabled:                           st.Enabled,
		Installed:                         st.Installed,
		Config:                            cfg,
		RuntimeStatus:                     runtimeStatus,
		LastError:                         st.LastError,
		LastEventAt:                       st.LastEventAt,
		CreatedAt:                         st.CreatedAt,
		UpdatedAt:                         st.UpdatedAt,
	}, nil
}

func normalizeManifest(manifest *Manifest) {
	if manifest.APIVersion == "" {
		manifest.APIVersion = CurrentPluginAPIVersion
	}
	manifest.HostCapabilities = effectiveManifestHostCapabilities(*manifest)
	manifest.OptionalHostCapabilities = normalizeOptionalHostCapabilities(manifest.OptionalHostCapabilities)
	manifest.Dependencies = normalizeDependencies(manifest.Dependencies)
	if manifest.Permissions == nil {
		manifest.Permissions = []string{}
	}
	if manifest.Events == nil {
		manifest.Events = []EventSpec{}
	}
	if manifest.Actions == nil {
		manifest.Actions = []ActionSpec{}
	}
	if manifest.ConfigSchema == nil {
		manifest.ConfigSchema = []ConfigField{}
	}
	if manifest.Schedule == nil {
		manifest.Schedule = []ScheduleSpec{}
	}
	for i := range manifest.Actions {
		if manifest.Actions[i].InputSchema == nil {
			manifest.Actions[i].InputSchema = []ConfigField{}
		}
	}
	if manifest.Keywords == nil {
		manifest.Keywords = []string{}
	}
	if manifest.OptionalHostCapabilities == nil {
		manifest.OptionalHostCapabilities = []string{}
	}
	if manifest.Dependencies == nil {
		manifest.Dependencies = []PluginDependency{}
	}
	if manifest.Contributes != nil {
		if manifest.Contributes.Commands == nil {
			manifest.Contributes.Commands = []CommandContribution{}
		}
		if manifest.Contributes.Views == nil {
			manifest.Contributes.Views = []ViewContribution{}
		}
		if manifest.Contributes.Menus == nil {
			manifest.Contributes.Menus = []MenuContribution{}
		}
	}
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

// refreshSchedules 闁插秴缂撻弻鎰絻娴犲墎娈戦崗銊╁劥鐎规碍妞傛禒璇插閵嗗倸绠撶粵澶涚窗閸忓牏些闂勩倛顕氶幓鎺嶆瀹稿弶婀?job閿?// 閸愬秵瀵滆ぐ鎾冲 manifest + 闁板秶鐤?+ 閸氼垳鏁ら悩鑸碘偓渚€鍣搁弬鎵鐠佽埇鈧静nable/SaveConfig/Register/Start 閸у洩鐨熷銈冣偓?//
// refreshSchedules rebuilds all cron jobs for a plugin.
func (m *Manager) refreshSchedules(id string) {
	m.schedMu.Lock()
	sched := m.sched
	// 閸忓牊绔婚弮?job
	for _, name := range m.schedJobs[id] {
		if sched != nil {
			sched.Remove(name)
		}
	}
	m.schedJobs[id] = nil
	if sched == nil {
		m.schedMu.Unlock()
		return
	}
	p, err := m.plugin(id)
	if err != nil {
		m.schedMu.Unlock()
		return
	}
	manifest := p.Manifest()
	if len(manifest.Schedule) == 0 {
		m.schedMu.Unlock()
		return
	}
	m.schedMu.Unlock()

	// Read state/config outside the scheduler lock.
	st, cfg, err := m.stateAndConfig(p)
	if err != nil || !st.Enabled {
		return
	}
	for _, s := range manifest.Schedule {
		cron := effectiveCron(s, cfg)
		if strings.TrimSpace(cron) == "" {
			continue // 缁?cron = 鐠囥儲娼崑婊呮暏
		}
		name := "plugin:" + id + ":" + s.ID
		action, input := s.Action, s.Input
		if err := sched.AddFunc(name, cron, func() {
			if _, err := m.ExecuteAction(context.Background(), id, action, input); err != nil {
				slog.Error("scheduled plugin action failed", "plugin", id, "action", action, "err", err)
			}
		}); err != nil {
			slog.Error("register plugin schedule", "plugin", id, "schedule", s.ID, "cron", cron, "err", err)
			continue
		}
		m.schedMu.Lock()
		m.schedJobs[id] = append(m.schedJobs[id], name)
		m.schedMu.Unlock()
	}
}

// saveRuntimeConfig updates one runtime-owned config key and refreshes schedules.
func (m *Manager) saveRuntimeConfig(pluginID, key string, val any) error {
	p, err := m.plugin(pluginID)
	if err != nil {
		return err
	}
	st, cfg, err := m.stateAndConfig(p)
	if err != nil {
		return err
	}
	cfg[key] = val
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	st.ConfigJSON = string(raw)
	if err := m.op.SaveState(st); err != nil {
		return err
	}
	m.refreshSchedules(pluginID)
	return nil
}

// removeSchedules removes all cron jobs for a plugin without rebuilding them.
func (m *Manager) removeSchedules(id string) {
	m.schedMu.Lock()
	sched := m.sched
	names := m.schedJobs[id]
	m.schedJobs[id] = nil
	m.schedMu.Unlock()
	if sched == nil {
		return
	}
	for _, name := range names {
		sched.Remove(name)
	}
}

// effectiveCron resolves the active cron expression for a schedule.
func effectiveCron(s ScheduleSpec, cfg map[string]any) string {
	if s.CronConfigKey != "" {
		return strings.TrimSpace(pluginString(cfg[s.CronConfigKey]))
	}
	return strings.TrimSpace(s.Cron)
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
