package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/mcp"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

// 本文件把宿主全部 chain 能力批量注册成 plugins.HostTool，经统一 Tool RPC（GET /tools、
// POST /tools/{name}、Bridge tool.list/tool.call）暴露给外部插件。两条来源：
//  (1) adopt registerAgentTools 已构建的 35 个 mcp.Tool——零 handler 重复，仅补
//      Permission/Category/Destructive 元数据；handler 返回 json.RawMessage 透传预序列化 JSON。
//  (2) 新增 host-only tool，覆盖 agent_tools 未触及的 chain（mediaserver/cloud/brushflow/
//      rss/subtitle/file/workflow + 下载器高级操作）。
//
// registerAgentTools 是 agent 与 plugin 共享的唯一真源；此处再次调用为相同 chain 构建全新
// 闭包，无全局状态，安全。未在 agentToolMeta 声明权限的 tool 不暴露（安全默认）。

// adoptAgentTool 把一个 mcp.Tool 适配为 plugins.HostTool。
// mcp.Tool.Handler 返回 JSON 字符串，这里包成 json.RawMessage——json.Marshal 会把它当作
// 已序列化的合法 JSON 直接写入，不会双重转义（RawMessage.MarshalJSON 返回原始字节）。
func adoptAgentTool(t mcp.Tool, permission, category string, destructive bool) (plugins.HostTool, plugins.HostToolHandler) {
	return plugins.HostTool{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.InputSchema,
		Permission:  permission,
		Category:    category,
		Destructive: destructive,
	}, func(ctx context.Context, in map[string]any) (any, error) {
		s, err := t.Handler(ctx, in)
		if err != nil {
			return nil, err
		}
		if s == "" {
			return json.RawMessage("null"), nil
		}
		return json.RawMessage(s), nil
	}
}

// toolMeta 是 agent tool 的插件侧元数据补充（agent tool 自身不带权限/分类）。
type toolMeta struct {
	permission  string
	category    string
	destructive bool
}

// agentToolMeta 为每个被 adopt 的 agent tool 指定插件权限、分类、是否破坏性。
// 权限沿用 plugins 包的域级约定（downloader:read|write 等），与 host_capabilities.go 对齐。
var agentToolMeta = map[string]toolMeta{
	"system.info":                {"", "system", false},
	"sites.list":                 {"site:read", "site", false},
	"site_messages":              {"site:read", "site", false},
	"site_freeleech":             {"site:read", "site", false},
	"site_checkin_all":           {"site:action", "site", false},
	"torrent_search":             {"site:read", "site", false},
	"media_search":               {"media:read", "media", false},
	"media_detail":               {"media:read", "media", false},
	"recommend_feeds":            {"media:read", "media", false},
	"subscribe_list":             {"subscribe:read", "subscribe", false},
	"subscribe_create":           {"subscribe:write", "subscribe", false},
	"subscribe_delete":           {"subscribe:write", "subscribe", false},
	"downloaders.list":           {"downloader:read", "downloader", false},
	"torrents_list":              {"downloader:read", "downloader", false},
	"torrents_stats":             {"downloader:read", "downloader", false},
	"torrents_pause":             {"downloader:write", "downloader", false},
	"torrents_resume":            {"downloader:write", "downloader", false},
	"torrents_delete":            {"downloader:write", "downloader", true},
	"torrents_add":               {"downloader:write", "downloader", false},
	"transfer_history":           {"transfer:read", "transfer", false},
	"transfer_organize":          {"transfer:write", "transfer", false},
	"reseed_list":                {"reseed:read", "reseed", false},
	"reseed_run":                 {"reseed:write", "reseed", false},
	"dashboard_overview":         {"dashboard:read", "dashboard", false},
	"dashboard_trend":            {"dashboard:read", "dashboard", false},
	"send_notification":          {"notify:send", "notify", false},
	"filter_builtin_rules_query": {"filter:read", "filter", false},
	"filter_custom_rules_query":  {"filter:read", "filter", false},
	"filter_custom_rule_create":  {"filter:write", "filter", false},
	"filter_custom_rule_update":  {"filter:write", "filter", false},
	"filter_custom_rule_delete":  {"filter:write", "filter", true},
	"filter_rule_groups_query":   {"filter:read", "filter", false},
	"filter_rule_group_create":   {"filter:write", "filter", false},
	"filter_rule_group_update":   {"filter:write", "filter", false},
	"filter_rule_group_delete":   {"filter:write", "filter", true},
}

// registerPluginHostTools 构建 plugin HostTool 注册表并注入 Manager。由 app.go 在所有 chain
// 构造完成后调用（放置顺序是硬约束：闭包捕获 chain 指针，nil 会 panic）。
// agentTools 是 app.go 已构建的 registerAgentTools(...) 结果（同一份闭包共享，agent 侧与
// plugin 侧独立调用，无全局状态）。
func registerPluginHostTools(
	mgr *plugins.Manager,
	agentTools []mcp.Tool,
	dlCh *chain.DownloadChain, subCh *chain.SubscribeChain,
	mediaServerCh *chain.MediaServerChain, cloudCh *chain.CloudChain,
	brushCh *chain.BrushFlowChain, rssCh *chain.RssChain,
	subtitleCh *chain.SubtitleChain, transferCh *chain.TransferChain,
	keepSeedCh *chain.KeepSeedChain, fileCh *chain.FileManagerChain,
	wfCh *chain.WorkflowChain,
) {
	reg := plugins.NewHostToolRegistry()

	// (1) adopt 35 个 agent 工具（未在 agentToolMeta 声明权限的不暴露=安全默认）。
	AdoptAgentTools(reg, agentTools)

	// (2) 新增 host-only 工具（按域）。
	registerDownloaderTools(reg, dlCh)
	registerMediaServerTools(reg, mediaServerCh)
	registerSubscribeTools(reg, subCh)
	registerTransferTools(reg, transferCh)
	registerReseedTools(reg, keepSeedCh)
	registerCloudTools(reg, cloudCh)
	registerBrushFlowTools(reg, brushCh)
	registerRssTools(reg, rssCh)
	registerSubtitleTools(reg, subtitleCh)
	registerFileTools(reg, fileCh)
	registerWorkflowTools(reg, wfCh)

	mgr.SetHostTools(reg)
}

// AdoptAgentTools 把一批已构建的 agent mcp.Tool 按 agentToolMeta adopt 进注册表。
// 由 app.go 在调用 registerAgentTools(...) 拿到真实 tool 列表后调用。
func AdoptAgentTools(reg *plugins.HostToolRegistry, tools []mcp.Tool) {
	for _, t := range tools {
		meta, ok := agentToolMeta[t.Name]
		if !ok {
			continue
		}
		ht, hh := adoptAgentTool(t, meta.permission, meta.category, meta.destructive)
		reg.Register(ht, hh)
	}
}

// ===== 参数/schema 辅助（agent_tools.go 的 noArgs/hashesSchema 是局部或同包；这里补 objSchema/noArgsSchema 包级版） =====

// noArgsSchema 返回无参 tool 的 JSON Schema。与 agent_tools.go 的局部 noArgs 等价（独立副本，互不影响）。
func noArgsSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

// objSchema 构造带属性的对象 schema。
func objSchema(properties map[string]any, required []string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": properties, "required": required}
}

// argFloat 从 LLM/插件输入安全取 float64（JSON 数字默认 float64）。
func argFloat(a map[string]any, key string) float64 {
	v, ok := a[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f
	}
	return 0
}

// intProp 构造一个 integer 属性 schema。
func intProp(desc string) map[string]any {
	m := map[string]any{"type": "integer"}
	if desc != "" {
		m["description"] = desc
	}
	return m
}

// strProp 构造一个 string 属性 schema。
func strProp(desc string) map[string]any {
	m := map[string]any{"type": "string"}
	if desc != "" {
		m["description"] = desc
	}
	return m
}

func boolProp(desc string) map[string]any {
	m := map[string]any{"type": "boolean"}
	if desc != "" {
		m["description"] = desc
	}
	return m
}

func numberProp(desc string) map[string]any {
	m := map[string]any{"type": "number"}
	if desc != "" {
		m["description"] = desc
	}
	return m
}

func decodeToolInput[T any](input map[string]any, out *T) error {
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("invalid tool input: %w", err)
	}
	return nil
}

// ===== 下载器高级操作 =====

func registerDownloaderTools(reg *plugins.HostToolRegistry, dl *chain.DownloadChain) {
	// —— 只读 ——
	reg.Register(plugins.HostTool{
		Name: "torrents_files", Category: "downloader", Permission: "downloader:read",
		Description: "List files inside a torrent (path/size/priority).",
		InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hash": strProp("")}, []string{"downloader_id", "hash"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return dl.ListFiles(ctx, uint(argInt(a, "downloader_id")), argString(a, "hash", ""))
	})
	reg.Register(plugins.HostTool{
		Name: "torrents_peers", Category: "downloader", Permission: "downloader:read",
		Description: "List connected peers of a torrent.",
		InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hash": strProp("")}, []string{"downloader_id", "hash"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return dl.ListPeers(ctx, uint(argInt(a, "downloader_id")), argString(a, "hash", ""))
	})
	reg.Register(plugins.HostTool{
		Name: "torrents_trackers", Category: "downloader", Permission: "downloader:read",
		Description: "List trackers of a torrent.",
		InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hash": strProp("")}, []string{"downloader_id", "hash"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return dl.ListTrackers(ctx, uint(argInt(a, "downloader_id")), argString(a, "hash", ""))
	})
	reg.Register(plugins.HostTool{
		Name: "downloader_categories", Category: "downloader", Permission: "downloader:read",
		Description: "List available categories in a downloader.",
		InputSchema: objSchema(map[string]any{"downloader_id": intProp("")}, []string{"downloader_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return dl.ListCategories(ctx, uint(argInt(a, "downloader_id")))
	})
	reg.Register(plugins.HostTool{
		Name: "downloader_tags", Category: "downloader", Permission: "downloader:read",
		Description: "List available tags in a downloader.",
		InputSchema: objSchema(map[string]any{"downloader_id": intProp("")}, []string{"downloader_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return dl.ListTags(ctx, uint(argInt(a, "downloader_id")))
	})

	// —— 写操作 ——
	reg.Register(plugins.HostTool{
		Name: "torrents_set_category", Category: "downloader", Permission: "downloader:write",
		Description: "Set the category of torrents by hashes.",
		InputSchema: hashesSchema(),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := dl.SetCategory(ctx, uint(argInt(a, "downloader_id")), argStrSlice(a, "hashes"), argString(a, "category", ""))
		return okCount("updated", len(argStrSlice(a, "hashes")), err)
	})
	reg.Register(plugins.HostTool{
		Name: "torrents_set_tags", Category: "downloader", Permission: "downloader:write",
		Description: "Replace the tag set of torrents (set op). tags is a string array.",
		InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hashes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}, []string{"downloader_id", "hashes"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := dl.SetTags(ctx, uint(argInt(a, "downloader_id")), argStrSlice(a, "hashes"), argStrSlice(a, "tags"))
		return okCount("updated", len(argStrSlice(a, "hashes")), err)
	})
	reg.Register(plugins.HostTool{
		Name: "torrents_recheck", Category: "downloader", Permission: "downloader:write",
		Description: "Force recheck torrents by hashes.",
		InputSchema: hashesSchema(),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := dl.Recheck(ctx, uint(argInt(a, "downloader_id")), argStrSlice(a, "hashes"))
		return okCount("rechecked", len(argStrSlice(a, "hashes")), err)
	})
	reg.Register(plugins.HostTool{
		Name: "torrents_reannounce", Category: "downloader", Permission: "downloader:write",
		Description: "Reannounce torrents to trackers by hashes.",
		InputSchema: hashesSchema(),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := dl.Reannounce(ctx, uint(argInt(a, "downloader_id")), argStrSlice(a, "hashes"))
		return okCount("reannounced", len(argStrSlice(a, "hashes")), err)
	})
	reg.Register(plugins.HostTool{
		Name: "torrents_set_limits", Category: "downloader", Permission: "downloader:write",
		Description: "Per-torrent speed limits (bytes/s). upload_limit/download_limit: -1=unlimited, 0=no change.",
		InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hashes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "upload_limit": intProp("bytes/s, -1=unlimited"), "download_limit": intProp("bytes/s, -1=unlimited")}, []string{"downloader_id", "hashes"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := dl.SetLimits(ctx, uint(argInt(a, "downloader_id")), argStrSlice(a, "hashes"), downloader.Limits{
			UploadLimit:   int64(argInt(a, "upload_limit")),
			DownloadLimit: int64(argInt(a, "download_limit")),
		})
		return okCount("updated", len(argStrSlice(a, "hashes")), err)
	})
	reg.Register(plugins.HostTool{
		Name: "torrents_set_location", Category: "downloader", Permission: "downloader:write",
		Description: "Move torrents' save location.",
		InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hashes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "location": strProp("new save path")}, []string{"downloader_id", "hashes", "location"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := dl.SetLocation(ctx, uint(argInt(a, "downloader_id")), argStrSlice(a, "hashes"), argString(a, "location", ""))
		return okCount("moved", len(argStrSlice(a, "hashes")), err)
	})
	reg.Register(plugins.HostTool{
		Name: "torrents_set_share_limits", Category: "downloader", Permission: "downloader:write",
		Description: "Set share limits on torrents. ratio_limit: -2=unlimited,-1=global,>=0=value; seeding_time_limit in minutes (same convention).",
		InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hashes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "ratio_limit": map[string]any{"type": "number"}, "seeding_time_limit": intProp("minutes")}, []string{"downloader_id", "hashes"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := dl.SetShareLimits(ctx, uint(argInt(a, "downloader_id")), argStrSlice(a, "hashes"), downloader.ShareLimits{
			RatioLimit:       argFloat(a, "ratio_limit"),
			SeedingTimeLimit: argInt(a, "seeding_time_limit"),
		})
		return okCount("updated", len(argStrSlice(a, "hashes")), err)
	})
	reg.Register(plugins.HostTool{
		Name: "torrents_set_force_start", Category: "downloader", Permission: "downloader:write",
		Description: "Force start / cancel force start on torrents.",
		InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "hashes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "enable": map[string]any{"type": "boolean"}}, []string{"downloader_id", "hashes"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := dl.SetForceStart(ctx, uint(argInt(a, "downloader_id")), argStrSlice(a, "hashes"), argBool(a, "enable"))
		return okCount("updated", len(argStrSlice(a, "hashes")), err)
	})
}

// okCount 把一个仅返回 error 的写操作包装为 {<key>: n} 结果，便于插件确认影响条数。
func okCount(key string, n int, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return map[string]any{key: n}, nil
}

// ===== 媒体服务器 =====

func registerMediaServerTools(reg *plugins.HostToolRegistry, ms *chain.MediaServerChain) {
	reg.Register(plugins.HostTool{
		Name: "mediaserver_list", Category: "mediaserver", Permission: "mediaserver:read",
		Description: "List configured media servers (Emby/Plex/Jellyfin): id, name, type, address.",
		InputSchema: noArgsSchema(),
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		return ms.List()
	})
	reg.Register(plugins.HostTool{
		Name: "mediaserver_libraries", Category: "mediaserver", Permission: "mediaserver:read",
		Description: "List libraries of a media server.",
		InputSchema: objSchema(map[string]any{"server_id": intProp("")}, []string{"server_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return ms.Libraries(ctx, uint(argInt(a, "server_id")))
	})
	reg.Register(plugins.HostTool{
		Name: "mediaserver_library_scan", Category: "mediaserver", Permission: "mediaserver:write",
		Description: "Trigger a library scan on a media server (unblocks NFO/poster → library refresh).",
		InputSchema: objSchema(map[string]any{"server_id": intProp(""), "library_id": strProp("optional; empty=scan all")}, []string{"server_id"}),
		Timeout:     60 * time.Second,
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := ms.LibraryScan(ctx, uint(argInt(a, "server_id")), argString(a, "library_id", ""))
		return okCount("scanned", 1, err)
	})
	reg.Register(plugins.HostTool{
		Name: "mediaserver_refresh", Category: "mediaserver", Permission: "mediaserver:write",
		Description: "Refresh metadata of a library on a media server.",
		InputSchema: objSchema(map[string]any{"server_id": intProp(""), "library_id": strProp("")}, []string{"server_id", "library_id"}),
		Timeout:     60 * time.Second,
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := ms.Refresh(ctx, uint(argInt(a, "server_id")), argString(a, "library_id", ""))
		return okCount("refreshed", 1, err)
	})
	reg.Register(plugins.HostTool{
		Name: "mediaserver_notify", Category: "mediaserver", Permission: "mediaserver:write",
		Description: "Send an in-app notification to a single media server (Emby/Plex admin toast).",
		InputSchema: objSchema(map[string]any{"server_id": intProp(""), "title": strProp(""), "body": strProp("")}, []string{"server_id", "title", "body"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := ms.NotifyOne(ctx, uint(argInt(a, "server_id")), argString(a, "title", ""), argString(a, "body", ""))
		return okCount("notified", 1, err)
	})
	reg.Register(plugins.HostTool{
		Name: "mediaserver_playback_report", Category: "mediaserver", Permission: "mediaserver:read",
		Description: "Playback report of a media server over N days.",
		InputSchema: objSchema(map[string]any{"server_id": intProp(""), "days": intProp("default 7")}, []string{"server_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return ms.PlaybackReport(ctx, uint(argInt(a, "server_id")), argIntDefault(a, "days", 7))
	})
	reg.Register(plugins.HostTool{
		Name: "mediaserver_duplicates", Category: "mediaserver", Permission: "mediaserver:read",
		Description: "Detect duplicate media items in a library.",
		InputSchema: objSchema(map[string]any{"server_id": intProp(""), "library_id": strProp("")}, []string{"server_id", "library_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return ms.DetectDuplicates(ctx, uint(argInt(a, "server_id")), argString(a, "library_id", ""))
	})
}

// ===== 订阅 =====

func registerSubscribeTools(reg *plugins.HostToolRegistry, sub *chain.SubscribeChain) {
	reg.Register(plugins.HostTool{
		Name: "subscribe_get", Category: "subscribe", Permission: "subscribe:read",
		Description: "Get a single subscription by id.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return sub.Get(uint(argInt(a, "id")))
	})
	reg.Register(plugins.HostTool{
		Name: "subscribe_check", Category: "subscribe", Permission: "subscribe:write",
		Description: "Force-check a subscription (search + push to downloader). Returns number of new hits.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
		Timeout:     60 * time.Second,
	}, func(ctx context.Context, a map[string]any) (any, error) {
		s, err := sub.Get(uint(argInt(a, "id")))
		if err != nil {
			return nil, err
		}
		n, err := sub.Check(ctx, s)
		if err != nil {
			return nil, err
		}
		return map[string]any{"checked": n}, nil
	})
}

// ===== 整理 =====

func registerTransferTools(reg *plugins.HostToolRegistry, tr *chain.TransferChain) {
	reg.Register(plugins.HostTool{
		Name: "transfer_redo", Category: "transfer", Permission: "transfer:write",
		Description: "Re-organize a past transfer by history id.",
		InputSchema: objSchema(map[string]any{"history_id": intProp("")}, []string{"history_id"}),
		Timeout:     60 * time.Second,
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return tr.Redo(ctx, uint(argInt(a, "history_id")))
	})
	reg.Register(plugins.HostTool{
		Name: "transfer_sweep", Category: "transfer", Permission: "transfer:write",
		Description: "Trigger a full organize sweep over completed torrents (async on the host).",
		InputSchema: noArgsSchema(),
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		go tr.Sweep(context.Background())
		return map[string]any{"ok": true}, nil
	})
}

// ===== 辅种 =====

func registerReseedTools(reg *plugins.HostToolRegistry, ks *chain.KeepSeedChain) {
	reg.Register(plugins.HostTool{
		Name: "reseed_find_candidates", Category: "reseed", Permission: "reseed:read",
		Description: "Fuzzy-match cross-site seed candidates by name + size (read-only audit).",
		InputSchema: objSchema(map[string]any{"name": strProp(""), "size": map[string]any{"type": "integer", "description": "bytes"}}, []string{"name"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return ks.FindCandidates(ctx, argString(a, "name", ""), int64(argInt(a, "size"))), nil
	})
	reg.Register(plugins.HostTool{
		Name: "reseed_find_missing", Category: "reseed", Permission: "reseed:read",
		Description: "Audit which target sites are missing cross-seeds for a downloader's torrents.",
		InputSchema: objSchema(map[string]any{"downloader_id": intProp(""), "target_site_ids": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}}, []string{"downloader_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		rawSites, _ := a["target_site_ids"].([]any)
		ids := make([]uint, 0, len(rawSites))
		for _, e := range rawSites {
			if f, ok := e.(float64); ok {
				ids = append(ids, uint(f))
			}
		}
		return ks.FindMissing(ctx, uint(argInt(a, "downloader_id")), ids)
	})
	reg.Register(plugins.HostTool{
		Name: "reseed_resolve", Category: "reseed", Permission: "reseed:write",
		Description: "Resolve a cross-seed candidate task: action = add | skip.",
		InputSchema: objSchema(map[string]any{"task_id": intProp(""), "action": strProp("add | skip")}, []string{"task_id", "action"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := ks.Resolve(ctx, uint(argInt(a, "task_id")), argString(a, "action", ""))
		return okCount("resolved", 1, err)
	})
}

// ===== 云盘 / STRM =====

func registerCloudTools(reg *plugins.HostToolRegistry, cl *chain.CloudChain) {
	reg.Register(plugins.HostTool{
		Name: "cloud_list", Category: "cloud", Permission: "cloud:read",
		Description: "List configured cloud drives (115/夸克/123/Alist).",
		InputSchema: noArgsSchema(),
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		return cl.List()
	})
	cloudDriveProps := map[string]any{
		"name": strProp("Display name"), "type": strProp("115 | quark | 123"),
		"token": strProp("Provider cookie or access token; write-only"), "root_path": strProp("Accessible root path"),
		"is_default": boolProp("Use as the default cloud drive"),
	}
	reg.Register(plugins.HostTool{
		Name: "cloud_create", Category: "cloud", Permission: "cloud:write",
		Description: "Create a cloud-drive configuration. Credentials are encrypted by the host.",
		InputSchema: objSchema(cloudDriveProps, []string{"name", "type", "token"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		var in oper.CloudDriveInput
		if err := decodeToolInput(a, &in); err != nil {
			return nil, err
		}
		return cl.Create(in)
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_update", Category: "cloud", Permission: "cloud:write",
		Description: "Update a cloud-drive configuration; omit token to keep the existing credential.",
		InputSchema: objSchema(map[string]any{"id": intProp("Cloud drive id"), "name": cloudDriveProps["name"], "type": cloudDriveProps["type"], "token": cloudDriveProps["token"], "root_path": cloudDriveProps["root_path"], "is_default": cloudDriveProps["is_default"]}, []string{"id", "name", "type"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		var in oper.CloudDriveInput
		if err := decodeToolInput(a, &in); err != nil {
			return nil, err
		}
		return cl.Update(uint(argInt(a, "id")), in)
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_delete", Category: "cloud", Permission: "cloud:write", Destructive: true,
		Description: "Delete a cloud-drive configuration.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := cl.Delete(uint(argInt(a, "id")))
		return okCount("deleted", 1, err)
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_test", Category: "cloud", Permission: "cloud:read",
		Description: "Test connectivity of a configured cloud drive.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		message, err := cl.Test(ctx, uint(argInt(a, "id")))
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true, "message": message}, nil
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_browse", Category: "cloud", Permission: "cloud:read",
		Description: "Browse a cloud drive directory.",
		InputSchema: objSchema(map[string]any{"drive_id": intProp(""), "path": strProp("")}, []string{"drive_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return cl.Browse(ctx, uint(argInt(a, "drive_id")), argString(a, "path", "/"))
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_quota", Category: "cloud", Permission: "cloud:read",
		Description: "Query storage quota of a cloud drive.",
		InputSchema: objSchema(map[string]any{"drive_id": intProp("")}, []string{"drive_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return cl.Quota(ctx, uint(argInt(a, "drive_id")))
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_rules", Category: "cloud", Permission: "cloud:read",
		Description: "List STRM generation rules.",
		InputSchema: noArgsSchema(),
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		return cl.ListRules()
	})
	ruleProps := map[string]any{
		"name": strProp(""), "drive_id": intProp(""), "cloud_path": strProp(""), "output_dir": strProp(""),
		"mount_root": strProp(""), "template": strProp(""), "recursive": boolProp(""), "keep_ext": boolProp(""), "enabled": boolProp(""),
	}
	reg.Register(plugins.HostTool{
		Name: "cloud_rule_create", Category: "cloud", Permission: "cloud:write",
		Description: "Create a STRM generation rule.",
		InputSchema: objSchema(ruleProps, []string{"name", "drive_id", "cloud_path", "output_dir"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		var in oper.StrmRuleInput
		if err := decodeToolInput(a, &in); err != nil {
			return nil, err
		}
		return cl.CreateRule(in)
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_rule_update", Category: "cloud", Permission: "cloud:write",
		Description: "Update a STRM generation rule. Omitted fields keep their current values.",
		InputSchema: objSchema(map[string]any{"id": intProp(""), "name": ruleProps["name"], "drive_id": ruleProps["drive_id"], "cloud_path": ruleProps["cloud_path"], "output_dir": ruleProps["output_dir"], "mount_root": ruleProps["mount_root"], "template": ruleProps["template"], "recursive": ruleProps["recursive"], "keep_ext": ruleProps["keep_ext"], "enabled": ruleProps["enabled"]}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		var in oper.StrmRuleInput
		if err := decodeToolInput(a, &in); err != nil {
			return nil, err
		}
		return cl.UpdateRule(uint(argInt(a, "id")), in)
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_rule_delete", Category: "cloud", Permission: "cloud:write", Destructive: true,
		Description: "Delete a STRM generation rule.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := cl.DeleteRule(uint(argInt(a, "id")))
		return okCount("deleted", 1, err)
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_records", Category: "cloud", Permission: "cloud:read",
		Description: "List generated STRM records, optionally filtered by cloud drive.",
		InputSchema: objSchema(map[string]any{"drive_id": intProp("0 lists all drives")}, nil),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return cl.ListRecords(uint(argInt(a, "drive_id")))
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_record_delete", Category: "cloud", Permission: "cloud:write", Destructive: true,
		Description: "Delete a generated STRM record.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := cl.DeleteRecord(uint(argInt(a, "id")))
		return okCount("deleted", 1, err)
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_generate_strm", Category: "cloud", Permission: "cloud:write",
		Description: "Generate .strm files for a STRM rule.",
		InputSchema: objSchema(map[string]any{"rule_id": intProp("")}, []string{"rule_id"}),
		Timeout:     60 * time.Second,
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return cl.GenerateStrm(ctx, uint(argInt(a, "rule_id")))
	})
	reg.Register(plugins.HostTool{
		Name: "cloud_generate_all", Category: "cloud", Permission: "cloud:write",
		Description: "Generate .strm files for all enabled rules (async on the host).",
		InputSchema: noArgsSchema(),
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		go cl.GenerateAll(context.Background())
		return map[string]any{"ok": true}, nil
	})
}

// ===== 刷流 =====

func registerBrushFlowTools(reg *plugins.HostToolRegistry, bf *chain.BrushFlowChain) {
	reg.Register(plugins.HostTool{
		Name: "brushflow_tasks_list", Category: "brushflow", Permission: "brushflow:read",
		Description: "List brush-flow tasks.",
		InputSchema: noArgsSchema(),
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		return bf.ListTasks()
	})
	brushProps := map[string]any{
		"name": strProp(""), "site_id": intProp(""), "downloader_id": intProp(""), "enabled": boolProp(""),
		"keyword": strProp(""), "category": strProp(""), "min_size_mb": intProp(""), "max_size_mb": intProp(""),
		"min_seeders": intProp(""), "max_seeders": intProp(""), "free_only": boolProp(""), "paused": boolProp(""),
		"filter_groups": strProp(""), "delete_ratio": numberProp(""), "delete_seed_hours": numberProp(""),
		"delete_upload_mb": intProp(""), "delete_files": boolProp(""), "max_seeds": intProp(""),
		"save_path": strProp(""), "category_dl": strProp(""),
	}
	reg.Register(plugins.HostTool{
		Name: "brushflow_task_create", Category: "brushflow", Permission: "brushflow:write",
		Description: "Create a brush-flow task.",
		InputSchema: objSchema(brushProps, []string{"name", "site_id", "downloader_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		var task models.BrushFlowTask
		if err := decodeToolInput(a, &task); err != nil {
			return nil, err
		}
		if task.Name == "" || task.SiteID == 0 || task.DownloaderID == 0 {
			return nil, fmt.Errorf("name/site_id/downloader_id are required")
		}
		if task.MaxSeeds <= 0 {
			task.MaxSeeds = 10
		}
		if err := bf.CreateTask(&task); err != nil {
			return nil, err
		}
		return task, nil
	})
	reg.Register(plugins.HostTool{
		Name: "brushflow_task_update", Category: "brushflow", Permission: "brushflow:write",
		Description: "Update a brush-flow task. Omitted fields keep their current values.",
		InputSchema: objSchema(map[string]any{"id": intProp(""), "name": brushProps["name"], "site_id": brushProps["site_id"], "downloader_id": brushProps["downloader_id"], "enabled": brushProps["enabled"], "keyword": brushProps["keyword"], "category": brushProps["category"], "min_size_mb": brushProps["min_size_mb"], "max_size_mb": brushProps["max_size_mb"], "min_seeders": brushProps["min_seeders"], "max_seeders": brushProps["max_seeders"], "free_only": brushProps["free_only"], "paused": brushProps["paused"], "filter_groups": brushProps["filter_groups"], "delete_ratio": brushProps["delete_ratio"], "delete_seed_hours": brushProps["delete_seed_hours"], "delete_upload_mb": brushProps["delete_upload_mb"], "delete_files": brushProps["delete_files"], "max_seeds": brushProps["max_seeds"], "save_path": brushProps["save_path"], "category_dl": brushProps["category_dl"]}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		task, err := bf.GetTask(uint(argInt(a, "id")))
		if err != nil {
			return nil, err
		}
		id, totalAdded, totalDeleted, totalUpload := task.ID, task.TotalAdded, task.TotalDeleted, task.TotalUpload
		createdAt, lastSelectAt, lastSweepAt, lastError := task.CreatedAt, task.LastSelectAt, task.LastSweepAt, task.LastError
		if err := decodeToolInput(a, task); err != nil {
			return nil, err
		}
		task.ID, task.TotalAdded, task.TotalDeleted, task.TotalUpload = id, totalAdded, totalDeleted, totalUpload
		task.CreatedAt, task.LastSelectAt, task.LastSweepAt, task.LastError = createdAt, lastSelectAt, lastSweepAt, lastError
		if err := bf.UpdateTask(task); err != nil {
			return nil, err
		}
		return task, nil
	})
	reg.Register(plugins.HostTool{
		Name: "brushflow_task_delete", Category: "brushflow", Permission: "brushflow:write", Destructive: true,
		Description: "Delete a brush-flow task.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := bf.DeleteTask(uint(argInt(a, "id")))
		return okCount("deleted", 1, err)
	})
	reg.Register(plugins.HostTool{
		Name: "brushflow_task_get", Category: "brushflow", Permission: "brushflow:read",
		Description: "Get a brush-flow task by id.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return bf.GetTask(uint(argInt(a, "id")))
	})
	reg.Register(plugins.HostTool{
		Name: "brushflow_seeds", Category: "brushflow", Permission: "brushflow:read",
		Description: "List seed history of a brush-flow task.",
		InputSchema: objSchema(map[string]any{"task_id": intProp("")}, []string{"task_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return bf.ListSeeds(uint(argInt(a, "task_id")))
	})
	reg.Register(plugins.HostTool{
		Name: "brushflow_run_select", Category: "brushflow", Permission: "brushflow:write",
		Description: "Manually trigger seed selection of a brush-flow task.",
		InputSchema: objSchema(map[string]any{"task_id": intProp("")}, []string{"task_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := bf.RunSelect(ctx, uint(argInt(a, "task_id")))
		return okCount("selected", 1, err)
	})
	reg.Register(plugins.HostTool{
		Name: "brushflow_run_sweep", Category: "brushflow", Permission: "brushflow:write",
		Description: "Manually trigger sweep (delete) of a brush-flow task.",
		InputSchema: objSchema(map[string]any{"task_id": intProp("")}, []string{"task_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := bf.RunSweep(ctx, uint(argInt(a, "task_id")))
		return okCount("swept", 1, err)
	})
}

// ===== RSS =====

func registerRssTools(reg *plugins.HostToolRegistry, rss *chain.RssChain) {
	reg.Register(plugins.HostTool{
		Name: "rss_list", Category: "rss", Permission: "rss:read",
		Description: "List RSS subscriptions.",
		InputSchema: noArgsSchema(),
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		return rss.List()
	})
	rssProps := map[string]any{
		"name": strProp(""), "url": strProp(""), "site_id": intProp("0 for anonymous RSS"), "enabled": boolProp(""),
		"interval_min": intProp(""), "title_include": strProp(""), "title_exclude": strProp(""), "min_size": intProp("bytes"),
		"max_size": intProp("bytes"), "resolution": strProp(""), "auto_download": boolProp(""), "downloader_id": intProp(""),
		"save_path": strProp(""), "category": strProp(""), "tags": strProp(""), "filter_groups": strProp(""),
	}
	reg.Register(plugins.HostTool{
		Name: "rss_get", Category: "rss", Permission: "rss:read",
		Description: "Get an RSS subscription by id.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return rss.Get(uint(argInt(a, "id")))
	})
	reg.Register(plugins.HostTool{
		Name: "rss_create", Category: "rss", Permission: "rss:write",
		Description: "Create an RSS subscription.",
		InputSchema: objSchema(rssProps, []string{"name", "url"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		var subscription models.RssSubscription
		if err := decodeToolInput(a, &subscription); err != nil {
			return nil, err
		}
		if subscription.Name == "" || subscription.URL == "" {
			return nil, fmt.Errorf("name and url are required")
		}
		if subscription.IntervalMin <= 0 {
			subscription.IntervalMin = 30
		}
		return rss.Create(&subscription)
	})
	reg.Register(plugins.HostTool{
		Name: "rss_update", Category: "rss", Permission: "rss:write",
		Description: "Update an RSS subscription. Omitted fields keep their current values.",
		InputSchema: objSchema(map[string]any{"id": intProp(""), "name": rssProps["name"], "url": rssProps["url"], "site_id": rssProps["site_id"], "enabled": rssProps["enabled"], "interval_min": rssProps["interval_min"], "title_include": rssProps["title_include"], "title_exclude": rssProps["title_exclude"], "min_size": rssProps["min_size"], "max_size": rssProps["max_size"], "resolution": rssProps["resolution"], "auto_download": rssProps["auto_download"], "downloader_id": rssProps["downloader_id"], "save_path": rssProps["save_path"], "category": rssProps["category"], "tags": rssProps["tags"], "filter_groups": rssProps["filter_groups"]}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		subscription, err := rss.Get(uint(argInt(a, "id")))
		if err != nil {
			return nil, err
		}
		id, createdAt, lastCheckedAt := subscription.ID, subscription.CreatedAt, subscription.LastCheckedAt
		if err := decodeToolInput(a, subscription); err != nil {
			return nil, err
		}
		subscription.ID, subscription.CreatedAt, subscription.LastCheckedAt = id, createdAt, lastCheckedAt
		if err := rss.Update(subscription); err != nil {
			return nil, err
		}
		return subscription, nil
	})
	reg.Register(plugins.HostTool{
		Name: "rss_delete", Category: "rss", Permission: "rss:write", Destructive: true,
		Description: "Delete an RSS subscription and its stored items.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := rss.Delete(uint(argInt(a, "id")))
		return okCount("deleted", 1, err)
	})
	reg.Register(plugins.HostTool{
		Name: "rss_items", Category: "rss", Permission: "rss:read",
		Description: "List parsed items of an RSS subscription.",
		InputSchema: objSchema(map[string]any{"id": intProp(""), "limit": intProp("default 50")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return rss.ListItems(uint(argInt(a, "id")), argIntDefault(a, "limit", 50))
	})
	reg.Register(plugins.HostTool{
		Name: "rss_check", Category: "rss", Permission: "rss:write",
		Description: "Force-check an RSS subscription. Returns number of new items.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
		Timeout:     60 * time.Second,
	}, func(ctx context.Context, a map[string]any) (any, error) {
		r, err := rss.Get(uint(argInt(a, "id")))
		if err != nil {
			return nil, err
		}
		n, err := rss.Check(ctx, r)
		if err != nil {
			return nil, err
		}
		return map[string]any{"checked": n}, nil
	})
}

// ===== 字幕 =====

func registerSubtitleTools(reg *plugins.HostToolRegistry, st *chain.SubtitleChain) {
	reg.Register(plugins.HostTool{
		Name: "subtitle_search", Category: "subtitle", Permission: "subtitle:read",
		Description: "Cross-source subtitle search (ASSRT/OpenSubtitles/SubHD/Zimuku).",
		InputSchema: objSchema(map[string]any{"query": strProp("movie/episode name"), "language": strProp("zh/en/...")}, []string{"query"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return st.Search(ctx, argString(a, "query", ""), argString(a, "language", ""))
	})
	// TODO(subtitle_download): 需从 search 结果回传 subtitle.Subtitle 结构体；当前 search 返回的
	// JSON 可由插件回传 input.sub，后续按需补结构体反序列化。
}

// ===== 文件存储 =====

func registerFileTools(reg *plugins.HostToolRegistry, fc *chain.FileManagerChain) {
	reg.Register(plugins.HostTool{
		Name: "file_list", Category: "file", Permission: "file:read",
		Description: "List entries of a storage backend (WebDAV/Alist/SMB) directory.",
		InputSchema: objSchema(map[string]any{"storage_id": intProp(""), "path": strProp("")}, []string{"storage_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		return fc.List(ctx, uint(argInt(a, "storage_id")), argString(a, "path", "/"))
	})
	reg.Register(plugins.HostTool{
		Name: "file_mkdir", Category: "file", Permission: "file:write",
		Description: "Create a directory on a storage backend.",
		InputSchema: objSchema(map[string]any{"storage_id": intProp(""), "path": strProp("")}, []string{"storage_id", "path"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := fc.Mkdir(ctx, uint(argInt(a, "storage_id")), argString(a, "path", ""))
		return okCount("created", 1, err)
	})
	reg.Register(plugins.HostTool{
		Name: "file_rename", Category: "file", Permission: "file:write",
		Description: "Rename a file or directory within its current directory.",
		InputSchema: objSchema(map[string]any{"storage_id": intProp(""), "path": strProp(""), "new_name": strProp("")}, []string{"storage_id", "path", "new_name"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := fc.Rename(ctx, uint(argInt(a, "storage_id")), argString(a, "path", ""), argString(a, "new_name", ""))
		return okCount("renamed", 1, err)
	})
	reg.Register(plugins.HostTool{
		Name: "file_move", Category: "file", Permission: "file:write",
		Description: "Move a file or directory to another directory on the same storage backend.",
		InputSchema: objSchema(map[string]any{"storage_id": intProp(""), "source": strProp(""), "destination_dir": strProp("")}, []string{"storage_id", "source", "destination_dir"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := fc.Move(ctx, uint(argInt(a, "storage_id")), argString(a, "source", ""), argString(a, "destination_dir", ""))
		return okCount("moved", 1, err)
	})
	reg.Register(plugins.HostTool{
		Name: "file_copy", Category: "file", Permission: "file:write",
		Description: "Copy a file or directory to another directory on the same storage backend.",
		InputSchema: objSchema(map[string]any{"storage_id": intProp(""), "source": strProp(""), "destination_dir": strProp("")}, []string{"storage_id", "source", "destination_dir"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := fc.Copy(ctx, uint(argInt(a, "storage_id")), argString(a, "source", ""), argString(a, "destination_dir", ""))
		return okCount("copied", 1, err)
	})
	reg.Register(plugins.HostTool{
		Name: "file_test", Category: "file", Permission: "file:read",
		Description: "Test connectivity of a configured storage backend.",
		InputSchema: objSchema(map[string]any{"storage_id": intProp("")}, []string{"storage_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		if err := fc.Test(ctx, uint(argInt(a, "storage_id"))); err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	})
	reg.Register(plugins.HostTool{
		Name: "file_delete", Category: "file", Permission: "file:write", Destructive: true,
		Description: "Delete a file or directory on a storage backend (irreversible).",
		InputSchema: objSchema(map[string]any{"storage_id": intProp(""), "path": strProp("")}, []string{"storage_id", "path"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := fc.Delete(ctx, uint(argInt(a, "storage_id")), argString(a, "path", ""))
		return okCount("deleted", 1, err)
	})
	// TODO(file_upload/file_download): 二进制流，不适合 JSON RPC；后续可走 multipart 端点。
}

// ===== 工作流 =====

func registerWorkflowTools(reg *plugins.HostToolRegistry, wf *chain.WorkflowChain) {
	reg.Register(plugins.HostTool{
		Name: "workflow_list", Category: "workflow", Permission: "workflow:read",
		Description: "List workflows.",
		InputSchema: noArgsSchema(),
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		return wf.List()
	})
	reg.Register(plugins.HostTool{
		Name: "workflow_catalog", Category: "workflow", Permission: "workflow:read",
		Description: "List available workflow node types (catalog).",
		InputSchema: noArgsSchema(),
	}, func(ctx context.Context, _ map[string]any) (any, error) {
		return wf.Catalog(), nil
	})
	reg.Register(plugins.HostTool{
		Name: "workflow_run", Category: "workflow", Permission: "workflow:write",
		Description: "Run a workflow by id with a trigger and optional event data.",
		InputSchema: objSchema(map[string]any{"id": intProp(""), "trigger": strProp("manual | api"), "event_data": map[string]any{"type": "object"}}, []string{"id"}),
		Timeout:     60 * time.Second,
	}, func(ctx context.Context, a map[string]any) (any, error) {
		data, _ := a["event_data"].(map[string]any)
		return wf.Run(ctx, uint(argInt(a, "id")), argString(a, "trigger", "api"), data)
	})
}
