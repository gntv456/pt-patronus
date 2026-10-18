package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

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
// ErrMarketSourcesUnreadable：市场源**读不出来**（系统配置读失败，或那条记录被写坏）。
// 它与「用户把源清空了」必须分开——两者在界面上都是空列表，而界面保存是整份替换：
// 把读不出来当成清空，一次保存就把全部源抹掉，还会回一句「已保存 N 个市场源」。
var ErrMarketSourcesUnreadable = errors.New("插件市场源读不出来")

// MarketSourcesSnapshot 读出持久化的市场源，并带回「读不出来」这个事实。
// GetMarketSources 是它的薄壳（内部拉取路径容忍无源，界面不容忍）。
func (m *Manager) MarketSourcesSnapshot() ([]string, error) {
	m.mu.RLock()
	cfgOp := m.cfgOp
	m.mu.RUnlock()
	if cfgOp == nil {
		return nil, nil
	}
	raw, err := cfgOp.Get(pluginMarketSourcesKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMarketSourcesUnreadable, err)
	}
	var sources []string
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &sources); err != nil {
			slog.Warn("parse plugin market sources", "err", err, "raw", raw)
			return nil, fmt.Errorf("%w: 记录内容不是合法的源列表: %v", ErrMarketSourcesUnreadable, err)
		}
	}
	if len(sources) == 0 {
		// 迁移：新版列表为空时回退读旧版单源 key。
		if old, oerr := cfgOp.Get(pluginMarketSourceKey); oerr == nil && strings.TrimSpace(old) != "" {
			sources = []string{old}
		}
	}
	return normalizeSources(sources), nil
}

func (m *Manager) GetMarketSources() []string {
	// 内部取数路径本来就容忍「一个源都没有」，读不出来时沿用旧行为（空列表），
	// 但界面条走 MarketSourcesSnapshot——那里空列表会被当成用户意图写回去。
	sources, _ := m.MarketSourcesSnapshot()
	return sources
}

// SetMarketSources 持久化插件市场源列表（normalize 后写入；空列表即清除，回退本地 plugin-market.json）。
// 同时清空旧版单源 key 完成迁移。
//
// 不变式：空列表必须持久化为字面量 "[]"（而非删除 key）。EnsureDefaultMarketSource 据此区分
// 「用户显式清空所有源」（"[]"，尊重、不重新播种官方源）与「全新安装从未配置」（key 缺失，需播种）。
// 若改为删 key，将导致用户删掉的官方源在下次启动被"复活"。
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

// EnsureDefaultMarketSource 在全新安装（从未配置过市场源）时种子化官方市场仓库。
// 判定依据（无需独立 flag）：直接读持久化原始值——
//   - 新 key 存在任意非空值（含 "[]"＝用户显式清空）→ 已被用户管理，跳过；
//   - 否则旧 key 非空（老版本迁移态）→ 尊重既有源，跳过；
//   - 否则视为全新安装：写入官方源。
//
// 注意：必须读原始 key（cfgOp.Get），不能用 GetMarketSources()——后者会把 "[]"（用户清空）
// 与 key 缺失（全新）都塌缩成空切片，无法区分。不变式见 SetMarketSources 注释。
// 失败仅记录日志不 panic；cfgOp 未注入时安全返回（供测试与未完成初始化的场景）。
func (m *Manager) EnsureDefaultMarketSource() {
	m.mu.RLock()
	cfgOp := m.cfgOp
	m.mu.RUnlock()
	if cfgOp == nil {
		return
	}
	if raw, err := cfgOp.Get(pluginMarketSourcesKey); err == nil && strings.TrimSpace(raw) != "" {
		return // 用户已管理（含显式清空 "[]"）
	}
	if old, err := cfgOp.Get(pluginMarketSourceKey); err == nil && strings.TrimSpace(old) != "" {
		return // 老版本单源迁移态：保留其源，交由 GetMarketSources 回退读取
	}
	if err := m.SetMarketSources([]string{OfficialMarketSource}); err != nil {
		slog.Warn("seed default plugin market source", "err", err)
		return
	}
	slog.Info("seeded default plugin market source", "source", OfficialMarketSource)
}
