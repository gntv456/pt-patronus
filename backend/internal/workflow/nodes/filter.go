// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package nodes

import (
	"context"
	"fmt"
	"sort"

	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

// FilterTorrentsNode 按 include/exclude、大小、分辨率、免费过滤 ecx.Torrents（原地裁剪）。
type FilterTorrentsNode struct{}

func NewFilterTorrentsNode() *FilterTorrentsNode { return &FilterTorrentsNode{} }

func (n *FilterTorrentsNode) Type() string     { return "filter_torrents" }
func (n *FilterTorrentsNode) Name() string     { return "过滤资源" }
func (n *FilterTorrentsNode) Category() string { return "过滤" }
func (n *FilterTorrentsNode) Description() string {
	return "按关键词/大小/分辨率/免费/官种过滤资源集合"
}
func (n *FilterTorrentsNode) Icon() string { return "filter" }
func (n *FilterTorrentsNode) DefaultParams() map[string]any {
	return map[string]any{
		"include": "", "exclude": "", "min_size": "", "max_size": "",
		"resolution": "", "free_only": false, "official_only": false,
	}
}
func (n *FilterTorrentsNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "include", Label: "标题包含", Type: "text", Help: "任一命中即保留，| 分隔多个"},
		{Name: "exclude", Label: "标题排除", Type: "text", Help: "任一命中即丢弃，| 分隔多个"},
		{Name: "min_size", Label: "最小体积(GB)", Type: "number", Help: "0=不限"},
		{Name: "max_size", Label: "最大体积(GB)", Type: "number", Help: "0=不限"},
		{Name: "resolution", Label: "分辨率", Type: "select", Options: []workflow.ParamOption{
			{Label: "不限", Value: ""}, {Label: "1080p", Value: "1080p"}, {Label: "2160p/4K", Value: "2160p"},
		}},
		{Name: "free_only", Label: "仅免费", Type: "switch"},
		{Name: "official_only", Label: "仅官种", Type: "switch", Help: "只保留官方/官种/官组发布（标题或标签命中）"},
	}
}

func (n *FilterTorrentsNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	include := workflow.ParamString(params, "include")
	exclude := workflow.ParamString(params, "exclude")
	res := workflow.ParamString(params, "resolution")
	freeOnly := workflow.ParamBool(params, "free_only")
	officialOnly := workflow.ParamBool(params, "official_only")
	minBytes := int64(workflow.ParamInt(params, "min_size")) * 1024 * 1024 * 1024
	maxBytes := int64(workflow.ParamInt(params, "max_size")) * 1024 * 1024 * 1024

	kept := ecx.Torrents[:0] // 原地过滤
	for _, t := range ecx.Torrents {
		if !matchAny(t.Title, include) || !matchNone(t.Title, exclude) {
			continue
		}
		if res != "" && !matchResolution(t.Title, res) {
			continue
		}
		if freeOnly && !t.Free {
			continue
		}
		if officialOnly && !site.IsOfficial(t.Title, t.Labels) {
			continue
		}
		if minBytes > 0 || maxBytes > 0 {
			sz := site.ParseBytes(t.Size)
			if minBytes > 0 && sz < minBytes {
				continue
			}
			if maxBytes > 0 && sz > maxBytes {
				continue
			}
		}
		kept = append(kept, t)
	}
	ecx.Torrents = kept
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("过滤后剩余 %d 条资源", len(kept)))
	return nil
}

// LimitNode 取资源集合前 N 条（按做种数降序），丢弃其余。
type LimitNode struct{}

func NewLimitNode() *LimitNode { return &LimitNode{} }

func (n *LimitNode) Type() string        { return "limit" }
func (n *LimitNode) Name() string        { return "取前 N 条" }
func (n *LimitNode) Category() string    { return "过滤" }
func (n *LimitNode) Description() string { return "按做种数降序取前 N 条资源" }
func (n *LimitNode) Icon() string        { return "limit" }
func (n *LimitNode) DefaultParams() map[string]any {
	return map[string]any{"count": float64(1)}
}
func (n *LimitNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "count", Label: "数量 N", Type: "number", Required: true, Default: float64(1)},
	}
}

func (n *LimitNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	count := workflow.ParamInt(params, "count")
	if count <= 0 {
		count = 1
	}
	sort.SliceStable(ecx.Torrents, func(i, j int) bool {
		return ecx.Torrents[i].Seeders > ecx.Torrents[j].Seeders
	})
	if len(ecx.Torrents) > count {
		ecx.Torrents = ecx.Torrents[:count]
	}
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("取前 %d 条资源", len(ecx.Torrents)))
	return nil
}
