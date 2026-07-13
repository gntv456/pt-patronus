package workflow

// Template 是预置工作流模板（GET /workflows/templates 返回）。前端「从模板创建」加载到编辑器。
// Nodes/Edges 为完整图，前端据此填入画布并提交 /workflows 创建。
type Template struct {
	Key             string           `json:"key"`
	Name            string           `json:"name"`
	Description     string           `json:"description"`
	Category        string           `json:"category"`
	TriggerType     string           `json:"trigger_type"`
	Timer           string           `json:"timer"`
	EventType       string           `json:"event_type,omitempty"`
	EventConditions []EventCondition `json:"event_conditions,omitempty"`
	Nodes           []GraphNode      `json:"nodes"`
	Edges           []GraphEdge      `json:"edges"`
}

// Templates 内置工作流模板。节点 type 必须命中 Registry（见 nodes 包）。
func Templates() []Template {
	return []Template{
		{
			Key: "auto_chase", Name: "自动追剧", Category: "媒体",
			Description: "定时搜索剧集 → 过滤 1080p → 有资源则下载，否则通知「暂无」",
			TriggerType: "timer", Timer: "*/30 * * * *",
			Nodes: []GraphNode{
				{ID: "n1", Type: "search_torrents", Name: "搜索剧集", Params: map[string]any{"keyword": "请填剧名", "category": "tv"}, Position: Position{X: 80, Y: 40}},
				{ID: "n2", Type: "filter_torrents", Name: "过滤 1080p", Params: map[string]any{"resolution": "1080p"}, Position: Position{X: 80, Y: 180}},
				{ID: "n3", Type: "condition", Name: "有无资源", Params: map[string]any{"left": "torrents", "operator": "gt", "value": "0"}, Position: Position{X: 80, Y: 320}},
				{ID: "n4", Type: "add_download", Name: "添加下载", Params: map[string]any{"category": "workflow"}, Position: Position{X: 0, Y: 460}},
				{ID: "n5", Type: "send_message", Name: "通知暂无", Params: map[string]any{"title": "暂无资源", "body": "{{count}} 条", "level": "warning"}, Position: Position{X: 260, Y: 460}},
			},
			Edges: []GraphEdge{
				{ID: "e1", Source: "n1", Target: "n2"},
				{ID: "e2", Source: "n2", Target: "n3"},
				{ID: "e3", Source: "n3", Target: "n4", SourcePort: "true"},
				{ID: "e4", Source: "n3", Target: "n5", SourcePort: "false"},
			},
		},
		{
			Key: "rss_auto_dl", Name: "RSS 自动下载", Category: "RSS",
			Description: "定时抓取 RSS → 过滤 → 命中则下载并通知",
			TriggerType: "timer", Timer: "*/15 * * * *",
			Nodes: []GraphNode{
				{ID: "n1", Type: "fetch_rss", Name: "RSS 获取", Params: map[string]any{"url": "请填 RSS 地址"}, Position: Position{X: 80, Y: 40}},
				{ID: "n2", Type: "filter_torrents", Name: "过滤", Params: map[string]any{"include": "", "exclude": ""}, Position: Position{X: 80, Y: 180}},
				{ID: "n3", Type: "condition", Name: "有无命中", Params: map[string]any{"left": "torrents", "operator": "gt", "value": "0"}, Position: Position{X: 80, Y: 320}},
				{ID: "n4", Type: "add_download", Name: "添加下载", Params: map[string]any{}, Position: Position{X: 0, Y: 460}},
				{ID: "n5", Type: "send_message", Name: "通知命中", Params: map[string]any{"title": "RSS 命中", "body": "{{count}} 条", "level": "success"}, Position: Position{X: 260, Y: 460}},
			},
			Edges: []GraphEdge{
				{ID: "e1", Source: "n1", Target: "n2"},
				{ID: "e2", Source: "n2", Target: "n3"},
				{ID: "e3", Source: "n3", Target: "n4", SourcePort: "true"},
				{ID: "e4", Source: "n3", Target: "n5", SourcePort: "false"},
			},
		},
		{
			Key: "daily_checkin", Name: "每日签到", Category: "站点",
			Description: "每天定时对站点签到（可复制节点签到多个站）",
			TriggerType: "timer", Timer: "0 8 * * *",
			Nodes: []GraphNode{
				{ID: "n1", Type: "checkin", Name: "签到", Params: map[string]any{"site_id": float64(0)}, Position: Position{X: 80, Y: 40}},
			},
			Edges: []GraphEdge{},
		},
		{
			Key: "scheduled_reseed", Name: "定时辅种", Category: "站点",
			Description: "每天定时对下载器执行 IYUU 辅种",
			TriggerType: "timer", Timer: "0 3 * * *",
			Nodes: []GraphNode{
				{ID: "n1", Type: "reseed", Name: "辅种", Params: map[string]any{"downloader_id": float64(0)}, Position: Position{X: 80, Y: 40}},
			},
			Edges: []GraphEdge{},
		},
		{
			Key: "scheduled_ssh_command", Name: "定时 SSH 命令", Category: "运维",
			Description: "定时在指定 SSH 主机上执行巡检、清理、备份等命令，输出可继续通知或分支判断",
			TriggerType: "timer", Timer: "0 3 * * *",
			Nodes: []GraphNode{
				{ID: "n1", Type: "ssh_command", Name: "执行命令", Params: map[string]any{"target_id": float64(0), "command": "uptime && df -h", "timeout_sec": float64(30), "output_var": "ssh_output"}, Position: Position{X: 80, Y: 40}},
				{ID: "n2", Type: "send_message", Name: "发送结果", Params: map[string]any{"title": "SSH 巡检结果", "body": "{{var.ssh_output}}", "level": "info"}, Position: Position{X: 80, Y: 180}},
			},
			Edges: []GraphEdge{{ID: "e1", Source: "n1", Target: "n2"}},
		},
		{
			Key: "site_stats_notify", Name: "站点数据播报", Category: "站点",
			Description: "读取站点统计数据 → 发送通知播报",
			TriggerType: "timer", Timer: "0 9 * * *",
			Nodes: []GraphNode{
				{ID: "n1", Type: "fetch_site_stats", Name: "站点数据", Params: map[string]any{"site_id": float64(0)}, Position: Position{X: 80, Y: 40}},
				{ID: "n2", Type: "send_message", Name: "播报", Params: map[string]any{"title": "站点播报", "body": "上传 {{var.uploaded}} / 分享率 {{var.ratio}}", "level": "info"}, Position: Position{X: 80, Y: 180}},
			},
			Edges: []GraphEdge{{ID: "e1", Source: "n1", Target: "n2"}},
		},
		{
			Key: "cross_site_best", Name: "跨站择优下载", Category: "资源",
			Description: "跨所有站搜索 → 择优（免费/高分辨率/多做种）→ 命中则下载，否则通知",
			TriggerType: "timer", Timer: "0 */2 * * *",
			Nodes: []GraphNode{
				{ID: "n1", Type: "search_all_sites", Name: "跨站搜索", Params: map[string]any{"keyword": "请填片名", "category": ""}, Position: Position{X: 80, Y: 40}},
				{ID: "n2", Type: "pick_best", Name: "择优", Params: map[string]any{"count": float64(1)}, Position: Position{X: 80, Y: 180}},
				{ID: "n3", Type: "condition", Name: "有无资源", Params: map[string]any{"left": "torrents", "operator": "gt", "value": "0"}, Position: Position{X: 80, Y: 320}},
				{ID: "n4", Type: "add_download", Name: "添加下载", Params: map[string]any{"category": "workflow"}, Position: Position{X: 0, Y: 460}},
				{ID: "n5", Type: "send_message", Name: "通知暂无", Params: map[string]any{"title": "暂无资源", "body": "跨站未命中", "level": "warning"}, Position: Position{X: 260, Y: 460}},
			},
			Edges: []GraphEdge{
				{ID: "e1", Source: "n1", Target: "n2"},
				{ID: "e2", Source: "n2", Target: "n3"},
				{ID: "e3", Source: "n3", Target: "n4", SourcePort: "true"},
				{ID: "e4", Source: "n3", Target: "n5", SourcePort: "false"},
			},
		},
		{
			Key: "hr_risk_monitor", Name: "H&R 风险监控", Category: "站点",
			Description: "定时检查 H&R 任务，有风险时发送多通道告警", TriggerType: "timer", Timer: "0 */6 * * *",
			Nodes: []GraphNode{
				{ID: "n1", Type: "check_hr", Name: "检查 H&R", Params: map[string]any{"site_id": float64(0)}, Position: Position{X: 80, Y: 40}},
				{ID: "n2", Type: "condition", Name: "存在 H&R", Params: map[string]any{"left": "variable", "variable": "hr_count", "operator": "gt", "value": "0"}, Position: Position{X: 80, Y: 180}},
				{ID: "n3", Type: "send_notification", Name: "H&R 告警", Params: map[string]any{"title": "H&R 告警", "body": "当前有 {{var.hr_count}} 个 H&R 任务待处理", "level": "danger"}, Position: Position{X: 80, Y: 320}},
			}, Edges: []GraphEdge{{ID: "e1", Source: "n1", Target: "n2"}, {ID: "e2", Source: "n2", Target: "n3", SourcePort: "true"}},
		},
		{
			Key: "stalled_torrent_recovery", Name: "下载停滞自愈", Category: "下载", Description: "定时检查停滞任务，自动重报 Tracker 并重新校验",
			TriggerType: "timer", Timer: "*/20 * * * *",
			Nodes: []GraphNode{
				{ID: "n1", Type: "list_torrents", Name: "列出停滞任务", Params: map[string]any{"state": "stalled"}, Position: Position{X: 80, Y: 40}},
				{ID: "n2", Type: "condition", Name: "是否有停滞", Params: map[string]any{"left": "variable", "variable": "torrent_count", "operator": "gt", "value": "0"}, Position: Position{X: 80, Y: 180}},
				{ID: "n3", Type: "reannounce_torrents", Name: "重报 Tracker", Params: map[string]any{"hashes": ""}, Position: Position{X: 0, Y: 320}},
				{ID: "n4", Type: "recheck_torrents", Name: "重新校验", Params: map[string]any{"hashes": ""}, Position: Position{X: 220, Y: 320}},
				{ID: "n5", Type: "send_notification", Name: "自愈已触发", Params: map[string]any{"title": "自愈已触发", "body": "已处理 {{var.torrent_count}} 个停滞任务", "level": "warning"}, Position: Position{X: 110, Y: 460}},
			}, Edges: []GraphEdge{{ID: "e1", Source: "n1", Target: "n2"}, {ID: "e2", Source: "n2", Target: "n3", SourcePort: "true"}, {ID: "e3", Source: "n2", Target: "n4", SourcePort: "true"}, {ID: "e4", Source: "n3", Target: "n5"}, {ID: "e5", Source: "n4", Target: "n5"}},
		},
		{
			Key: "daily_media_digest", Name: "每日影视推荐", Category: "媒体", Description: "每日拉取热门影视推荐，并推送多通道摘要",
			TriggerType: "timer", Timer: "0 10 * * *",
			Nodes: []GraphNode{{ID: "n1", Type: "recommend_feed", Name: "获取热门推荐", Params: map[string]any{"page": float64(1)}, Position: Position{X: 80, Y: 40}}, {ID: "n2", Type: "send_notification", Name: "影视推荐摘要", Params: map[string]any{"title": "影视推荐摘要", "body": "今日推荐：{{content}}", "level": "info"}, Position: Position{X: 80, Y: 180}}},
			Edges: []GraphEdge{{ID: "e1", Source: "n1", Target: "n2"}},
		},
		{
			Key: "api_health_watch", Name: "API 健康巡检", Category: "运维", Description: "定时请求外部健康接口并推送响应",
			TriggerType: "timer", Timer: "*/10 * * * *",
			Nodes: []GraphNode{{ID: "n1", Type: "http_request", Name: "请求健康接口", Params: map[string]any{"method": "GET", "url": "https://example.com/health"}, Position: Position{X: 80, Y: 40}}, {ID: "n2", Type: "send_notification", Name: "API 巡检结果", Params: map[string]any{"title": "API 巡检结果", "body": "接口响应：{{content}}", "level": "warning"}, Position: Position{X: 80, Y: 180}}},
			Edges: []GraphEdge{{ID: "e1", Source: "n1", Target: "n2"}},
		},
		{
			Key: "webhook_event_notice", Name: "Webhook 事件通知", Category: "事件", Description: "收到入站 Webhook 事件时转换为多通道通知",
			TriggerType: "event", EventType: "webhook",
			Nodes: []GraphNode{{ID: "n1", Type: "send_notification", Name: "Webhook 事件", Params: map[string]any{"title": "Webhook 事件", "body": "{{var.title}} {{var.message}}", "level": "info"}, Position: Position{X: 80, Y: 40}}}, Edges: []GraphEdge{},
		},
		{
			Key: "auto_categorize", Name: "完成自动分类", Category: "下载",
			Description: "列出已完成任务 → 批量设置分类（哈希自动承接上一步）→ 多通道通知",
			TriggerType: "timer", Timer: "*/30 * * * *",
			Nodes: []GraphNode{
				{ID: "n1", Type: "list_torrents", Name: "列出已完成", Params: map[string]any{"state": "completed"}, Position: Position{X: 80, Y: 40}},
				{ID: "n2", Type: "set_category", Name: "设置分类", Params: map[string]any{"hashes": "", "category": "已完成"}, Position: Position{X: 80, Y: 180}},
				{ID: "n3", Type: "send_notification", Name: "通知", Params: map[string]any{"title": "自动分类", "body": "已处理 {{var.torrent_count}} 个任务", "level": "info"}, Position: Position{X: 80, Y: 320}},
			},
			Edges: []GraphEdge{
				{ID: "e1", Source: "n1", Target: "n2"},
				{ID: "e2", Source: "n2", Target: "n3"},
			},
		},
		{
			Key: "disk_full_alert", Name: "磁盘满告警", Category: "运维",
			Description: "定时 SSH 查看磁盘使用率，超阈值时多通道告警（需先在「服务器」页配置 SSH 主机并填 target_id）",
			TriggerType: "timer", Timer: "*/15 * * * *",
			Nodes: []GraphNode{
				{ID: "n1", Type: "ssh_command", Name: "查看磁盘", Params: map[string]any{"target_id": float64(0), "command": "df -h /", "timeout_sec": float64(30)}, Position: Position{X: 80, Y: 40}},
				{ID: "n2", Type: "extract_fields", Name: "解析使用率", Params: map[string]any{"pattern": "(?P<use>\\d+)%"}, Position: Position{X: 80, Y: 180}},
				{ID: "n3", Type: "condition", Name: "是否超 90%", Params: map[string]any{"left": "variable", "variable": "use", "operator": "gt", "value": "90"}, Position: Position{X: 80, Y: 320}},
				{ID: "n4", Type: "send_notification", Name: "磁盘告警", Params: map[string]any{"title": "磁盘使用率告警", "body": "磁盘使用率已达 {{var.use}}%，请及时清理", "level": "danger"}, Position: Position{X: 80, Y: 460}},
			},
			Edges: []GraphEdge{
				{ID: "e1", Source: "n1", Target: "n2"},
				{ID: "e2", Source: "n2", Target: "n3"},
				{ID: "e3", Source: "n3", Target: "n4", SourcePort: "true"},
			},
		},
		{
			Key: "seeding_ratio_manage", Name: "做种分享率管理", Category: "下载",
			Description: "定时列出做种任务，批量设置分享率上限（哈希自动承接上一步），避免长期高分享率浪费带宽",
			TriggerType: "timer", Timer: "0 3 * * *",
			Nodes: []GraphNode{
				{ID: "n1", Type: "list_torrents", Name: "列出做种", Params: map[string]any{"state": "seeding"}, Position: Position{X: 80, Y: 40}},
				{ID: "n2", Type: "condition", Name: "是否有做种", Params: map[string]any{"left": "variable", "variable": "torrent_count", "operator": "gt", "value": "0"}, Position: Position{X: 80, Y: 180}},
				{ID: "n3", Type: "set_share_limit", Name: "设置分享率上限", Params: map[string]any{"hashes": "", "ratio_limit": "2", "seeding_time_limit": float64(-1)}, Position: Position{X: 80, Y: 320}},
				{ID: "n4", Type: "send_notification", Name: "通知", Params: map[string]any{"title": "分享率管理", "body": "已为 {{var.torrent_count}} 个做种任务设置分享率上限", "level": "info"}, Position: Position{X: 80, Y: 460}},
			},
			Edges: []GraphEdge{
				{ID: "e1", Source: "n1", Target: "n2"},
				{ID: "e2", Source: "n2", Target: "n3", SourcePort: "true"},
				{ID: "e3", Source: "n3", Target: "n4"},
			},
		},
	}
}
