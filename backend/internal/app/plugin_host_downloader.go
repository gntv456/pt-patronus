package app

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/plugins"
)


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
