package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/mcp"
)

// 本文件为 AI Agent 补齐 plugin HostTool 已有、但 agent 未挂载的 chain 能力
// （媒体服务器/云盘/刷流/RSS/字幕/文件/工作流 + 下载器/订阅/整理/辅种增量）。
//
// 设计权衡（见 plan curried-pondering-matsumoto）：handler 逻辑与 plugin_host_tools.go
// 对应 host-only tool 一致，仅把「返回 any」适配成 mcp.Tool 要求的「返回 JSON 字符串」。
// 接受有限重复换 plugin 侧零改动、零回归（plugin 83 个 host tool 现全绿）。agent 与 plugin
// 各自是独立消费者；mcp.Tool 不带 Permission/Category/Destructive（那是 plugin 闸门用的，
// agent 整条路由已在 middleware.RequireAdmin 保护下）。
//
// 辅助函数 noArgsSchema/objSchema/argFloat/intProp/strProp/okCount 复用 plugin_host_tools.go；
// argString/argInt/argIntDefault/argBool/argStrSlice/hashesSchema 复用 agent_tools.go（同包）。

// jsonResult 把 chain 方法常见的 (any, error) 返回适配成 mcp.Tool handler 要的 (string, error)：
// err 非空时透传（agent 层会回灌给模型自适应），否则压缩序列化为 JSON 字符串。
func jsonResult(v any, err error) (string, error) {
	if err != nil {
		return "", err
	}
	return mcpJSON(v), nil
}

// slowTimeout 标记慢操作（库扫描/刷新/订阅检查/整理重跑/STRM 生成/RSS 检查/工作流执行）的单工具超时。
// agent 默认 toolExecTimeout=30s 不够这些远端调用；plugin 侧同样设 60s。
const slowTimeout = 60 * time.Second

// ===== 下载器高级操作（13） =====

func buildDownloaderAdvTools(dl *chain.DownloadChain) []mcp.Tool {
	return []mcp.Tool{
		{Name: "torrents_files", Description: "List files inside a torrent (path/size/priority).", InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hash": strProp("")}, []string{"downloader_id", "hash"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(dl.ListFiles(ctx, uint(argInt(a, "downloader_id")), argString(a, "hash", "")))
		}},
		{Name: "torrents_peers", Description: "List connected peers of a torrent.", InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hash": strProp("")}, []string{"downloader_id", "hash"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(dl.ListPeers(ctx, uint(argInt(a, "downloader_id")), argString(a, "hash", "")))
		}},
		{Name: "torrents_trackers", Description: "List trackers of a torrent.", InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hash": strProp("")}, []string{"downloader_id", "hash"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(dl.ListTrackers(ctx, uint(argInt(a, "downloader_id")), argString(a, "hash", "")))
		}},
		{Name: "downloader_categories", Description: "List available categories in a downloader.", InputSchema: objSchema(map[string]any{"downloader_id": intProp("")}, []string{"downloader_id"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(dl.ListCategories(ctx, uint(argInt(a, "downloader_id"))))
		}},
		{Name: "downloader_tags", Description: "List available tags in a downloader.", InputSchema: objSchema(map[string]any{"downloader_id": intProp("")}, []string{"downloader_id"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(dl.ListTags(ctx, uint(argInt(a, "downloader_id"))))
		}},
		{Name: "torrents_set_category", Description: "Set the category of torrents by hashes. category is required — pass an explicit \"\" to clear it.", InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hashes": hashesProp(), "category": strProp("target category; \"\" = move out of any category")}, []string{"downloader_id", "hashes", "category"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			// 省略 category 过去会被补成 ""，等价于把这批种子全部移出分类——
			// 分类是刷流分流与自动删种判定的依据，必须显式才允许。
			if err := requireArgs(a, "category"); err != nil {
				return "", err
			}
			h := argStrSlice(a, "hashes")
			return jsonResult(okCount("updated", len(h), dl.SetCategory(ctx, uint(argInt(a, "downloader_id")), h, argString(a, "category", ""))))
		}},
		{Name: "torrents_set_tags", Description: "Replace the tag set of torrents (set op). tags is a string array and is required (chain rejects an empty list rather than clearing tags).", InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hashes": hashesProp(), "tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, []string{"downloader_id", "hashes", "tags"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			h := argStrSlice(a, "hashes")
			return jsonResult(okCount("updated", len(h), dl.SetTags(ctx, uint(argInt(a, "downloader_id")), h, argStrSlice(a, "tags"))))
		}},
		{Name: "torrents_recheck", Description: "Force recheck torrents by hashes.", InputSchema: hashesSchema(), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			h := argStrSlice(a, "hashes")
			return jsonResult(okCount("rechecked", len(h), dl.Recheck(ctx, uint(argInt(a, "downloader_id")), h)))
		}},
		{Name: "torrents_reannounce", Description: "Reannounce torrents to trackers by hashes.", InputSchema: hashesSchema(), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			h := argStrSlice(a, "hashes")
			return jsonResult(okCount("reannounced", len(h), dl.Reannounce(ctx, uint(argInt(a, "downloader_id")), h)))
		}},
		{Name: "torrents_set_limits", Description: "Per-torrent speed limits (bytes/s). Both limits are required: 0 removes the limit in most clients, so it is never treated as \"no change\".", InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hashes": hashesProp(), "upload_limit": intProp("bytes/s; 0 = no limit"), "download_limit": intProp("bytes/s; 0 = no limit")}, []string{"downloader_id", "hashes", "upload_limit", "download_limit"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			// 原描述写「0=no change」，但 qb.SetLimits 是两个值都始终下发、0 即取消限速：
			// 省略参数=批量摘掉这批种子的限速。要求两个都显式给出。
			if err := requireArgs(a, "upload_limit", "download_limit"); err != nil {
				return "", err
			}
			h := argStrSlice(a, "hashes")
			return jsonResult(okCount("updated", len(h), dl.SetLimits(ctx, uint(argInt(a, "downloader_id")), h, downloader.Limits{
				UploadLimit:   int64(argInt(a, "upload_limit")),
				DownloadLimit: int64(argInt(a, "download_limit")),
			})))
		}},
		{Name: "torrents_set_location", Description: "Move torrents' save location.", InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hashes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "location": strProp("new save path")}, []string{"downloader_id", "hashes", "location"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			h := argStrSlice(a, "hashes")
			return jsonResult(okCount("moved", len(h), dl.SetLocation(ctx, uint(argInt(a, "downloader_id")), h, argString(a, "location", ""))))
		}},
		{Name: "torrents_set_share_limits", Description: "Set share limits on torrents. ratio_limit: -2=unlimited,-1=global,>=0=value; seeding_time_limit in minutes (same convention). Both are required — they are always sent, and ratio_limit=0 makes every listed torrent trip its share limit at once.", InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hashes": hashesProp(), "ratio_limit": map[string]any{"type": "number"}, "seeding_time_limit": intProp("minutes")}, []string{"downloader_id", "hashes", "ratio_limit", "seeding_time_limit"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			// qb.SetShareLimits 把 ratioLimit/seedingTimeLimit 一并下发，省略即发 0：
			// 比率上限 0 = 这批种子当场触顶，接着会被自动删种逻辑清掉。必须都显式给。
			if err := requireArgs(a, "ratio_limit", "seeding_time_limit"); err != nil {
				return "", err
			}
			h := argStrSlice(a, "hashes")
			return jsonResult(okCount("updated", len(h), dl.SetShareLimits(ctx, uint(argInt(a, "downloader_id")), h, downloader.ShareLimits{
				RatioLimit:       argFloat(a, "ratio_limit"),
				SeedingTimeLimit: argInt(a, "seeding_time_limit"),
			})))
		}},
		{Name: "torrents_set_force_start", Description: "Force start / cancel force start on torrents. enable is required (omitting it is not the same as false).", InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hashes": hashesProp(), "enable": map[string]any{"type": "boolean", "description": "true = force start, false = cancel"}}, []string{"downloader_id", "hashes", "enable"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			// 省略 enable 会被 argBool 取成 false = 「取消强制开始」，与模型说「强制开始这批」相反。
			if err := requireArgs(a, "enable"); err != nil {
				return "", err
			}
			h := argStrSlice(a, "hashes")
			return jsonResult(okCount("updated", len(h), dl.SetForceStart(ctx, uint(argInt(a, "downloader_id")), h, argBool(a, "enable"))))
		}},
	}
}

// ===== 媒体服务器（7） =====

func buildMediaServerTools(ms *chain.MediaServerChain) []mcp.Tool {
	return []mcp.Tool{
		{Name: "mediaserver_list", Description: "List configured media servers (Emby/Plex/Jellyfin): id, name, type, address.", InputSchema: noArgsSchema(), Handler: func(_ context.Context, _ map[string]any) (string, error) {
			return jsonResult(ms.List())
		}},
		{Name: "mediaserver_libraries", Description: "List libraries of a media server.", InputSchema: objSchema(map[string]any{"server_id": intProp("")}, []string{"server_id"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(ms.Libraries(ctx, uint(argInt(a, "server_id"))))
		}},
		{Name: "mediaserver_library_scan", Description: "Trigger a library scan on a media server (unblocks NFO/poster → library refresh). Empty library_id scans every library and reports how many.", InputSchema: objSchema(map[string]any{"server_id": intProp(""), "library_id": strProp("optional; empty=scan all")}, []string{"server_id"}), Timeout: slowTimeout, Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(scanMediaLibraries(ctx, ms, uint(argInt(a, "server_id")), argString(a, "library_id", "")))
		}},
		{Name: "mediaserver_refresh", Description: "Refresh metadata of a library on a media server.", InputSchema: objSchema(map[string]any{"server_id": intProp(""), "library_id": strProp("")}, []string{"server_id", "library_id"}), Timeout: slowTimeout, Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(okCount("refreshed", 1, ms.Refresh(ctx, uint(argInt(a, "server_id")), argString(a, "library_id", ""))))
		}},
		{Name: "mediaserver_notify", Description: "Send an in-app notification to a single media server (Emby/Plex admin toast).", InputSchema: objSchema(map[string]any{"server_id": intProp(""), "title": strProp(""), "body": strProp("")}, []string{"server_id", "title", "body"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(okCount("notified", 1, ms.NotifyOne(ctx, uint(argInt(a, "server_id")), argString(a, "title", ""), argString(a, "body", ""))))
		}},
		{Name: "mediaserver_playback_report", Description: "Playback report of a media server over N days.", InputSchema: objSchema(map[string]any{"server_id": intProp(""), "days": intProp("default 7")}, []string{"server_id"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(ms.PlaybackReport(ctx, uint(argInt(a, "server_id")), argIntDefault(a, "days", 7)))
		}},
		{Name: "mediaserver_duplicates", Description: "Detect duplicate media items in a library.", InputSchema: objSchema(map[string]any{"server_id": intProp(""), "library_id": strProp("")}, []string{"server_id", "library_id"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(ms.DetectDuplicates(ctx, uint(argInt(a, "server_id")), argString(a, "library_id", "")))
		}},
	}
}

// ===== 订阅补充（2） =====

func buildSubscribeAdvTools(sub *chain.SubscribeChain) []mcp.Tool {
	return []mcp.Tool{
		{Name: "subscribe_get", Description: "Get a single subscription by id.", InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}), Handler: func(_ context.Context, a map[string]any) (string, error) {
			return jsonResult(sub.Get(uint(argInt(a, "id"))))
		}},
		{Name: "subscribe_check", Description: "Force-check a subscription (search + push to downloader). Returns number of new hits.", InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}), Timeout: slowTimeout, Handler: func(ctx context.Context, a map[string]any) (string, error) {
			s, err := sub.Get(uint(argInt(a, "id")))
			if err != nil {
				return "", err
			}
			n, err := sub.Check(ctx, s)
			return jsonResult(map[string]any{"checked": n}, err)
		}},
	}
}

// ===== 整理补充（2） =====

func buildTransferAdvTools(tr *chain.TransferChain) []mcp.Tool {
	return []mcp.Tool{
		{Name: "transfer_redo", Description: "Re-organize a past transfer by history id.", InputSchema: objSchema(map[string]any{"history_id": intProp("")}, []string{"history_id"}), Timeout: slowTimeout, Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(tr.Redo(ctx, uint(argInt(a, "history_id"))))
		}},
		{Name: "transfer_sweep", Description: "Trigger a full organize sweep over completed torrents (async on the host). Returns an error when the organize feature is disabled, because then the sweep would do nothing.", InputSchema: noArgsSchema(), Handler: func(_ context.Context, _ map[string]any) (string, error) {
			if !tr.SweepEnabled() {
				return "", errors.New("整理功能未启用，扫描不会处理任何种子（先在整理页开启总开关）")
			}
			go tr.Sweep(context.Background())
			return mcpJSON(map[string]any{"started": true, "note": "扫描在后台进行，完成后看整理历史"}), nil
		}},
	}
}

// ===== 辅种补充（3） =====

func buildReseedAdvTools(ks *chain.KeepSeedChain) []mcp.Tool {
	return []mcp.Tool{
		{Name: "reseed_find_candidates", Description: "Fuzzy-match cross-site seed candidates by name + size (read-only audit).", InputSchema: objSchema(map[string]any{"name": strProp(""), "size": map[string]any{"type": "integer", "description": "bytes"}}, []string{"name"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return mcpJSON(ks.FindCandidates(ctx, argString(a, "name", ""), int64(argInt(a, "size")))), nil
		}},
		{Name: "reseed_find_missing", Description: "Audit which target sites are missing cross-seeds for a downloader's torrents. target_site_ids must list at least one site.", InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "target_site_ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "minItems": 1, "description": "target site ids to audit; at least one"}}, []string{"downloader_id", "target_site_ids"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(ks.FindMissing(ctx, uint(argInt(a, "downloader_id")), argIDSlice(a, "target_site_ids")))
		}},
		{Name: "reseed_resolve", Description: "Resolve a cross-seed candidate task: action = add | skip.", InputSchema: objSchema(map[string]any{"task_id": intProp(""), "action": strProp("add | skip")}, []string{"task_id", "action"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(okCount("resolved", 1, ks.Resolve(ctx, uint(argInt(a, "task_id")), argString(a, "action", ""))))
		}},
	}
}

// ===== 云盘 / STRM（6） =====

func buildCloudTools(cl *chain.CloudChain) []mcp.Tool {
	return []mcp.Tool{
		{Name: "cloud_list", Description: "List configured cloud drives (115/夸克/123/Alist).", InputSchema: noArgsSchema(), Handler: func(_ context.Context, _ map[string]any) (string, error) {
			return jsonResult(cl.List())
		}},
		{Name: "cloud_browse", Description: "Browse a cloud drive directory.", InputSchema: objSchema(map[string]any{"drive_id": intProp(""), "path": strProp("")}, []string{"drive_id"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(cl.Browse(ctx, uint(argInt(a, "drive_id")), argString(a, "path", "/")))
		}},
		{Name: "cloud_quota", Description: "Query storage quota of a cloud drive.", InputSchema: objSchema(map[string]any{"drive_id": intProp("")}, []string{"drive_id"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(cl.Quota(ctx, uint(argInt(a, "drive_id"))))
		}},
		{Name: "cloud_rules", Description: "List STRM generation rules.", InputSchema: noArgsSchema(), Handler: func(_ context.Context, _ map[string]any) (string, error) {
			return jsonResult(cl.ListRules())
		}},
		{Name: "cloud_generate_strm", Description: "Generate .strm files for a STRM rule.", InputSchema: objSchema(map[string]any{"rule_id": intProp("")}, []string{"rule_id"}), Timeout: slowTimeout, Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(cl.GenerateStrm(ctx, uint(argInt(a, "rule_id"))))
		}},
		{Name: "cloud_generate_all", Description: "Generate .strm files for all enabled rules; returns the cross-rule summary (rules/generated/skipped/file_errors/failed).", InputSchema: noArgsSchema(), Timeout: slowTimeout, Handler: func(ctx context.Context, _ map[string]any) (string, error) {
			// 与 plugin 面同步：原先 go + {ok:true} 让「一条都没生成」也回执成功。
			return mcpJSON(cl.GenerateAll(ctx)), nil
		}},
	}
}

// ===== 刷流（5） =====

func buildBrushFlowTools(bf *chain.BrushFlowChain) []mcp.Tool {
	return []mcp.Tool{
		{Name: "brushflow_tasks_list", Description: "List brush-flow tasks.", InputSchema: noArgsSchema(), Handler: func(_ context.Context, _ map[string]any) (string, error) {
			return jsonResult(bf.ListTasks())
		}},
		{Name: "brushflow_task_get", Description: "Get a brush-flow task by id.", InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}), Handler: func(_ context.Context, a map[string]any) (string, error) {
			return jsonResult(bf.GetTask(uint(argInt(a, "id"))))
		}},
		{Name: "brushflow_seeds", Description: "List seed history of a brush-flow task.", InputSchema: objSchema(map[string]any{"task_id": intProp("")}, []string{"task_id"}), Handler: func(_ context.Context, a map[string]any) (string, error) {
			return jsonResult(bf.ListSeeds(uint(argInt(a, "task_id"))))
		}},
		{Name: "brushflow_run_select", Description: "Manually trigger seed selection of a brush-flow task. Fails if the task is disabled (a disabled task does nothing).", InputSchema: objSchema(map[string]any{"task_id": intProp("")}, []string{"task_id"}), Timeout: slowTimeout, Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(brushflowRunOutcome(ctx, bf, uint(argInt(a, "task_id")), "select", "selected", bf.RunSelect))
		}},
		{Name: "brushflow_run_sweep", Description: "Manually trigger sweep (delete) of a brush-flow task. Fails if the task is disabled (a disabled task deletes nothing).", Risk: mcp.RiskDestructive, RiskNote: "触发刷流删除已达标种子（做种数据丢失）", InputSchema: objSchema(map[string]any{"task_id": intProp("")}, []string{"task_id"}), Timeout: slowTimeout, Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(brushflowRunOutcome(ctx, bf, uint(argInt(a, "task_id")), "sweep", "swept", bf.RunSweep))
		}},
	}
}

// brushflowRunOutcome 执行一次刷流运行并给出诚实回执。
//
// 链层两处会把「什么都没做」报成成功：RunSelect/RunSweep 遇到未启用任务直接 return nil；
// 逐项失败（推送下载器失败 N 条）只落进任务诊断字段、函数仍 return nil。
// 工具原本写死 okCount(key, 1, err)，于是模型/插件看到 {"selected":1} 就以为这批种子
// 已进下载器，删除类工具更会把一次空跑说成「已删除」。这里补上：未启用直接失败，
// 跑完回读任务诊断一并带回。agent 与 plugin 两个工具面共用本函数。
func brushflowRunOutcome(ctx context.Context, bf *chain.BrushFlowChain, taskID uint, errKey, countKey string, run func(context.Context, uint) error) (map[string]any, error) {
	if taskID == 0 {
		return nil, fmt.Errorf("缺少 task_id")
	}
	before, err := bf.GetTask(taskID)
	if err != nil {
		return nil, fmt.Errorf("刷流任务 #%d 读取失败: %w", taskID, err)
	}
	if !before.Enabled {
		return nil, fmt.Errorf("刷流任务 #%d 未启用：本次不会选取也不会删除任何种子", taskID)
	}
	if err := run(ctx, taskID); err != nil {
		return nil, err
	}
	out := map[string]any{countKey: 1, "task_id": taskID}
	if after, e := bf.GetTask(taskID); e == nil && after.LastError != "" && after.LastErrorFrom == errKey {
		out["task_error"] = after.LastError
		// 同一条诊断可能来自上一次运行；变化过才标 new，避免把旧账算在本次头上。
		if before.LastError != after.LastError || before.LastErrorFrom != errKey {
			out["task_error_new"] = true
		}
	}
	return out, nil
}

// ===== RSS（3） =====

func buildRssTools(rss *chain.RssChain) []mcp.Tool {
	return []mcp.Tool{
		{Name: "rss_list", Description: "List RSS subscriptions.", InputSchema: noArgsSchema(), Handler: func(_ context.Context, _ map[string]any) (string, error) {
			return jsonResult(rss.List())
		}},
		{Name: "rss_items", Description: "List parsed items of an RSS subscription.", InputSchema: objSchema(map[string]any{"id": intProp(""), "limit": intProp("default 50")}, []string{"id"}), Handler: func(_ context.Context, a map[string]any) (string, error) {
			return jsonResult(rss.ListItems(uint(argInt(a, "id")), argIntDefault(a, "limit", 50)))
		}},
		{Name: "rss_check", Description: "Force-check an RSS subscription. Returns number of new items.", InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}), Timeout: slowTimeout, Handler: func(ctx context.Context, a map[string]any) (string, error) {
			r, err := rss.Get(uint(argInt(a, "id")))
			if err != nil {
				return "", err
			}
			n, err := rss.Check(ctx, r)
			return jsonResult(map[string]any{"checked": n}, err)
		}},
	}
}

// ===== 字幕（1） =====
// 仅 search：Download 返回字节流不适合 LLM 上下文（plugin 侧同样未暴露 download）。

func buildSubtitleTools(st *chain.SubtitleChain) []mcp.Tool {
	return []mcp.Tool{
		{Name: "subtitle_search", Description: "Cross-source subtitle search (ASSRT/OpenSubtitles/SubHD/Zimuku). Returns scored candidates; download via the Subtitle page.", InputSchema: objSchema(map[string]any{"query": strProp("movie/episode name"), "language": strProp("zh/en/...")}, []string{"query"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(st.Search(ctx, argString(a, "query", ""), argString(a, "language", "")))
		}},
	}
}

// ===== 文件存储（3） =====

func buildFileTools(fc *chain.FileManagerChain) []mcp.Tool {
	return []mcp.Tool{
		{Name: "file_list", Description: "List entries of a storage backend (WebDAV/Alist/SMB/local) directory.", InputSchema: objSchema(map[string]any{"storage_id": intProp(""), "path": strProp("")}, []string{"storage_id"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(fc.List(ctx, uint(argInt(a, "storage_id")), argString(a, "path", "/")))
		}},
		{Name: "file_mkdir", Description: "Create a directory on a storage backend.", InputSchema: objSchema(map[string]any{"storage_id": intProp(""), "path": strProp("")}, []string{"storage_id", "path"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(okCount("created", 1, fc.Mkdir(ctx, uint(argInt(a, "storage_id")), argString(a, "path", ""))))
		}},
		{Name: "file_delete", Description: "Delete a file or directory on a storage backend (irreversible).", Risk: mcp.RiskDestructive, RiskNote: "删除存储后端文件/目录（不可恢复）", InputSchema: objSchema(map[string]any{"storage_id": intProp(""), "path": strProp("")}, []string{"storage_id", "path"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(okCount("deleted", 1, fc.Delete(ctx, uint(argInt(a, "storage_id")), argString(a, "path", ""))))
		}},
	}
}

// ===== 工作流（3） =====

func buildWorkflowTools(wf *chain.WorkflowChain) []mcp.Tool {
	return []mcp.Tool{
		{Name: "workflow_list", Description: "List workflows.", InputSchema: noArgsSchema(), Handler: func(_ context.Context, _ map[string]any) (string, error) {
			return jsonResult(wf.List())
		}},
		{Name: "workflow_catalog", Description: "List available workflow node types (catalog).", InputSchema: noArgsSchema(), Handler: func(_ context.Context, _ map[string]any) (string, error) {
			return mcpJSON(wf.Catalog()), nil
		}},
		{Name: "workflow_run", Description: "Run a workflow by id with a trigger and optional event data.", Risk: mcp.RiskDestructive, RiskNote: "运行任意工作流（可含删除/覆盖/发送等任意节点，影响不可预判）", InputSchema: objSchema(map[string]any{"id": intProp(""), "trigger": strProp("provenance recorded in run history; default api; e.g. manual | api | webhook | timer"), "event_data": map[string]any{"type": "object"}}, []string{"id"}), Timeout: slowTimeout, Handler: func(ctx context.Context, a map[string]any) (string, error) {
			data, _ := a["event_data"].(map[string]any)
			return jsonResult(wf.Run(ctx, uint(argInt(a, "id")), runTrigger(a, "api"), data))
		}},
	}
}
