package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
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

// OfficialMarketSource 是 PTPatronus 官方插件市场仓库（github.com 页面地址）。
// normalizeMarketSource（market.go）会将其转换为 raw.githubusercontent.com 上的 plugin-market.json。
// 全新安装时由 EnsureDefaultMarketSource 种子化为默认市场源；用户可删除或追加第三方源。
const OfficialMarketSource = "https://github.com/gntv456/ptpatronus-plugin-market"

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
// ErrTrustConfigUnreadable：受信公钥记录读不出来（系统配置读失败或被写坏）。
// 从前解析失败是 `_ = json.Unmarshal(...)`——连日志都不留，界面于是把「读不出」画成
// 「一条公钥都没配」，而信任配置的保存是整份替换：用户点一次保存就把真公钥清空，
// 接下来要么装不上任何包，要么为了装上而勾上「允许未签名」——安全闸门被一次读故障劝退。
var ErrTrustConfigUnreadable = errors.New("插件签名信任配置读不出来")

// TrustConfigSnapshot 读出信任配置并带回「读不出来」这个事实；GetTrustConfig 是它的薄壳。
func (m *Manager) TrustConfigSnapshot() (TrustConfig, error) {
	cfg := TrustConfig{OfficialKey: OfficialMarketPublicKey, AllowUnsigned: m.AllowUnsigned(), TrustedPublicKeys: []string{}}
	if m.cfgOp == nil {
		return cfg, nil
	}
	raw, err := m.cfgOp.Get(cfgKeyTrustedPublicKeys)
	if err != nil {
		return cfg, fmt.Errorf("%w: %v", ErrTrustConfigUnreadable, err)
	}
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &cfg.TrustedPublicKeys); err != nil {
			return cfg, fmt.Errorf("%w: 受信公钥记录不是合法 JSON: %v", ErrTrustConfigUnreadable, err)
		}
	}
	return cfg, nil
}

func (m *Manager) GetTrustConfig() TrustConfig {
	cfg, _ := m.TrustConfigSnapshot()
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
	subscribed    map[string]map[string]int // eventType → bus handle（Unsubscribe 用，治 reload/卸载泄漏）
	op            *oper.PluginOper
	marketOp      *oper.PluginMarketOper // 市场运营层（安装量/收藏/多实例/天序）；nil 时相关 API 空转
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
	mediaSearcher MediaSearcher     // 由 app.go 注入（MediaChain 适配器），nil 时 /media/search 返回空
	hostTools     *HostToolRegistry // 由 app.go 注入（registerPluginHostTools 批量注册 chain 能力）；nil 时 /tools 返回空
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
		subscribed: make(map[string]map[string]int),
		op:         oper.NewPluginOper(db),
		kv:         kv,
		sites:      sites,
		bus:        bus,
		notice:     noticeSvc,
		schedJobs:  make(map[string][]string),
	}
	// Built-in feature plugins have been moved to the open plugin market:
	// https://github.com/gntv456/ptpatronus-plugin-market
	// Install them from the market UI instead of compiling into the host.
	// MCP server plugin remains host-registered in app.go (needs in-process mcp.Server).
	return m
}

// SetMediaSearcher 注入媒体检索适配器（app.go 用 MediaChain 包装），供 Host API /media/search 使用。
func (m *Manager) SetMediaSearcher(s MediaSearcher) { m.mediaSearcher = s }

// SetHostTools 注入宿主工具注册表（app.go 用 registerPluginHostTools 批量注册 chain 能力），
// 供 Host API GET /tools、POST /tools/{name} 与 Bridge tool.list/tool.call 使用。应在 Start 前调用。
func (m *Manager) SetHostTools(r *HostToolRegistry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hostTools = r
}

// HostTools 返回已注入的工具注册表（未注入返回 nil；调用方需 nil 检查）。
func (m *Manager) HostTools() *HostToolRegistry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hostTools
}

// MediaSearch 供 externalHostServer 的 /media/search 端点调用；未注入时返回空。
func (m *Manager) MediaSearch(ctx context.Context, keyword string, year int) ([]MediaHit, error) {
	if m.mediaSearcher == nil {
		return []MediaHit{}, nil
	}
	return m.mediaSearcher.Search(ctx, keyword, year)
}

func (m *Manager) plugin(id string) (Plugin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.plugins[id]
	if !ok {
		return nil, fmt.Errorf("%w: plugin %s not found", ErrPluginNotRegistered, id)
	}
	return p, nil
}
