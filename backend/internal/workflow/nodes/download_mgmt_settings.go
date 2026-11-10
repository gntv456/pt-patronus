// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package nodes

import (
	"context"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

func speedSchema(withHashes bool) ([]workflow.ParamField, map[string]any) {
	fields := []workflow.ParamField{
		{Name: "downloader_id", Label: "下载器", Type: "downloader_select"},
	}
	defs := map[string]any{"downloader_id": float64(0)}
	if withHashes {
		fields = append(fields, workflow.ParamField{Name: "hashes", Label: "任务哈希", Type: "textarea",
			Help: "逗号/换行分隔，支持 {{var.x}}；留空用上一步结果"})
		defs["hashes"] = ""
	}
	fields = append(fields,
		workflow.ParamField{Name: "up_limit", Label: "上传限速(KB/s)", Type: "number", Help: "0=不限"},
		workflow.ParamField{Name: "dl_limit", Label: "下载限速(KB/s)", Type: "number", Help: "0=不限"},
	)
	defs["up_limit"] = float64(0)
	defs["dl_limit"] = float64(0)
	return fields, defs
}

func NewSetSpeedLimitNode(dlCh *chain.DownloadChain, dlOp *oper.DownloaderOper) *dlMgmtNode {
	s, d := speedSchema(false)
	return &dlMgmtNode{
		typ: "set_speed_limit", name: "全局限速", desc: "设置下载器全局上传/下载限速", icon: "speed",
		dlCh: dlCh, dlOp: dlOp, schema: s, defaults: d,
		run: func(ctx context.Context, dl *chain.DownloadChain, id uint, _ []string, p map[string]any, ecx *workflow.Context) error {
			if dl == nil {
				return fmt.Errorf("下载器服务未初始化")
			}
			limits := downloader.Limits{
				UploadLimit:   int64(workflow.ParamInt(p, "up_limit")) * 1024,
				DownloadLimit: int64(workflow.ParamInt(p, "dl_limit")) * 1024,
			}
			if err := dl.SetSpeedLimits(ctx, id, limits); err != nil {
				return fmt.Errorf("全局限速失败: %w", err)
			}
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("全局限速：↑%d ↓%d KB/s", workflow.ParamInt(p, "up_limit"), workflow.ParamInt(p, "dl_limit")))
			return nil
		},
	}
}

func NewSetTorrentLimitNode(dlCh *chain.DownloadChain, dlOp *oper.DownloaderOper) *dlMgmtNode {
	s, d := speedSchema(true)
	return &dlMgmtNode{
		typ: "set_torrent_limit", name: "单种限速", desc: "为指定任务设置上传/下载限速", icon: "speed",
		dlCh: dlCh, dlOp: dlOp, schema: s, defaults: d,
		run: func(ctx context.Context, dl *chain.DownloadChain, id uint, h []string, p map[string]any, ecx *workflow.Context) error {
			if len(h) == 0 {
				return fmt.Errorf("未提供任务哈希")
			}
			limits := downloader.Limits{
				UploadLimit:   int64(workflow.ParamInt(p, "up_limit")) * 1024,
				DownloadLimit: int64(workflow.ParamInt(p, "dl_limit")) * 1024,
			}
			if err := dl.SetLimits(ctx, id, h, limits); err != nil {
				return fmt.Errorf("单种限速失败: %w", err)
			}
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("已对 %d 个任务限速", len(h)))
			return nil
		},
	}
}

func NewSetShareLimitNode(dlCh *chain.DownloadChain, dlOp *oper.DownloaderOper) *dlMgmtNode {
	return &dlMgmtNode{
		typ: "set_share_limit", name: "分享率/时长限制", desc: "设置任务的做种分享率与做种时长上限", icon: "share",
		dlCh: dlCh, dlOp: dlOp,
		schema: []workflow.ParamField{
			{Name: "downloader_id", Label: "下载器", Type: "downloader_select"},
			{Name: "hashes", Label: "任务哈希", Type: "textarea", Help: "逗号/换行分隔，支持 {{var.x}}；留空用上一步结果"},
			{Name: "ratio_limit", Label: "分享率上限", Type: "text",
				Help: "数字=具体值；-2=不限；-1=跟随全局默认；留空=-1"},
			{Name: "seeding_time_limit", Label: "做种时长(分钟)", Type: "number",
				Help: "-2=不限；-1=跟随全局默认；0 与正数=分钟数"},
		},
		defaults: map[string]any{"downloader_id": float64(0), "hashes": "", "ratio_limit": "", "seeding_time_limit": float64(-1)},
		run: func(ctx context.Context, dl *chain.DownloadChain, id uint, h []string, p map[string]any, ecx *workflow.Context) error {
			if len(h) == 0 {
				return fmt.Errorf("未提供任务哈希")
			}
			limits := downloader.ShareLimits{
				RatioLimit:       parseShareRatio(workflow.ParamString(p, "ratio_limit")),
				SeedingTimeLimit: workflow.ParamInt(p, "seeding_time_limit"),
			}
			if err := dl.SetShareLimits(ctx, id, h, limits); err != nil {
				return fmt.Errorf("设置分享率失败: %w", err)
			}
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("已对 %d 个任务设置分享率 %.1f / %d 分钟", len(h), limits.RatioLimit, limits.SeedingTimeLimit))
			return nil
		},
	}
}

func parseShareRatio(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return -1
	}
	switch s {
	case "-2":
		return -2
	case "-1":
		return -1
	}
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
		return f
	}
	return -1
}

func NewAddTrackerNode(dlCh *chain.DownloadChain, dlOp *oper.DownloaderOper) *dlMgmtNode {
	return &dlMgmtNode{
		typ: "add_tracker", name: "添加 Tracker", desc: "为指定任务追加 Tracker", icon: "tracker",
		dlCh: dlCh, dlOp: dlOp,
		schema: []workflow.ParamField{
			{Name: "downloader_id", Label: "下载器", Type: "downloader_select"},
			{Name: "hash", Label: "任务哈希", Type: "text", Required: true, Help: "单个任务哈希，支持 {{var.x}}"},
			{Name: "trackers", Label: "Tracker", Type: "textarea", Required: true, Help: "每行一个 URL"},
		},
		defaults: map[string]any{"downloader_id": float64(0), "hash": "", "trackers": ""},
		run: func(ctx context.Context, dl *chain.DownloadChain, id uint, _ []string, p map[string]any, ecx *workflow.Context) error {
			hash := strings.TrimSpace(render(workflow.ParamString(p, "hash"), ecx))
			if hash == "" {
				return fmt.Errorf("未提供任务哈希")
			}
			raw := workflow.ParamString(p, "trackers")
			urls := nonEmptyLines(raw)
			if len(urls) == 0 {
				return fmt.Errorf("Tracker 为空")
			}
			if err := dl.AddTrackers(ctx, id, hash, urls); err != nil {
				return fmt.Errorf("添加 Tracker 失败: %w", err)
			}
			ecx.Messages = append(ecx.Messages, fmt.Sprintf("已添加 %d 个 Tracker", len(urls)))
			return nil
		},
	}
}

func hashAction(action string, fn func(ctx context.Context, dl *chain.DownloadChain, id uint, h []string) error) func(context.Context, *chain.DownloadChain, uint, []string, map[string]any, *workflow.Context) error {
	return func(ctx context.Context, dl *chain.DownloadChain, id uint, h []string, _ map[string]any, ecx *workflow.Context) error {
		if len(h) == 0 {
			return fmt.Errorf("未提供任务哈希")
		}
		if err := fn(ctx, dl, id, h); err != nil {
			return fmt.Errorf("%s失败: %w", action, err)
		}
		ecx.Messages = append(ecx.Messages, fmt.Sprintf("已%s %d 个任务", action, len(h)))
		return nil
	}
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if l := strings.TrimSpace(line); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func ifElse(b bool, t, f string) string {
	if b {
		return t
	}
	return f
}
