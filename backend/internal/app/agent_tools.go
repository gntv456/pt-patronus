package app

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/filter"
	"github.com/hxpt/ptpatronus/internal/mcp"
	"github.com/hxpt/ptpatronus/internal/notification"
)

// registerAgentTools 构建 AI Agent 的全功能工具集（读+写），绑定各业务 chain。
// 与公开 MCP 工具（/api/v1/mcp）分离：MCP 公开只读；Agent 仅 admin 可用且含动作工具。
// 末尾 7 个 chain（mediaServerCh/cloudCh/brushCh/rssCh/subtitleCh/fileCh/wfCh）由
// agent_tools_ext.go 的 build* 函数展开为各域工具，使 agent 与 plugin Host API 能力对等。
func registerAgentTools(
	siteOp *oper.SiteOper, siteCh *chain.SiteChain,
	subCh *chain.SubscribeChain,
	dlOp *oper.DownloaderOper, dlCh *chain.DownloadChain,
	mediaCh *chain.MediaChain, recommend *chain.RecommendChain,
	transfer *chain.TransferChain, keepseed *chain.KeepSeedChain,
	dash *chain.DashboardChain, notify *chain.NotificationChain,
	filterStore *filter.Store,
	mediaServerCh *chain.MediaServerChain, cloudCh *chain.CloudChain,
	brushCh *chain.BrushFlowChain, rssCh *chain.RssChain,
	subtitleCh *chain.SubtitleChain, fileCh *chain.FileManagerChain,
	wfCh *chain.WorkflowChain,
) []mcp.Tool {
	var tools []mcp.Tool
	reg := func(t mcp.Tool) { tools = append(tools, t) }
	noArgs := map[string]any{"type": "object", "properties": map[string]any{}}

	// —— 系统 ——
	reg(mcp.Tool{Name: "system.info", Description: "Return PTPatronus version and counts of sites/subscriptions/downloaders.", InputSchema: noArgs, Handler: func(ctx context.Context, _ map[string]any) (string, error) {
		return mcpJSON(systemInfoPayload(siteOp, subCh, dlOp)), nil
	}})

	// —— 站点 ——
	reg(mcp.Tool{Name: "sites.list", Description: "List configured PT sites (id, name, platform, base_url). Includes 'total' and a 'duplicates' map of sites sharing the same registered domain (same site added twice).", InputSchema: noArgs, Handler: func(_ context.Context, _ map[string]any) (string, error) {
		payload, err := sitesListPayload(siteOp)
		return mcpJSON(payload), mcpErr(err)
	}})
	reg(mcp.Tool{Name: "site_messages", Description: "Read recent PT site inbox messages across all sites.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"mailbox": map[string]any{"type": "string", "description": "inbox | sent | system (default inbox)"}}, "required": []string{}}, Handler: func(ctx context.Context, a map[string]any) (string, error) {
		mb := argString(a, "mailbox", "inbox")
		return mcpJSON(siteCh.MessagesAll(ctx, mailboxType(mb), false)), nil
	}})
	reg(mcp.Tool{Name: "site_freeleech", Description: "List current site-wide freeleech/promotions across all sites.", InputSchema: noArgs, Handler: func(ctx context.Context, _ map[string]any) (string, error) {
		results, err := siteCh.FreeleechAll(ctx, false)
		if err != nil {
			return "", err
		}
		return mcpJSON(results), nil
	}})
	reg(mcp.Tool{Name: "site_checkin_all", Description: "Sign in (check-in) on all sites that support it, for daily bonus.", InputSchema: noArgs, Handler: func(ctx context.Context, _ map[string]any) (string, error) {
		results, err := siteCh.CheckinAll(ctx)
		if err != nil {
			return mcpJSON(map[string]any{"results": []any{}, "list_error": err.Error()}), nil
		}
		return mcpJSON(results), nil
	}})

	// —— 媒体 ——
	reg(mcp.Tool{Name: "media_search", Description: "Search movies/TV across metadata sources (Douban/TMDB/Bangumi). Returns title/year/source/source_id/poster/rating.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"keyword": map[string]any{"type": "string"}, "year": map[string]any{"type": "integer"}, "filter": map[string]any{"type": "string", "description": "video | movie | tv (default video)"}}, "required": []string{"keyword"}}, Handler: func(ctx context.Context, a map[string]any) (string, error) {
		r, err := mediaCh.Search(ctx, argString(a, "keyword", ""), argInt(a, "year"), argString(a, "filter", "video"))
		return mcpJSON(r), mcpErr(err)
	}})
	reg(mcp.Tool{Name: "media_detail", Description: "Get full detail (cast/synopsis/runtime) of a media item by source+source_id (from media_search).", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"source": map[string]any{"type": "string"}, "source_id": map[string]any{"type": "string"}}, "required": []string{"source", "source_id"}}, Handler: func(ctx context.Context, a map[string]any) (string, error) {
		mi, err := mediaCh.Detail(ctx, argString(a, "source", ""), argString(a, "source_id", ""))
		return mcpJSON(mi), mcpErr(err)
	}})
	reg(mcp.Tool{Name: "recommend_feeds", Description: "Trending/hot media feeds (Douban now-playing / TMDB trending / Bangumi hot / Douban charts topMovie|topTv|weekly).", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"source": map[string]any{"type": "string", "description": "douban | tmdb | bangumi (optional, empty=all)"}, "category": map[string]any{"type": "string", "description": "movie | tv | anime | coming | topMovie | topTv | weekly (optional)"}}, "required": []string{}}, Handler: func(ctx context.Context, a map[string]any) (string, error) {
		return mcpJSON(recommend.RecommendFeeds(ctx, argString(a, "source", ""), argString(a, "category", ""), 1, false)), nil
	}})
	reg(mcp.Tool{Name: "torrent_search", Description: "Search torrents across PT sites by keyword (read-only, does not download). Returns title/size/seeders/download_url.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"keyword": map[string]any{"type": "string"}, "category": map[string]any{"type": "string"}}, "required": []string{"keyword"}}, Handler: func(ctx context.Context, a map[string]any) (string, error) {
		results, err := siteCh.SearchSites(ctx, argString(a, "keyword", ""), argString(a, "category", ""), nil)
		if err != nil {
			return "", err
		}
		return mcpJSON(results), nil
	}})

	// —— 订阅 ——
	reg(mcp.Tool{Name: "subscribe_list", Description: "List media subscriptions (title/type/year/source/status).", InputSchema: noArgs, Handler: func(_ context.Context, _ map[string]any) (string, error) {
		s, err := subCh.List()
		return mcpJSON(s), mcpErr(err)
	}})
	reg(mcp.Tool{Name: "subscribe_create", Description: "Create a media subscription (auto-search + push to downloader on schedule). Requires title/type/source/source_id from media_search.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"title": map[string]any{"type": "string"}, "type": map[string]any{"type": "string", "description": "movie | tv"}, "source": map[string]any{"type": "string", "description": "tmdb | douban | bangumi"}, "source_id": map[string]any{"type": "string"}, "year": map[string]any{"type": "integer"}, "resolution": map[string]any{"type": "string"}}, "required": []string{"title", "type", "source", "source_id"}}, Handler: func(ctx context.Context, a map[string]any) (string, error) {
		s, err := subCh.Create(ctx, &models.Subscription{
			Title: argString(a, "title", ""), Type: argString(a, "type", ""), Category: argString(a, "type", ""),
			Source: argString(a, "source", ""), SourceID: argString(a, "source_id", ""),
			Year: argInt(a, "year"), Resolution: argString(a, "resolution", ""), Status: "subscribed",
		})
		if err != nil {
			return "", err
		}
		return mcpJSON(s), nil
	}})
	reg(mcp.Tool{Name: "subscribe_delete", Description: "Delete a subscription by id.", Risk: mcp.RiskDestructive, RiskNote: "删除媒体订阅及其搜索历史", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "integer"}}, "required": []string{"id"}}, Handler: func(_ context.Context, a map[string]any) (string, error) {
		id := uint(argInt(a, "id"))
		if err := subCh.Delete(id); err != nil {
			return "", err
		}
		return mcpJSON(map[string]any{"deleted": id}), nil
	}})

	// —— 下载器 / 种子 ——
	reg(mcp.Tool{Name: "downloaders.list", Description: "List downloaders (id, name, type, is_default).", InputSchema: noArgs, Handler: func(_ context.Context, _ map[string]any) (string, error) {
		d, err := dlOp.List()
		return mcpJSON(d), mcpErr(err)
	}})
	reg(mcp.Tool{Name: "torrents_list", Description: "List torrents in a downloader (hash/name/state/size/progress/category/tags).", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"downloader_id": map[string]any{"type": "integer"}}, "required": []string{"downloader_id"}}, Handler: func(ctx context.Context, a map[string]any) (string, error) {
		t, err := dlCh.List(ctx, uint(argInt(a, "downloader_id")))
		return mcpJSON(t), mcpErr(err)
	}})
	reg(mcp.Tool{Name: "torrents_stats", Description: "Global speed/session stats of a downloader.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"downloader_id": map[string]any{"type": "integer"}}, "required": []string{"downloader_id"}}, Handler: func(ctx context.Context, a map[string]any) (string, error) {
		s, err := dlCh.Stats(ctx, uint(argInt(a, "downloader_id")))
		return mcpJSON(s), mcpErr(err)
	}})
	reg(mcp.Tool{Name: "torrents_pause", Description: "Pause torrents by hashes in a downloader.", InputSchema: hashesSchema(), Handler: func(ctx context.Context, a map[string]any) (string, error) {
		return mcpJSON(map[string]any{"paused": len(argStrSlice(a, "hashes"))}), dlCh.Pause(ctx, uint(argInt(a, "downloader_id")), argStrSlice(a, "hashes"))
	}})
	reg(mcp.Tool{Name: "torrents_resume", Description: "Resume torrents by hashes in a downloader.", InputSchema: hashesSchema(), Handler: func(ctx context.Context, a map[string]any) (string, error) {
		return mcpJSON(map[string]any{"resumed": len(argStrSlice(a, "hashes"))}), dlCh.Resume(ctx, uint(argInt(a, "downloader_id")), argStrSlice(a, "hashes"))
	}})
	reg(mcp.Tool{Name: "torrents_delete", Description: "Delete torrents by hashes (irreversible). Set delete_files=true to also remove data.", Risk: mcp.RiskDestructive, RiskNote: "删除下载器种子（delete_files=true 时同时删除磁盘数据文件，不可恢复）", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"downloader_id": map[string]any{"type": "integer"}, "hashes": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1}, "delete_files": map[string]any{"type": "boolean"}}, "required": []string{"downloader_id", "hashes"}}, Handler: func(ctx context.Context, a map[string]any) (string, error) {
		return mcpJSON(map[string]any{"deleted": len(argStrSlice(a, "hashes"))}), dlCh.Delete(ctx, uint(argInt(a, "downloader_id")), argStrSlice(a, "hashes"), argBool(a, "delete_files"))
	}})
	reg(mcp.Tool{Name: "torrents_add", Description: "Add a torrent by magnet/URL to a downloader.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"downloader_id": map[string]any{"type": "integer"}, "url": map[string]any{"type": "string"}, "category": map[string]any{"type": "string"}}, "required": []string{"downloader_id", "url"}}, Handler: func(ctx context.Context, a map[string]any) (string, error) {
		opts := downloader.AddOptions{URL: argString(a, "url", ""), Category: argString(a, "category", "")}
		// URL 可能来自 PT 站（需 cookie 下载），反查站点设 SiteID。
		if st, _ := siteOp.FindByDownloadURL(opts.URL); st != nil {
			opts.SiteID = st.ID
		}
		err := dlCh.Add(ctx, uint(argInt(a, "downloader_id")), opts)
		return mcpJSON(map[string]any{"added": err == nil}), mcpErr(err)
	}})

	// —— 整理 ——
	reg(mcp.Tool{Name: "transfer_history", Description: "List media organize/transfer history (rename/hardlink/scrape results).", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"status": map[string]any{"type": "string", "description": "success | failed | (empty=all)"}, "limit": map[string]any{"type": "integer"}}, "required": []string{}}, Handler: func(_ context.Context, a map[string]any) (string, error) {
		h, err := transfer.History(argString(a, "status", ""), argIntDefault(a, "limit", 50))
		return mcpJSON(h), mcpErr(err)
	}})
	reg(mcp.Tool{Name: "transfer_organize", Description: "Manually organize a completed torrent (identify→rename→hardlink→scrape NFO/poster→library).", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"downloader_id": map[string]any{"type": "integer"}, "hash": map[string]any{"type": "string"}}, "required": []string{"downloader_id", "hash"}}, Handler: func(ctx context.Context, a map[string]any) (string, error) {
		r, err := transfer.ManualOrganize(ctx, uint(argInt(a, "downloader_id")), argString(a, "hash", ""))
		return mcpJSON(r), mcpErr(err)
	}})

	// —— 辅种 ——
	reg(mcp.Tool{Name: "reseed_list", Description: "List cross-seed (IYUU) candidate tasks.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"status": map[string]any{"type": "string"}}, "required": []string{}}, Handler: func(_ context.Context, a map[string]any) (string, error) {
		t, err := keepseed.ListTasks(argString(a, "status", ""), uint(argIntDefault(a, "site_id", 0)))
		return mcpJSON(t), mcpErr(err)
	}})
	reg(mcp.Tool{Name: "reseed_run", Description: "Run IYUU precise cross-seed scan on a downloader.", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"downloader_id": map[string]any{"type": "integer"}}, "required": []string{"downloader_id"}}, Handler: func(ctx context.Context, a map[string]any) (string, error) {
		t, err := keepseed.Reseed(ctx, uint(argInt(a, "downloader_id")))
		return mcpJSON(t), mcpErr(err)
	}})

	// —— 仪表盘 ——
	reg(mcp.Tool{Name: "dashboard_overview", Description: "Aggregated cross-site overview: per-site stats (upload/ratio/bonus/seeding/level) + grand totals.", InputSchema: noArgs, Handler: func(ctx context.Context, _ map[string]any) (string, error) {
		o, err := dash.Overview(ctx)
		return mcpJSON(o), mcpErr(err)
	}})
	reg(mcp.Tool{Name: "dashboard_trend", Description: "Daily upload/download/seeding trend over N days (default 7).", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"days": map[string]any{"type": "integer"}}, "required": []string{}}, Handler: func(ctx context.Context, a map[string]any) (string, error) {
		t, err := dash.Trend(ctx, argIntDefault(a, "days", 7))
		return mcpJSON(t), mcpErr(err)
	}})

	// —— 通知 ——
	reg(mcp.Tool{Name: "send_notification", Description: "Send a notification to all configured channels (Bark/TG/QQ/WeChat/Webhook/Email).", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"title": map[string]any{"type": "string"}, "body": map[string]any{"type": "string"}, "level": map[string]any{"type": "string", "description": "info | warning | danger | celebration"}}, "required": []string{"title", "body"}}, Handler: func(ctx context.Context, a map[string]any) (string, error) {
		r := notify.Notify(ctx, notification.Message{Title: argString(a, "title", ""), Body: argString(a, "body", ""), Level: argString(a, "level", "info")})
		return mcpJSON(r), nil
	}})

	// —— 过滤规则 ——
	// 无需单独 admin 闸门：整条 Agent 路由已在 middleware.RequireAdmin 保护之下。
	reg(mcp.Tool{Name: "filter_builtin_rules_query", Description: "查询全部内置过滤规则（如 4K/1080P/H265/BLURAY/CNSUB 等共 22 条，含 id/name/include/exclude/size_range/seeders/publish_time）。", InputSchema: noArgs, Handler: func(_ context.Context, _ map[string]any) (string, error) {
		return mcpJSON(filter.ListBuiltins()), nil
	}})
	reg(mcp.Tool{Name: "filter_custom_rules_query", Description: "查询全部用户自定义过滤规则（自定义 id 可覆盖同名内置规则）。", InputSchema: noArgs, Handler: func(_ context.Context, _ map[string]any) (string, error) {
		rs, err := filterStore.GetCustomRules()
		return mcpJSON(rs), mcpErr(err)
	}})
	reg(mcp.Tool{Name: "filter_custom_rule_create", Description: "新建一条自定义过滤规则。id 仅限字母数字且不可与已有自定义重复；include/exclude 为单个正则字符串（IGNORECASE）；size_range 形如 1000-5000/>1000/<5000（MB）；publish_time 形如 60 或 60-1440（分钟）。", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"}, "include": map[string]any{"type": "string", "description": "命中即保留的正则（IGNORECASE）"}, "exclude": map[string]any{"type": "string", "description": "命中即拒绝的正则（IGNORECASE）"}, "size_range": map[string]any{"type": "string", "description": "MB，如 1000-5000 / >1000 / <5000"}, "seeders": map[string]any{"type": "integer", "description": "最少做种人数"}, "publish_time": map[string]any{"type": "string", "description": "分钟，如 60 或 60-1440"}}, "required": []string{"id"}}, Handler: func(_ context.Context, a map[string]any) (string, error) {
		r, err := filterStore.AddCustomRule(filter.CustomRuleInput{
			ID:          argString(a, "id", ""),
			Name:        argString(a, "name", ""),
			Include:     argString(a, "include", ""),
			Exclude:     argString(a, "exclude", ""),
			SizeRange:   argString(a, "size_range", ""),
			Seeders:     argInt(a, "seeders"),
			PublishTime: argString(a, "publish_time", ""),
		})
		if err != nil {
			return "", err
		}
		return mcpJSON(r), nil
	}})
	reg(mcp.Tool{Name: "filter_custom_rule_update", Description: "按 old_id 定位并整体替换一条自定义过滤规则（允许同时改 id）。新 id 不可与其他自定义规则冲突。其余字段语义同 create。", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"old_id": map[string]any{"type": "string", "description": "待更新的现有规则 id"}, "id": map[string]any{"type": "string"}, "name": map[string]any{"type": "string"}, "include": map[string]any{"type": "string"}, "exclude": map[string]any{"type": "string"}, "size_range": map[string]any{"type": "string"}, "seeders": map[string]any{"type": "integer"}, "publish_time": map[string]any{"type": "string"}}, "required": []string{"old_id", "id"}}, Handler: func(_ context.Context, a map[string]any) (string, error) {
		r, err := filterStore.UpdateCustomRule(argString(a, "old_id", ""), filter.CustomRuleInput{
			ID:          argString(a, "id", ""),
			Name:        argString(a, "name", ""),
			Include:     argString(a, "include", ""),
			Exclude:     argString(a, "exclude", ""),
			SizeRange:   argString(a, "size_range", ""),
			Seeders:     argInt(a, "seeders"),
			PublishTime: argString(a, "publish_time", ""),
		})
		if err != nil {
			return "", err
		}
		return mcpJSON(r), nil
	}})
	reg(mcp.Tool{Name: "filter_custom_rule_delete", Description: "按 id 删除一条自定义过滤规则。若被任一规则组引用则拒绝删除。", Risk: mcp.RiskDestructive, RiskNote: "删除自定义过滤规则（匹配行为立即改变）", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}}, "required": []string{"id"}}, Handler: func(_ context.Context, a map[string]any) (string, error) {
		if err := filterStore.DeleteCustomRule(argString(a, "id", "")); err != nil {
			return "", err
		}
		return mcpJSON(map[string]any{"deleted": argString(a, "id", "")}), nil
	}})
	reg(mcp.Tool{Name: "filter_rule_groups_query", Description: "查询全部用户规则组（每个组含 name/rule_string/media_type/category）。", InputSchema: noArgs, Handler: func(_ context.Context, _ map[string]any) (string, error) {
		gs, err := filterStore.GetGroups()
		return mcpJSON(gs), mcpErr(err)
	}})
	reg(mcp.Tool{Name: "filter_rule_group_create", Description: "新建一个规则组。rule_string 为规则 id 的布尔表达式（! & | ()），'>' 分优先级层（如 4K&H265>1080P&H265>H264）；media_type 取 电影/电视剧/空（全局）；引用的规则 id 必须已存在（内置或自定义）。", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}, "rule_string": map[string]any{"type": "string", "description": "如 4K&H265>1080P&H265>H264"}, "media_type": map[string]any{"type": "string", "description": "电影 | 电视剧 | 空（全局）"}, "category": map[string]any{"type": "string"}}, "required": []string{"name", "rule_string"}}, Handler: func(_ context.Context, a map[string]any) (string, error) {
		g, err := filterStore.AddGroup(filter.RuleGroupInput{
			Name:       argString(a, "name", ""),
			RuleString: argString(a, "rule_string", ""),
			MediaType:  argString(a, "media_type", ""),
			Category:   argString(a, "category", ""),
		})
		if err != nil {
			return "", err
		}
		return mcpJSON(g), nil
	}})
	reg(mcp.Tool{Name: "filter_rule_group_update", Description: "按 old_name 定位并整体替换一个规则组（允许改 name；改名时旧名从全局绑定级联移除）。其余字段语义同 create。", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"old_name": map[string]any{"type": "string", "description": "待更新的现有规则组名"}, "name": map[string]any{"type": "string"}, "rule_string": map[string]any{"type": "string"}, "media_type": map[string]any{"type": "string"}, "category": map[string]any{"type": "string"}}, "required": []string{"old_name", "name", "rule_string"}}, Handler: func(_ context.Context, a map[string]any) (string, error) {
		g, err := filterStore.UpdateGroup(argString(a, "old_name", ""), filter.RuleGroupInput{
			Name:       argString(a, "name", ""),
			RuleString: argString(a, "rule_string", ""),
			MediaType:  argString(a, "media_type", ""),
			Category:   argString(a, "category", ""),
		})
		if err != nil {
			return "", err
		}
		return mcpJSON(g), nil
	}})
	reg(mcp.Tool{Name: "filter_rule_group_delete", Description: "按 name 删除一个规则组，并清理搜索/订阅/洗版全局绑定中的悬空引用。", Risk: mcp.RiskDestructive, RiskNote: "删除规则组并级联清理全局绑定引用", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}, "required": []string{"name"}}, Handler: func(_ context.Context, a map[string]any) (string, error) {
		if err := filterStore.DeleteGroup(argString(a, "name", "")); err != nil {
			return "", err
		}
		return mcpJSON(map[string]any{"deleted": argString(a, "name", "")}), nil
	}})

	// —— 扩展域：与 plugin Host API 对等的 chain 能力（agent_tools_ext.go）——
	tools = append(tools, buildDownloaderAdvTools(dlCh)...)
	tools = append(tools, buildMediaServerTools(mediaServerCh)...)
	tools = append(tools, buildSubscribeAdvTools(subCh)...)
	tools = append(tools, buildTransferAdvTools(transfer)...)
	tools = append(tools, buildReseedAdvTools(keepseed)...)
	tools = append(tools, buildCloudTools(cloudCh)...)
	tools = append(tools, buildBrushFlowTools(brushCh)...)
	tools = append(tools, buildRssTools(rssCh)...)
	tools = append(tools, buildSubtitleTools(subtitleCh)...)
	tools = append(tools, buildFileTools(fileCh)...)
	tools = append(tools, buildWorkflowTools(wfCh)...)

	classifyToolRisks(tools) // 给未显式标注的工具补 read/write，供白名单 UI 分组
	return tools
}

// classifyToolRisks 给未显式标注 Risk 的工具补 read/write 等级（destructive 已在各字面量
// 显式标注含 RiskNote，此处不覆盖）。依据 destructive 工具普查结果：下列为只读查询，其余默认 write。
// 仅用于白名单 UI 分组展示与未来细粒度授权；不影响确认闸门（闸门只看 RiskDestructive）。
func classifyToolRisks(tools []mcp.Tool) {
	reads := map[string]bool{
		"system.info": true, "sites.list": true, "site_messages": true, "site_freeleech": true,
		"media_search": true, "media_detail": true, "recommend_feeds": true, "torrent_search": true,
		"subscribe_list": true, "downloaders.list": true, "torrents_list": true, "torrents_stats": true,
		"transfer_history": true, "reseed_list": true, "dashboard_overview": true, "dashboard_trend": true,
		"filter_builtin_rules_query": true, "filter_custom_rules_query": true, "filter_rule_groups_query": true,
		"torrents_files": true, "torrents_peers": true, "torrents_trackers": true,
		"downloader_categories": true, "downloader_tags": true,
		"mediaserver_list": true, "mediaserver_libraries": true, "mediaserver_playback_report": true, "mediaserver_duplicates": true,
		"subscribe_get": true, "reseed_find_candidates": true, "reseed_find_missing": true,
		"cloud_list": true, "cloud_browse": true, "cloud_quota": true, "cloud_rules": true,
		"brushflow_tasks_list": true, "brushflow_task_get": true, "brushflow_seeds": true,
		"rss_list": true, "rss_items": true, "subtitle_search": true, "file_list": true,
		"workflow_list": true, "workflow_catalog": true,
	}
	for i := range tools {
		if tools[i].Risk != "" {
			continue // 显式标注（destructive）优先
		}
		if reads[tools[i].Name] {
			tools[i].Risk = mcp.RiskRead
		} else {
			tools[i].Risk = mcp.RiskWrite
		}
	}
}
