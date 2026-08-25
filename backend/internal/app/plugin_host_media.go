package app

import (
	"context"
	"time"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

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
