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

// 本文件封装 chain.DownloadChain 的进阶操作（download.go + download_ops.go 的方法）为工作流节点。
// 14 个节点共用 dlMgmtNode 骨架：解析 downloader_id（0=默认下载器）与 hashes，调 run 执行。
// 参数丰富的节点（分类/标签/限速/分享率/Tracker）在各自 run 闭包内取参。

// dlMgmtNode 是下载器管理节点的通用骨架。按 typ 字段区分（Registry 按 Type() 去重）。
type dlMgmtNode struct {
	typ      string
	name     string
	desc     string
	icon     string
	dlCh     *chain.DownloadChain
	dlOp     *oper.DownloaderOper
	schema   []workflow.ParamField
	defaults map[string]any
	run      func(ctx context.Context, dl *chain.DownloadChain, id uint, hashes []string, p map[string]any, ecx *workflow.Context) error
}

func (n *dlMgmtNode) Type() string                       { return n.typ }
func (n *dlMgmtNode) Name() string                       { return n.name }
func (n *dlMgmtNode) Category() string                   { return "下载" }
func (n *dlMgmtNode) Description() string                { return n.desc }
func (n *dlMgmtNode) Icon() string                       { return n.icon }
func (n *dlMgmtNode) DefaultParams() map[string]any      { return n.defaults }
func (n *dlMgmtNode) ParamSchema() []workflow.ParamField { return n.schema }

func (n *dlMgmtNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	// 前置参数解析（哈希 / 分类 / 标签 / 路径 等空校验）放在 nil 依赖检查之前：
	// 既覆盖测试（nil 依赖 + 合法参数），也保证用户填错的图先于「服务未就绪」报出可读错误。
	hashes := resolveHashes(params, ecx)
	if early := n.runEarlyValidate(params, ecx, hashes); early != nil {
		return early
	}
	// dlOp/dlCh 为构造期注入，正常不会 nil；这里前置判 nil，避免测试与误用场景 panic。
	if n.dlOp == nil || n.dlCh == nil {
		return fmt.Errorf("下载器服务未初始化")
	}
	id, err := resolveDLID(params, n.dlOp)
	if err != nil {
		return err
	}
	return n.run(ctx, n.dlCh, id, hashes, params, ecx)
}

// runEarlyValidate 在依赖就绪前先做纯参数校验，nil dlCh/dlOp 下安全。
func (n *dlMgmtNode) runEarlyValidate(params map[string]any, ecx *workflow.Context, hashes []string) error {
	// 需要 hashes 的节点类型（list_torrents 列全部、set_speed_limit 全局限速 不需要）。
	switch n.typ {
	case "add_tracker":
		if strings.TrimSpace(render(workflow.ParamString(params, "hash"), ecx)) == "" {
			return fmt.Errorf("未提供任务哈希")
		}
		if len(nonEmptyLines(workflow.ParamString(params, "trackers"))) == 0 {
			return fmt.Errorf("Tracker 为空")
		}
	case "set_category":
		if len(hashes) == 0 {
			return fmt.Errorf("未提供任务哈希")
		}
		if workflow.ParamString(params, "category") == "" {
			return fmt.Errorf("分类为空")
		}
	case "add_tags":
		if len(hashes) == 0 {
			return fmt.Errorf("未提供任务哈希")
		}
		if len(workflow.ParamStringSlice(params, "tags")) == 0 {
			return fmt.Errorf("标签为空")
		}
	case "set_save_path":
		if len(hashes) == 0 {
			return fmt.Errorf("未提供任务哈希")
		}
		if workflow.ParamString(params, "location") == "" {
			return fmt.Errorf("路径为空")
		}
	case "pause_torrents", "resume_torrents", "recheck_torrents", "reannounce_torrents",
		"delete_torrents", "force_start", "set_torrent_limit", "set_share_limit":
		if len(hashes) == 0 {
			return fmt.Errorf("未提供任务哈希")
		}
	}
	return nil
}

// resolveDLID 解析 downloader_id 参数：0 → 默认下载器（复用 AddDownloadNode 同款回退）。
func resolveDLID(params map[string]any, dlOp *oper.DownloaderOper) (uint, error) {
	id := uint(workflow.ParamInt(params, "downloader_id"))
	if id == 0 {
		id = defaultDownloaderID(dlOp)
		if id == 0 {
			return 0, fmt.Errorf("未配置下载器")
		}
	}
	return id, nil
}

// defaultDownloaderID 取默认下载器 ID（IsDefault 优先，否则首个）。无则 0。
// 与 download.go AddDownloadNode.defaultDownloaderID 同逻辑；独立一份避免改动已测代码。
func defaultDownloaderID(dlOp *oper.DownloaderOper) uint {
	list, err := dlOp.List()
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

// resolveHashes 解析任务哈希：优先 hashes 参数（支持 {{var.x}} 模板，逗号/空白/换行分隔）；
// 为空则回退 ecx.Variables["torrent_hashes"]（list_torrents 写入），从而「列出→操作」零配置串联。
func resolveHashes(params map[string]any, ecx *workflow.Context) []string {
	raw := strings.TrimSpace(render(workflow.ParamString(params, "hashes"), ecx))
	if raw == "" {
		if v, ok := ecx.Variables["torrent_hashes"]; ok {
			raw = strings.TrimSpace(fmt.Sprintf("%v", v))
		}
	}
	if raw == "" {
		return nil
	}
	var out []string
	for _, p := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	}) {
		if h := strings.TrimSpace(p); h != "" {
			out = append(out, h)
		}
	}
	return out
}

// hashSchema 是需要任务哈希的节点的通用表单（下载器 + 哈希）。
func hashSchema() ([]workflow.ParamField, map[string]any) {
	return []workflow.ParamField{
			{Name: "downloader_id", Label: "下载器", Type: "downloader_select"},
			{Name: "hashes", Label: "任务哈希", Type: "textarea",
				Help: "逗号/换行分隔，支持 {{var.x}}；留空则用上一步「列出种子」的结果"},
		},
		map[string]any{"downloader_id": float64(0), "hashes": ""}
}

// —— 生命周期 ——

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

// —— 分类 / 标签 / 路径 ——

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

// —— 限速 ——（单位 KB/s → bytes/s，0=不限）

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

// parseShareRatio 解析分享率：空/-1→-1(全局默认)；-2→-2(不限)；正数→值。
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

// —— Tracker ——

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

// —— 辅助 ——

// hashAction 构造「校验哈希非空 → 调链 → 记消息」的通用 run 闭包。
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

// nonEmptyLines 取多行文本中的非空行（trim 空白）。
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
