// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// MarshalConfigMap map → JSON（handler 存实例配置用）。
func MarshalConfigMap(cfg map[string]any) (string, error) {
	b, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// ---- 插件市场运营层（对标 MSaber 秘术宝阁：百鸟朝凤/百宝袋/分神/天序） ----

// currentMarketUserID 收藏归属用户（当前单管理员部署形态恒 0；多用户后接 auth uid）。
func currentMarketUserID() uint { return 0 }

// MarketCharts 百鸟朝凤（老树洞=累计安装 Top30；蹦蹦新窝=最近安装 Top30）。
func (m *Manager) MarketCharts(ctx context.Context) (*models.MarketChartsView, error) {
	m.mu.RLock()
	mop := m.marketOp
	m.mu.RUnlock()
	out := &models.MarketChartsView{
		Disciples: []models.ChartRow{},
		Newcomers: []models.ChartRow{},
	}
	if mop == nil {
		return out, nil
	}
	rows, err := mop.TopInstalled(30)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out.Disciples = append(out.Disciples, models.ChartRow{PluginID: r.PluginID, Installs: r.Installs})
	}
	recent, err := mop.RecentInstalls(30)
	if err != nil {
		return nil, err
	}
	for _, r := range recent {
		t := time.Time{}
		if v, err := time.Parse("2006-01-02 15:04:05.999999999-07:00", r.FirstInstalled); err == nil {
			t = v
		}
		out.Newcomers = append(out.Newcomers, models.ChartRow{PluginID: r.PluginID, Installs: r.Installs, LastInstalled: t})
	}
	return out, nil
}

// MarketFavorites 百宝袋：收藏插件的市场详情（拉市场索引后按收藏集过滤）。
// 返回 MarketPlugin 列表（与市场浏览同构）；收藏了但市场已下架的插件以占位行返回（id+标记）。
func (m *Manager) MarketFavorites(ctx context.Context) ([]MarketPlugin, error) {
	m.mu.RLock()
	mop := m.marketOp
	m.mu.RUnlock()
	if mop == nil {
		return []MarketPlugin{}, nil
	}
	favs, err := mop.Favorites(currentMarketUserID())
	if err != nil {
		return nil, err
	}
	if len(favs) == 0 {
		return []MarketPlugin{}, nil
	}
	idx, err := m.FetchMarketMulti(ctx, nil)
	if err != nil {
		return nil, err
	}
	favSet := make(map[string]bool, len(favs))
	for _, id := range favs {
		favSet[id] = true
	}
	out := make([]MarketPlugin, 0, len(favs))
	for _, p := range idx.Plugins {
		if favSet[p.ID] {
			out = append(out, p)
			delete(favSet, p.ID)
		}
	}
	// 市场已下架但仍有收藏：占位行（前端提示"已下架"）。
	for id := range favSet {
		out = append(out, MarketPlugin{ID: id, Name: id, Favorited: true})
	}
	// 按收藏时间序（favs 顺序）排列。
	sort.SliceStable(out, func(i, j int) bool {
		return favIndex(favs, out[i].ID) < favIndex(favs, out[j].ID)
	})
	return out, nil
}

func favIndex(favs []string, id string) int {
	for i, f := range favs {
		if f == id {
			return i
		}
	}
	return len(favs)
}

// FavoritePlugin / UnfavoritePlugin 收藏开关（幂等）。
func (m *Manager) FavoritePlugin(pluginID string) error {
	m.mu.RLock()
	mop := m.marketOp
	m.mu.RUnlock()
	if mop == nil {
		return ErrMarketOperUnavailable
	}
	return mop.Favorite(currentMarketUserID(), pluginID)
}

func (m *Manager) UnfavoritePlugin(pluginID string) error {
	m.mu.RLock()
	mop := m.marketOp
	m.mu.RUnlock()
	if mop == nil {
		return ErrMarketOperUnavailable
	}
	return mop.Unfavorite(currentMarketUserID(), pluginID)
}

// ---- 分神（多实例）与天序（拖拽排序） ----

var ErrMarketOperUnavailable = errMarketOper{}

// ErrSingleInstance 让「这个插件是单实例插件」这一档能被接口层认出来。
// 从前 CreatePluginInstance 的所有失败（含这句中文）都被压成 400「请求参数有误」，
// 而用户没有任何可检查的输入——他要的是「直接用主实例，别建分身」。
var ErrSingleInstance = errors.New("单实例插件")

type errMarketOper struct{}

func (errMarketOper) Error() string { return "市场运营层未装配" }

// SingleInstance 判定插件是否单实例（Builtin 或 Manifest.Contributes 声明；
// 简化约定：内置插件视为单实例——它们由宿主单点管理；市场插件默认可分身）。
func (m *Manager) SingleInstance(pluginID string) bool {
	m.mu.RLock()
	p, ok := m.plugins[pluginID]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	return p.Manifest().Builtin
}

// CreatePluginInstance 分神：为可多实例插件建独立配置实例。
func (m *Manager) CreatePluginInstance(pluginID, instanceKey, configJSON string) (*models.PluginInstance, error) {
	m.mu.RLock()
	mop := m.marketOp
	m.mu.RUnlock()
	if mop == nil {
		return nil, ErrMarketOperUnavailable
	}
	if m.SingleInstance(pluginID) {
		return nil, errSingleInstance{pluginID}
	}
	return mop.CreateInstance(pluginID, instanceKey, configJSON)
}

type errSingleInstance struct{ id string }

func (e errSingleInstance) Error() string {
	return "插件 " + e.id + " 为单实例插件，不支持分身"
}

// Unwrap 挂上哨兵：文案不变（可能有用例钉着原句），但接口层终于能 errors.Is 认出这一档。
func (e errSingleInstance) Unwrap() error { return ErrSingleInstance }

// PluginInstances 列实例（按天序）。
func (m *Manager) PluginInstances(pluginID string) ([]models.PluginInstance, error) {
	m.mu.RLock()
	mop := m.marketOp
	m.mu.RUnlock()
	if mop == nil {
		return []models.PluginInstance{}, nil
	}
	return mop.Instances(pluginID)
}

// DeletePluginInstance 归墟实例（幂等）。
func (m *Manager) DeletePluginInstance(pluginID, instanceKey string) error {
	m.mu.RLock()
	mop := m.marketOp
	m.mu.RUnlock()
	if mop == nil {
		return ErrMarketOperUnavailable
	}
	if _, err := mop.GetInstance(pluginID, instanceKey); err != nil {
		return err
	}
	return mop.DeleteInstance(pluginID, instanceKey)
}

// SavePluginInstanceConfig 更新实例配置/启停。
func (m *Manager) SavePluginInstanceConfig(pluginID, instanceKey, configJSON string, enabled bool) error {
	m.mu.RLock()
	mop := m.marketOp
	m.mu.RUnlock()
	if mop == nil {
		return ErrMarketOperUnavailable
	}
	return mop.SaveInstanceConfig(pluginID, instanceKey, configJSON, enabled)
}

// ReorderPluginInstances 天序：按拖拽后顺序重写 sort_order（服务端持久化，跨端同步）。
func (m *Manager) ReorderPluginInstances(pluginID string, ids []uint) error {
	m.mu.RLock()
	mop := m.marketOp
	m.mu.RUnlock()
	if mop == nil {
		return ErrMarketOperUnavailable
	}
	return mop.ReorderInstances(pluginID, ids)
}
