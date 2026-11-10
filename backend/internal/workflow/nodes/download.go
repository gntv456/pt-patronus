// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package nodes

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

// AddDownloadNode 将 ecx.Torrents 推送到下载器（DownloadChain.Add）。
type AddDownloadNode struct {
	dlCh *chain.DownloadChain
	dlOp *oper.DownloaderOper
}

func NewAddDownloadNode(dlCh *chain.DownloadChain, dlOp *oper.DownloaderOper) *AddDownloadNode {
	return &AddDownloadNode{dlCh: dlCh, dlOp: dlOp}
}

func (n *AddDownloadNode) Type() string        { return "add_download" }
func (n *AddDownloadNode) Name() string        { return "添加下载" }
func (n *AddDownloadNode) Category() string    { return "下载" }
func (n *AddDownloadNode) Description() string { return "将资源集合推送到下载器" }
func (n *AddDownloadNode) Icon() string        { return "download" }
func (n *AddDownloadNode) DefaultParams() map[string]any {
	return map[string]any{
		"downloader_id": float64(0), "save_path": "", "category": "workflow",
		"tags": "workflow", "paused": false, "max_count": float64(0),
	}
}
func (n *AddDownloadNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "downloader_id", Label: "下载器", Type: "downloader_select", Help: "留空=默认下载器"},
		{Name: "save_path", Label: "保存路径", Type: "text"},
		{Name: "category", Label: "分类", Type: "text"},
		{Name: "tags", Label: "标签", Type: "text", Help: "逗号分隔"},
		{Name: "paused", Label: "添加后暂停", Type: "switch"},
		{Name: "max_count", Label: "最多下载条数", Type: "number", Help: "0=全部"},
	}
}

func (n *AddDownloadNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	dlID := uint(workflow.ParamInt(params, "downloader_id"))
	if dlID == 0 {
		dlID = n.defaultDownloaderID()
		if dlID == 0 {
			return fmt.Errorf("未配置下载器")
		}
	}
	savePath := workflow.ParamString(params, "save_path")
	category := workflow.ParamString(params, "category")
	tags := workflow.ParamStringSlice(params, "tags")
	paused := workflow.ParamBool(params, "paused")
	maxCount := workflow.ParamInt(params, "max_count")

	pushed := 0
	for i, t := range ecx.Torrents {
		if t.DownloadURL == "" {
			continue
		}
		if maxCount > 0 && i >= maxCount {
			break
		}
		opts := downloader.AddOptions{
			URL: t.DownloadURL, Category: category, Tags: tags, SavePath: savePath, Paused: paused,
			SiteID: t.SiteID,
		}
		if err := n.dlCh.Add(ctx, dlID, opts); err != nil {
			slog.Warn("workflow add_download failed", "title", t.Title, "err", err)
			continue
		}
		pushed++
	}
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("已推送 %d 条到下载器", pushed))
	return nil
}

func (n *AddDownloadNode) defaultDownloaderID() uint {
	list, err := n.dlOp.List()
	if err != nil || len(list) == 0 {
		return 0
	}
	for _, d := range list {
		if d.IsDefault {
			return d.ID
		}
	}
	return list[0].ID
}

// AddSubscribeNode 用 ecx.Torrents（取首条标题）创建媒体订阅。
type AddSubscribeNode struct {
	subCh *chain.SubscribeChain
}

func NewAddSubscribeNode(subCh *chain.SubscribeChain) *AddSubscribeNode {
	return &AddSubscribeNode{subCh: subCh}
}

func (n *AddSubscribeNode) Type() string        { return "add_subscribe" }
func (n *AddSubscribeNode) Name() string        { return "添加订阅" }
func (n *AddSubscribeNode) Category() string    { return "下载" }
func (n *AddSubscribeNode) Description() string { return "按资源标题创建媒体订阅" }
func (n *AddSubscribeNode) Icon() string        { return "subscribe" }
func (n *AddSubscribeNode) DefaultParams() map[string]any {
	return map[string]any{"title": "", "type": "movie"}
}
func (n *AddSubscribeNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "title", Label: "标题", Type: "text", Help: "留空则用资源集合首条标题"},
		{Name: "type", Label: "类型", Type: "select", Options: []workflow.ParamOption{
			{Label: "电影", Value: "movie"}, {Label: "剧集", Value: "tv"},
		}},
	}
}

func (n *AddSubscribeNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	title := workflow.ParamString(params, "title")
	if title == "" && len(ecx.Torrents) > 0 {
		title = ecx.Torrents[0].Title
	}
	if title == "" {
		return fmt.Errorf("无标题可订阅")
	}
	tp := workflow.ParamString(params, "type")
	if tp == "" {
		tp = "movie"
	}
	if _, err := n.subCh.Create(ctx, &models.Subscription{Title: title, Type: tp, Category: tp, Source: "workflow"}); err != nil {
		return fmt.Errorf("创建订阅失败: %w", err)
	}
	ecx.Messages = append(ecx.Messages, "已创建订阅："+title)
	return nil
}
