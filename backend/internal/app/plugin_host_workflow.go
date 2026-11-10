// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

import (
	"context"
	"fmt"
	"time"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/plugins"
)

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
			return nil, mcpErr(err)
		}
		if subscription.Name == "" || subscription.URL == "" {
			return nil, fmt.Errorf("缺少必填参数：name（名称）与 url（路径）都不能为空")
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
			return nil, mcpErr(err)
		}
		id, createdAt, lastCheckedAt := subscription.ID, subscription.CreatedAt, subscription.LastCheckedAt
		if err := decodeToolInput(a, subscription); err != nil {
			return nil, mcpErr(err)
		}
		subscription.ID, subscription.CreatedAt, subscription.LastCheckedAt = id, createdAt, lastCheckedAt
		if err := rss.Update(subscription); err != nil {
			return nil, mcpErr(err)
		}
		return subscription, nil
	})
	reg.Register(plugins.HostTool{
		Name: "rss_delete", Category: "rss", Permission: "rss:write", Destructive: true,
		Description: "Delete an RSS subscription and its stored items.",
		InputSchema: objSchema(map[string]any{"id": intProp("")}, []string{"id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := rss.Delete(uint(argInt(a, "id")))
		return okCount("deleted", 1, mcpErr(err))
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
			return nil, mcpErr(err)
		}
		n, err := rss.Check(ctx, r)
		if err != nil {
			return nil, mcpErr(err)
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
		return okCount("created", 1, mcpErr(err))
	})
	reg.Register(plugins.HostTool{
		Name: "file_rename", Category: "file", Permission: "file:write",
		Description: "Rename a file or directory within its current directory.",
		InputSchema: objSchema(map[string]any{"storage_id": intProp(""), "path": strProp(""), "new_name": strProp("")}, []string{"storage_id", "path", "new_name"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := fc.Rename(ctx, uint(argInt(a, "storage_id")), argString(a, "path", ""), argString(a, "new_name", ""))
		return okCount("renamed", 1, mcpErr(err))
	})
	reg.Register(plugins.HostTool{
		Name: "file_move", Category: "file", Permission: "file:write",
		Description: "Move a file or directory to another directory on the same storage backend.",
		InputSchema: objSchema(map[string]any{"storage_id": intProp(""), "source": strProp(""), "destination_dir": strProp("")}, []string{"storage_id", "source", "destination_dir"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := fc.Move(ctx, uint(argInt(a, "storage_id")), argString(a, "source", ""), argString(a, "destination_dir", ""))
		return okCount("moved", 1, mcpErr(err))
	})
	reg.Register(plugins.HostTool{
		Name: "file_copy", Category: "file", Permission: "file:write",
		Description: "Copy a file or directory to another directory on the same storage backend.",
		InputSchema: objSchema(map[string]any{"storage_id": intProp(""), "source": strProp(""), "destination_dir": strProp("")}, []string{"storage_id", "source", "destination_dir"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := fc.Copy(ctx, uint(argInt(a, "storage_id")), argString(a, "source", ""), argString(a, "destination_dir", ""))
		return okCount("copied", 1, mcpErr(err))
	})
	reg.Register(plugins.HostTool{
		Name: "file_test", Category: "file", Permission: "file:read",
		Description: "Test connectivity of a configured storage backend.",
		InputSchema: objSchema(map[string]any{"storage_id": intProp("")}, []string{"storage_id"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		if err := fc.Test(ctx, uint(argInt(a, "storage_id"))); err != nil {
			return nil, mcpErr(err)
		}
		return map[string]any{"ok": true}, nil
	})
	reg.Register(plugins.HostTool{
		Name: "file_delete", Category: "file", Permission: "file:write", Destructive: true,
		Description: "Delete a file or directory on a storage backend (irreversible).",
		InputSchema: objSchema(map[string]any{"storage_id": intProp(""), "path": strProp("")}, []string{"storage_id", "path"}),
	}, func(ctx context.Context, a map[string]any) (any, error) {
		err := fc.Delete(ctx, uint(argInt(a, "storage_id")), argString(a, "path", ""))
		return okCount("deleted", 1, mcpErr(err))
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
		InputSchema: objSchema(map[string]any{"id": intProp(""), "trigger": strProp("provenance recorded in run history; default api; e.g. manual | api | webhook | timer"), "event_data": map[string]any{"type": "object"}}, []string{"id"}),
		Timeout:     60 * time.Second,
	}, func(ctx context.Context, a map[string]any) (any, error) {
		data, _ := a["event_data"].(map[string]any)
		return wf.Run(ctx, uint(argInt(a, "id")), runTrigger(a, "api"), data)
	})
}
