package app

import (
	"context"
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
		{Name: "torrents_set_category", Description: "Set the category of torrents by hashes.", InputSchema: hashesSchema(), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			h := argStrSlice(a, "hashes")
			return jsonResult(okCount("updated", len(h), dl.SetCategory(ctx, uint(argInt(a, "downloader_id")), h, argString(a, "category", ""))))
		}},
		{Name: "torrents_set_tags", Description: "Replace the tag set of torrents (set op). tags is a string array.", InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hashes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, []string{"downloader_id", "hashes"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
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
		{Name: "torrents_set_limits", Description: "Per-torrent speed limits (bytes/s). upload_limit/download_limit: -1=unlimited, 0=no change.", InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hashes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "upload_limit": intProp("bytes/s, -1=unlimited"), "download_limit": intProp("bytes/s, -1=unlimited")}, []string{"downloader_id", "hashes"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
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
		{Name: "torrents_set_share_limits", Description: "Set share limits on torrents. ratio_limit: -2=unlimited,-1=global,>=0=value; seeding_time_limit in minutes (same convention).", InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hashes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "ratio_limit": map[string]any{"type": "number"}, "seeding_time_limit": intProp("minutes")}, []string{"downloader_id", "hashes"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			h := argStrSlice(a, "hashes")
			return jsonResult(okCount("updated", len(h), dl.SetShareLimits(ctx, uint(argInt(a, "downloader_id")), h, downloader.ShareLimits{
				RatioLimit:       argFloat(a, "ratio_limit"),
				SeedingTimeLimit: argInt(a, "seeding_time_limit"),
			})))
		}},
		{Name: "torrents_set_force_start", Description: "Force start / cancel force start on torrents.", InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hashes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "enable": map[string]any{"type": "boolean"}}, []string{"downloader_id", "hashes"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
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
		{Name: "mediaserver_library_scan", Description: "Trigger a library scan on a media server (unblocks NFO/poster → library refresh).", InputSchema: objSchema(map[string]any{"server_id": intProp(""), "library_id": strProp("optional; empty=scan all")}, []string{"server_id"}), Timeout: slowTimeout, Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(okCount("scanned", 1, ms.LibraryScan(ctx, uint(argInt(a, "server_id")), argString(a, "library_id", ""))))
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
		{Name: "transfer_sweep", Description: "Trigger a full organize sweep over completed torrents (async on the host).", InputSchema: noArgsSchema(), Handler: func(_ context.Context, _ map[string]any) (string, error) {
			go tr.Sweep(context.Background())
			return mcpJSON(map[string]any{"ok": true}), nil
		}},
	}
}

// ===== 辅种补充（3） =====

func buildReseedAdvTools(ks *chain.KeepSeedChain) []mcp.Tool {
	return []mcp.Tool{
		{Name: "reseed_find_candidates", Description: "Fuzzy-match cross-site seed candidates by name + size (read-only audit).", InputSchema: objSchema(map[string]any{"name": strProp(""), "size": map[string]any{"type": "integer", "description": "bytes"}}, []string{"name"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return mcpJSON(ks.FindCandidates(ctx, argString(a, "name", ""), int64(argInt(a, "size")))), nil
		}},
		{Name: "reseed_find_missing", Description: "Audit which target sites are missing cross-seeds for a downloader's torrents.", InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "target_site_ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}}, []string{"downloader_id"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			rawSites, _ := a["target_site_ids"].([]any)
			ids := make([]uint, 0, len(rawSites))
			for _, e := range rawSites {
				if f, ok := e.(float64); ok {
					ids = append(ids, uint(f))
				}
			}
			return jsonResult(ks.FindMissing(ctx, uint(argInt(a, "downloader_id")), ids))
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
		{Name: "cloud_generate_all", Description: "Generate .strm files for all enabled rules (async on the host).", InputSchema: noArgsSchema(), Handler: func(_ context.Context, _ map[string]any) (string, error) {
			go cl.GenerateAll(context.Background())
			return mcpJSON(map[string]any{"ok": true}), nil
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
		{Name: "brushflow_run_select", Description: "Manually trigger seed selection of a brush-flow task.", InputSchema: objSchema(map[string]any{"task_id": intProp("")}, []string{"task_id"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(okCount("selected", 1, bf.RunSelect(ctx, uint(argInt(a, "task_id")))))
		}},
		{Name: "brushflow_run_sweep", Description: "Manually trigger sweep (delete) of a brush-flow task.", InputSchema: objSchema(map[string]any{"task_id": intProp("")}, []string{"task_id"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
			return jsonResult(okCount("swept", 1, bf.RunSweep(ctx, uint(argInt(a, "task_id")))))
		}},
	}
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
		{Name: "file_delete", Description: "Delete a file or directory on a storage backend (irreversible).", InputSchema: objSchema(map[string]any{"storage_id": intProp(""), "path": strProp("")}, []string{"storage_id", "path"}), Handler: func(ctx context.Context, a map[string]any) (string, error) {
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
		{Name: "workflow_run", Description: "Run a workflow by id with a trigger and optional event data.", InputSchema: objSchema(map[string]any{"id": intProp(""), "trigger": strProp("manual | api"), "event_data": map[string]any{"type": "object"}}, []string{"id"}), Timeout: slowTimeout, Handler: func(ctx context.Context, a map[string]any) (string, error) {
			data, _ := a["event_data"].(map[string]any)
			return jsonResult(wf.Run(ctx, uint(argInt(a, "id")), argString(a, "trigger", "api"), data))
		}},
	}
}
