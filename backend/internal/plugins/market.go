package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
)

const maxPluginArchiveSize = 50 << 20

func (m *Manager) FetchMarket(ctx context.Context, source string) (*MarketIndex, error) {
	idx, err := m.fetchRawMarket(ctx, source)
	if err != nil {
		return nil, err
	}
	m.finalizeIndex(idx)
	return idx, nil
}

// fetchRawMarket 取单个源的原始索引：归一化地址、读取、反序列化，并给每条 plugin 打上
// 解析后的 Source（供浏览/安装时回传定位其 archive）。不做兼容性/依赖后处理（交由 finalizeIndex）。
func (m *Manager) fetchRawMarket(ctx context.Context, source string) (*MarketIndex, error) {
	source = m.resolveMarketSource(source)
	raw, err := readSource(ctx, source, maxPluginArchiveSize)
	if err != nil {
		// 本地默认市场源（data/plugin-market.json）不存在 = 用户未配置市场源：
		// 返回空市场让前端引导点「设置仓库」配置 GitHub 源，而非把"no such file"当错误抛给用户。
		// （http/远程源失败仍按错误返回——只有本地文件缺失才视为"未配置"。）
		if errors.Is(err, fs.ErrNotExist) {
			return &MarketIndex{Plugins: []MarketPlugin{}}, nil
		}
		return nil, err
	}
	var idx MarketIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil, fmt.Errorf("parse market index: %w", err)
	}
	idx.APIVersion = CurrentPluginAPIVersion
	idx.AppVersion = currentAppVersion()
	idx.HostCapabilities = supportedHostCapabilities()
	for i := range idx.Plugins {
		idx.Plugins[i].Source = source
	}
	return &idx, nil
}

// finalizeIndex 对市场索引做兼容性评估、依赖/安装计划等后处理（原地修改）。
// 运营层字段（InstallCount/Favorited）一并填充：弟子数进索引、收藏态标记当前用户。
func (m *Manager) finalizeIndex(idx *MarketIndex) {
	m.mu.RLock()
	mop := m.marketOp
	m.mu.RUnlock()
	installCounts := map[string]int64{}
	favSet := map[string]bool{}
	if mop != nil {
		if rows, err := mop.TopInstalled(100); err == nil {
			for _, r := range rows {
				installCounts[r.PluginID] = r.Installs
			}
		}
		if favs, err := mop.Favorites(currentMarketUserID()); err == nil {
			for _, id := range favs {
				favSet[id] = true
			}
		}
	}
	for i := range idx.Plugins {
		if idx.Plugins[i].APIVersion == "" {
			idx.Plugins[i].APIVersion = CurrentPluginAPIVersion
		}
		idx.Plugins[i].HostCapabilities = effectiveMarketHostCapabilities(idx.Plugins[i])
		idx.Plugins[i].OptionalHostCapabilities = normalizeOptionalHostCapabilities(idx.Plugins[i].OptionalHostCapabilities)
		idx.Plugins[i].Dependencies = normalizeDependencies(idx.Plugins[i].Dependencies)
		compatibility := evaluateMarketCompatibility(idx.Plugins[i])
		idx.Plugins[i].Compatible = compatibility.Compatible
		idx.Plugins[i].CompatibilityReason = compatibility.Reason
		idx.Plugins[i].AvailableOptionalHostCapabilities, idx.Plugins[i].MissingOptionalHostCapabilities = evaluateOptionalHostCapabilities(idx.Plugins[i].OptionalHostCapabilities, idx.HostCapabilities)
		idx.Plugins[i].TrustLevel = trustLevel(idx.PublicKey, idx.Plugins[i])
		idx.Plugins[i].Installed = m.hasInstalled(idx.Plugins[i].ID)
		idx.Plugins[i].InstalledVersion = m.installedVersion(idx.Plugins[i].ID)
		idx.Plugins[i].UpdateAvailable = idx.Plugins[i].Installed && idx.Plugins[i].InstalledVersion != "" && idx.Plugins[i].InstalledVersion != idx.Plugins[i].Version
		idx.Plugins[i].InstallCount = installCounts[idx.Plugins[i].ID]
		idx.Plugins[i].Favorited = favSet[idx.Plugins[i].ID]
	}
	planner := newMarketInstallPlanner(m, idx)
	for i := range idx.Plugins {
		item := &idx.Plugins[i]
		if item.Compatible {
			result := planner.resolve(item.ID, nil)
			item.DependenciesStatus = result.statuses
			item.DependenciesSatisfied = result.installable
			item.InstallPlan = result.plan
			item.InstallReason = result.reason
			item.Installable = result.installable
			continue
		}
		statuses, _, depsSatisfied, reason := planner.resolveDependencies(item, []string{item.ID})
		item.DependenciesStatus = statuses
		item.DependenciesSatisfied = depsSatisfied
		item.InstallPlan = []InstallPlanStep{}
		item.Installable = false
		item.InstallReason = item.CompatibilityReason
		if item.InstallReason == "" {
			item.InstallReason = reason
		}
	}
}

// FetchMarketMulti 聚合多个市场源：逐源拉取原始索引，按 ID 合并（先列者优先），
// 统一做兼容性/依赖后处理。sources 为空时解析为持久化配置，再退本地默认源。
// 单个源失败记日志并跳过（容错，单仓挂掉不影响其它）；全部失败才返回首个错误。
func (m *Manager) FetchMarketMulti(ctx context.Context, sources []string) (*MarketIndex, error) {
	resolved := normalizeSources(sources)
	if len(resolved) == 0 {
		resolved = m.GetMarketSources()
	}
	if len(resolved) == 0 {
		if def := m.DefaultMarketSource(); def != "" {
			resolved = []string{def}
		}
	}

	merged := &MarketIndex{
		APIVersion:       CurrentPluginAPIVersion,
		AppVersion:       currentAppVersion(),
		HostCapabilities: supportedHostCapabilities(),
	}
	var groups [][]MarketPlugin
	var firstErr error
	for _, src := range resolved {
		idx, err := m.fetchRawMarket(ctx, src)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			slog.Warn("fetch market source skipped", "source", src, "err", err)
			continue
		}
		groups = append(groups, idx.Plugins)
	}
	merged.Plugins = mergeMarketPlugins(merged.Plugins, groups...)
	if len(merged.Plugins) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return merged, nil // 无源且无默认源：空索引，前端显示"暂无市场数据"
	}
	m.finalizeIndex(merged)
	return merged, nil
}

// mergeMarketPlugins 把多组 plugin 按 ID 合并进 out（先列者优先；跳过空 ID），返回合并结果。
// 纯函数，便于单测；不做兼容性/依赖后处理（调用方负责 finalize）。
func mergeMarketPlugins(out []MarketPlugin, groups ...[]MarketPlugin) []MarketPlugin {
	seen := make(map[string]struct{}, len(out))
	for _, p := range out {
		if p.ID != "" {
			seen[p.ID] = struct{}{}
		}
	}
	for _, group := range groups {
		for _, p := range group {
			if p.ID == "" {
				continue
			}
			if _, ok := seen[p.ID]; ok {
				continue // 先列者优先：已存在则跳过
			}
			seen[p.ID] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}

func trustLevel(publicKey string, item MarketPlugin) string {
	signed := strings.TrimSpace(publicKey) != "" && strings.TrimSpace(item.Signature) != ""
	verifiedPublisher := item.Publisher != nil && item.Publisher.Verified
	switch {
	case signed && verifiedPublisher:
		return "verified-signed"
	case signed:
		return "signed"
	case verifiedPublisher:
		return "publisher-verified"
	default:
		return "unsigned"
	}
}

