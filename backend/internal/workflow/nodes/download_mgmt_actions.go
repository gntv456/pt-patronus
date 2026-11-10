// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package nodes

import (
	"context"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

func NewListTorrentsNode(dlCh *chain.DownloadChain, dlOp *oper.DownloaderOper) *dlMgmtNode {
	return &dlMgmtNode{
		typ: "list_torrents", name: "列出种子", desc: "列出下载器任务，哈希写入变量供后续节点使用", icon: "list",
		dlCh: dlCh, dlOp: dlOp,
		schema: []workflow.ParamField{
			{Name: "downloader_id", Label: "下载器", Type: "downloader_select"},
			{Name: "state", Label: "状态过滤", Type: "text", Help: "留空=全部；如 downloading/paused/completed"},
			{Name: "category", Label: "分类过滤", Type: "text", Help: "留空=全部"},
		},
		defaults: map[string]any{"downloader_id": float64(0), "state": "", "category": ""},
		run: func(ctx context.Context, dl *chain.DownloadChain, id uint, _ []string, p map[string]any, ecx *workflow.Context) error {
			if dl == nil {
				return fmt.Errorf("下载器服务未初始化")
			}
			list, err := dl.List(ctx, id)
			if err != nil {
				return fmt.Errorf("列出失败: %w", err)
			}
			state := strings.ToLower(workflow.ParamString(p, "state"))
			category := workflow.ParamString(p, "category")
			hashes := make([]string, 0, len(list))
			kept := 0
			for _, t := range list {
				if state != "" && !strings.Contains(strings.ToLower(t.State), state) {
					continue
				}
				if category != "" && t.Category != category {
					continue
				}
				if t.Hash != "" {
					hashes = append(hashes, t.Hash)
				}
				kept++
			}
			if ecx.Variables == nil {
				ecx.Variables = map[string]any{}
			}
			ecx.Variables["torrent_hashes"] = strings.Join(hashes, ",")
			ecx.Variables["torrent_count"] = kept
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("列出 %d 个任务（共 %d）", kept, len(list)))
			return nil
		},
	}
}

func NewPauseTorrentsNode(dlCh *chain.DownloadChain, dlOp *oper.DownloaderOper) *dlMgmtNode {
	s, d := hashSchema()
	return &dlMgmtNode{typ: "pause_torrents", name: "暂停下载", desc: "暂停指定的下载任务", icon: "pause",
		dlCh: dlCh, dlOp: dlOp, schema: s, defaults: d,
		run: hashAction("暂停", func(ctx context.Context, dl *chain.DownloadChain, id uint, h []string) error {
			return dl.Pause(ctx, id, h)
		})}
}

func NewResumeTorrentsNode(dlCh *chain.DownloadChain, dlOp *oper.DownloaderOper) *dlMgmtNode {
	s, d := hashSchema()
	return &dlMgmtNode{typ: "resume_torrents", name: "恢复下载", desc: "恢复指定的下载任务", icon: "resume",
		dlCh: dlCh, dlOp: dlOp, schema: s, defaults: d,
		run: hashAction("恢复", func(ctx context.Context, dl *chain.DownloadChain, id uint, h []string) error {
			return dl.Resume(ctx, id, h)
		})}
}

func NewDeleteTorrentsNode(dlCh *chain.DownloadChain, dlOp *oper.DownloaderOper) *dlMgmtNode {
	return &dlMgmtNode{
		typ: "delete_torrents", name: "删除下载", desc: "删除指定的下载任务（可选删除文件）", icon: "delete",
		dlCh: dlCh, dlOp: dlOp,
		schema: []workflow.ParamField{
			{Name: "downloader_id", Label: "下载器", Type: "downloader_select"},
			{Name: "hashes", Label: "任务哈希", Type: "textarea", Help: "逗号/换行分隔，支持 {{var.x}}；留空用上一步结果"},
			{Name: "delete_files", Label: "同时删除文件", Type: "switch"},
		},
		defaults: map[string]any{"downloader_id": float64(0), "hashes": "", "delete_files": false},
		run: func(ctx context.Context, dl *chain.DownloadChain, id uint, h []string, p map[string]any, ecx *workflow.Context) error {
			if len(h) == 0 {
				return fmt.Errorf("未提供任务哈希")
			}
			delFiles := workflow.ParamBool(p, "delete_files")
			if err := dl.Delete(ctx, id, h, delFiles); err != nil {
				return fmt.Errorf("删除失败: %w", err)
			}
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("已删除 %d 个任务", len(h)))
			return nil
		},
	}
}

func NewRecheckTorrentsNode(dlCh *chain.DownloadChain, dlOp *oper.DownloaderOper) *dlMgmtNode {
	s, d := hashSchema()
	return &dlMgmtNode{typ: "recheck_torrents", name: "强制校验", desc: "强制重新校验指定任务", icon: "recheck",
		dlCh: dlCh, dlOp: dlOp, schema: s, defaults: d,
		run: hashAction("校验", func(ctx context.Context, dl *chain.DownloadChain, id uint, h []string) error {
			return dl.Recheck(ctx, id, h)
		})}
}

func NewReannounceTorrentsNode(dlCh *chain.DownloadChain, dlOp *oper.DownloaderOper) *dlMgmtNode {
	s, d := hashSchema()
	return &dlMgmtNode{typ: "reannounce_torrents", name: "重新汇报", desc: "向 Tracker 重新汇报指定任务", icon: "reannounce",
		dlCh: dlCh, dlOp: dlOp, schema: s, defaults: d,
		run: hashAction("汇报", func(ctx context.Context, dl *chain.DownloadChain, id uint, h []string) error {
			return dl.Reannounce(ctx, id, h)
		})}
}

func NewForceStartNode(dlCh *chain.DownloadChain, dlOp *oper.DownloaderOper) *dlMgmtNode {
	return &dlMgmtNode{
		typ: "force_start", name: "强制开始", desc: "强制开始/取消强制指定任务（绕过队列）", icon: "force",
		dlCh: dlCh, dlOp: dlOp,
		schema: []workflow.ParamField{
			{Name: "downloader_id", Label: "下载器", Type: "downloader_select"},
			{Name: "hashes", Label: "任务哈希", Type: "textarea", Help: "逗号/换行分隔，支持 {{var.x}}；留空用上一步结果"},
			{Name: "enable", Label: "开启强制", Type: "switch", Default: true},
		},
		defaults: map[string]any{"downloader_id": float64(0), "hashes": "", "enable": true},
		run: func(ctx context.Context, dl *chain.DownloadChain, id uint, h []string, p map[string]any, ecx *workflow.Context) error {
			if len(h) == 0 {
				return fmt.Errorf("未提供任务哈希")
			}
			enable := workflow.ParamBool(p, "enable")
			if err := dl.SetForceStart(ctx, id, h, enable); err != nil {
				return fmt.Errorf("强制开始失败: %w", err)
			}
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("已对 %d 个任务%s强制开始", len(h), ifElse(enable, "开启", "取消")))
			return nil
		},
	}
}

func NewSetCategoryNode(dlCh *chain.DownloadChain, dlOp *oper.DownloaderOper) *dlMgmtNode {
	return &dlMgmtNode{
		typ: "set_category", name: "设置分类", desc: "批量设置任务的分类", icon: "category",
		dlCh: dlCh, dlOp: dlOp,
		schema: []workflow.ParamField{
			{Name: "downloader_id", Label: "下载器", Type: "downloader_select"},
			{Name: "hashes", Label: "任务哈希", Type: "textarea", Help: "逗号/换行分隔，支持 {{var.x}}；留空用上一步结果"},
			{Name: "category", Label: "分类", Type: "text", Required: true},
		},
		defaults: map[string]any{"downloader_id": float64(0), "hashes": "", "category": ""},
		run: func(ctx context.Context, dl *chain.DownloadChain, id uint, h []string, p map[string]any, ecx *workflow.Context) error {
			if len(h) == 0 {
				return fmt.Errorf("未提供任务哈希")
			}
			cat := workflow.ParamString(p, "category")
			if cat == "" {
				return fmt.Errorf("分类为空")
			}
			if err := dl.SetCategory(ctx, id, h, cat); err != nil {
				return fmt.Errorf("设置分类失败: %w", err)
			}
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("已对 %d 个任务设置分类 %s", len(h), cat))
			return nil
		},
	}
}

func NewAddTagsNode(dlCh *chain.DownloadChain, dlOp *oper.DownloaderOper) *dlMgmtNode {
	return &dlMgmtNode{
		typ: "add_tags", name: "添加标签", desc: "批量为任务追加标签", icon: "tag",
		dlCh: dlCh, dlOp: dlOp,
		schema: []workflow.ParamField{
			{Name: "downloader_id", Label: "下载器", Type: "downloader_select"},
			{Name: "hashes", Label: "任务哈希", Type: "textarea", Help: "逗号/换行分隔，支持 {{var.x}}；留空用上一步结果"},
			{Name: "tags", Label: "标签", Type: "text", Required: true, Help: "逗号分隔多个"},
		},
		defaults: map[string]any{"downloader_id": float64(0), "hashes": "", "tags": ""},
		run: func(ctx context.Context, dl *chain.DownloadChain, id uint, h []string, p map[string]any, ecx *workflow.Context) error {
			if len(h) == 0 {
				return fmt.Errorf("未提供任务哈希")
			}
			tags := workflow.ParamStringSlice(p, "tags")
			if len(tags) == 0 {
				return fmt.Errorf("标签为空")
			}
			if err := dl.AddTags(ctx, id, h, tags); err != nil {
				return fmt.Errorf("添加标签失败: %w", err)
			}
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("已对 %d 个任务添加标签 %s", len(h), strings.Join(tags, ",")))
			return nil
		},
	}
}

func NewSetSavePathNode(dlCh *chain.DownloadChain, dlOp *oper.DownloaderOper) *dlMgmtNode {
	return &dlMgmtNode{
		typ: "set_save_path", name: "更改保存路径", desc: "批量更改任务的保存路径", icon: "folder",
		dlCh: dlCh, dlOp: dlOp,
		schema: []workflow.ParamField{
			{Name: "downloader_id", Label: "下载器", Type: "downloader_select"},
			{Name: "hashes", Label: "任务哈希", Type: "textarea", Help: "逗号/换行分隔，支持 {{var.x}}；留空用上一步结果"},
			{Name: "location", Label: "新路径", Type: "text", Required: true},
		},
		defaults: map[string]any{"downloader_id": float64(0), "hashes": "", "location": ""},
		run: func(ctx context.Context, dl *chain.DownloadChain, id uint, h []string, p map[string]any, ecx *workflow.Context) error {
			if len(h) == 0 {
				return fmt.Errorf("未提供任务哈希")
			}
			loc := workflow.ParamString(p, "location")
			if loc == "" {
				return fmt.Errorf("路径为空")
			}
			if err := dl.SetLocation(ctx, id, h, loc); err != nil {
				return fmt.Errorf("更改路径失败: %w", err)
			}
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("已对 %d 个任务更改保存路径", len(h)))
			return nil
		},
	}
}
