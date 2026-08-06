package api

import (
	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/middleware"
	"github.com/hxpt/ptpatronus/internal/api/v1"
)

// registerMonitorRoutes 注册媒体服务器、网盘/STRM、字幕、Trakt、CookieCloud、RSS、媒体整理、
// 资源过滤、AI 助手、LLM/Telegram 配置、Webhook、监控/告警/Agent、PT监护室与全局通知路由。
func registerMonitorRoutes(guarded *gin.RouterGroup, d *Deps) {
	// 媒体服务器（Emby/JF/Plex）：CRUD + 测试 + 库扫描/刷新 + 封面 + 通知
	msH := v1.NewMediaServerHandler(d.MediaServer)
	ms := guarded.Group("/mediaserver")
	{
		ms.GET("", msH.List)
		ms.POST("", msH.Create)
		ms.PUT("/:id", msH.Update)
		ms.DELETE("/:id", middleware.RequireAdmin(), msH.Delete)
		ms.POST("/:id/test", msH.Test)
		ms.GET("/:id/libraries", msH.Libraries)
		ms.POST("/:id/scan", msH.Scan)
		ms.POST("/:id/refresh", msH.Refresh)
		ms.POST("/:id/notify", msH.Notify)
		ms.GET("/:id/cover", msH.Cover)
		ms.GET("/:id/playback-report", msH.PlaybackReport)
		ms.GET("/:id/duplicates", msH.Duplicates)
	}

	// 网盘 + STRM（115/夸克/123）：网盘 CRUD/探活/配额/浏览 + STRM 规则 + 生成 + 订阅追更
	// 子分组 /drives /rules /records 避免 :id 与同级静态段冲突。
	cloudH := v1.NewCloudHandler(d.Cloud)
	cl := guarded.Group("/cloud")
	{
		dr := cl.Group("/drives")
		{
			dr.GET("", cloudH.List)
			dr.POST("", cloudH.Create)
			dr.PUT("/:id", cloudH.Update)
			dr.DELETE("/:id", middleware.RequireAdmin(), cloudH.Delete)
			dr.POST("/:id/test", cloudH.Test)
			dr.GET("/:id/quota", cloudH.Quota)
			dr.GET("/:id/list", cloudH.Browse)
		}
		rl := cl.Group("/rules")
		{
			rl.GET("", cloudH.ListRules)
			rl.POST("", cloudH.CreateRule)
			rl.PUT("/:id", cloudH.UpdateRule)
			rl.DELETE("/:id", middleware.RequireAdmin(), cloudH.DeleteRule)
			rl.POST("/:id/generate", cloudH.GenerateRule)
		}
		rc := cl.Group("/records")
		{
			rc.GET("", cloudH.ListRecords)
			rc.DELETE("/:id", middleware.RequireAdmin(), cloudH.DeleteRecord)
		}
		cl.POST("/generate", cloudH.Generate)
		cl.POST("/generate-all", cloudH.GenerateAll)
		cl.POST("/sync-subscriptions", cloudH.SyncSubscriptions)
	}

	// 字幕：跨源搜索 + 下载 + 源配置（ASSRT/OpenSubtitles token、SubHD/Zimuku cookie、繁简）
	// + 字幕自动化配置（整理完成自动下载字幕）
	subtH := v1.NewSubtitleHandler(d.Subtitle, d.SubtitleAutomation)
	subt := guarded.Group("/subtitle")
	{
		subt.GET("/search", subtH.Search)
		subt.POST("/download", subtH.Download)
		subt.GET("/config", subtH.GetConfig)
		subt.PUT("/config", middleware.RequireAdmin(), subtH.SetConfig)
		subt.GET("/automation/config", subtH.GetAutomationConfig)
		subt.PUT("/automation/config", middleware.RequireAdmin(), subtH.SetAutomationConfig)
	}

	// Trakt 同步：Device OAuth 授权 + WatchList→订阅 + 同步状态/配置
	traktH := v1.NewTraktHandler(d.Trakt)
	trakt := guarded.Group("/trakt")
	{
		trakt.GET("/config", traktH.GetConfig)
		trakt.PUT("/config", middleware.RequireAdmin(), traktH.SetConfig)
		trakt.POST("/device-code", traktH.DeviceCode)
		trakt.GET("/device-status", traktH.DeviceStatus)
		trakt.POST("/sync", traktH.Sync)
		trakt.GET("/history", traktH.History)
	}

	// CookieCloud 同步：拉取浏览器扩展推送的最新 cookie，按域名覆盖本地失效站 + 自动新增匹配站。
	cookieCloudH := v1.NewCookieCloudHandler(d.CookieCloud)
	cookieCloud := guarded.Group("/cookiecloud")
	{
		cookieCloud.GET("/config", cookieCloudH.GetConfig)
		cookieCloud.PUT("/config", middleware.RequireAdmin(), cookieCloudH.SetConfig)
		cookieCloud.POST("/sync", middleware.RequireAdmin(), cookieCloudH.Sync)
		cookieCloud.POST("/push", middleware.RequireAdmin(), cookieCloudH.Push)
	}

	// RSS 订阅：feed 抓取→过滤→去重→(可选)下载→通知。
	rssH := v1.NewRssHandler(d.Rss)
	rss := guarded.Group("/rss")
	{
		rss.GET("", rssH.List)
		rss.POST("", rssH.Create)
		rss.PUT("/:id", rssH.Update)
		rss.DELETE("/:id", rssH.Delete)
		rss.POST("/:id/check", rssH.Check)
		rss.GET("/:id/items", rssH.ListItems)
	}

	// 媒体整理：下载完成→识别→重命名→硬链→刮削→入库历史/配置/手动整理/重做/扫描。
	trH := v1.NewTransferHandler(d.Transfer)
	tr := guarded.Group("/transfer")
	{
		tr.GET("/history", trH.History)
		tr.GET("/summary", trH.Summary)
		tr.DELETE("/history/:id", middleware.RequireAdmin(), trH.DeleteHistory)
		tr.POST("/history/batch-delete", middleware.RequireAdmin(), trH.BatchDeleteHistory)
		tr.POST("/history/clear-failed", middleware.RequireAdmin(), trH.ClearFailedHistory)
		tr.GET("/config", trH.GetConfig)
		tr.PUT("/config", middleware.RequireAdmin(), trH.SetConfig)
		tr.GET("/dirwatch", trH.GetDirwatchConfig)
		tr.PUT("/dirwatch", middleware.RequireAdmin(), trH.SetDirwatchConfig)
		tr.GET("/categories", trH.GetCategoriesConfig)
		tr.PUT("/categories", middleware.RequireAdmin(), trH.SetCategoriesConfig)
		tr.POST("/organize", middleware.RequireAdmin(), trH.Organize)
		tr.POST("/redo/:id", middleware.RequireAdmin(), trH.Redo)
		tr.POST("/sweep", middleware.RequireAdmin(), trH.Sweep)
	}

	// 资源过滤规则（对标 MP-2 filter module）：内置/自定义规则 + 规则组 + 全局绑定 + 规则测试。
	frH := v1.NewFilterRuleHandler(d.Filter, d.FilterEng)
	fr := guarded.Group("/filter")
	{
		fr.GET("/rules/builtin", frH.ListBuiltins)
		fr.GET("/rules/custom", frH.ListCustom)
		fr.POST("/rules/custom", middleware.RequireAdmin(), frH.CreateCustom)
		fr.PUT("/rules/custom/:id", middleware.RequireAdmin(), frH.UpdateCustom)
		fr.DELETE("/rules/custom/:id", middleware.RequireAdmin(), frH.DeleteCustom)
		fr.GET("/groups", frH.ListGroups)
		fr.POST("/groups", middleware.RequireAdmin(), frH.CreateGroup)
		fr.PUT("/groups/:name", middleware.RequireAdmin(), frH.UpdateGroup)
		fr.DELETE("/groups/:name", middleware.RequireAdmin(), frH.DeleteGroup)
		fr.GET("/bindings", frH.ListBindings)
		fr.PUT("/bindings", middleware.RequireAdmin(), frH.SetBindings)
		fr.POST("/test", frH.Test)
	}

	// AI 助手（admin-only）：SSE 对话 + 会话历史 + provider 配置。
	ag := guarded.Group("/agent")
	{
		ag.POST("/chat", middleware.RequireAdmin(), d.AgentChat.Chat)
		ag.GET("/conversations", middleware.RequireAdmin(), d.AgentChat.Conversations)
		ag.DELETE("/conversations/:id", middleware.RequireAdmin(), d.AgentChat.DeleteConversation)
		ag.GET("/conversations/:id/messages", middleware.RequireAdmin(), d.AgentChat.Messages)
		ag.POST("/conversations/:id/confirm", middleware.RequireAdmin(), d.AgentChat.Confirm)
		ag.GET("/tools", middleware.RequireAdmin(), d.AgentChat.Tools)
		ag.GET("/stats", middleware.RequireAdmin(), d.AgentChat.Stats)
		ag.GET("/config", d.AgentChat.GetConfig)
		ag.PUT("/config", middleware.RequireAdmin(), d.AgentChat.SetConfig)
	}

	// LLM 网关配置（管理员）。safe-mode 下 d.LLMGateway 为 nil 时不注册。
	if d.LLMGateway != nil {
		guarded.GET("/llmgateway/config", middleware.RequireAdmin(), d.LLMGateway.GetConfig)
		guarded.PUT("/llmgateway/config", middleware.RequireAdmin(), d.LLMGateway.SetConfig)
	}
	// Telegram bot 配置（管理员）。
	if d.TgBot != nil {
		guarded.GET("/tgbot/config", middleware.RequireAdmin(), d.TgBot.GetConfig)
		guarded.PUT("/tgbot/config", middleware.RequireAdmin(), d.TgBot.SetConfig)
	}

	// 进站 Webhook 管理（CRUD）；公开接收端点 /webhooks/in/:token 在下方公开区注册。
	whH := d.Webhook
	whs := guarded.Group("/webhooks")
	{
		whs.GET("", whH.List)
		whs.POST("", middleware.RequireAdmin(), whH.Create)
		whs.PUT("/:id", middleware.RequireAdmin(), whH.Update)
		whs.DELETE("/:id", middleware.RequireAdmin(), whH.Delete)
		whs.POST("/:id/rotate", middleware.RequireAdmin(), whH.Rotate)
	}

	monH := v1.NewMonitorHandler(d.MonOp, d.MonSvc, d.SSHHostKeyOp)
	mons := guarded.Group("/monitors")
	{
		mons.GET("", monH.List)
		mons.POST("", middleware.RequireAdmin(), monH.Create)
		mons.PUT("/:id", middleware.RequireAdmin(), monH.Update)
		mons.DELETE("/:id", middleware.RequireAdmin(), monH.Delete)
		mons.POST("/:id/probe", middleware.RequireAdmin(), monH.Probe)
		mons.POST("/:id/collect", middleware.RequireAdmin(), monH.Collect)
		mons.POST("/:id/exec", middleware.RequireAdmin(), monH.Exec)
		mons.POST("/:id/power", middleware.RequireAdmin(), monH.Power)
		mons.GET("/:id/metrics", monH.Metrics)
		mons.POST("/:id/accept-host-key", middleware.RequireAdmin(), monH.AcceptHostKey)
	}
	alerts := guarded.Group("/monitor/alerts")
	{
		alerts.GET("", monH.ListAlerts)
		alerts.POST("/:id/read", monH.ReadAlert)
		alerts.POST("/:id/ack", middleware.RequireAdmin(), monH.AckAlert)
		alerts.POST("/:id/silence", middleware.RequireAdmin(), monH.SilenceAlert)
	}

	// Agent 管理（push 架构）：创建 agent 返回明文 secret、列表、删除、重置 secret。
	agentH := v1.NewAgentHandler(d.AgentOp, d.MonOp, d.DB)
	agents := guarded.Group("/monitors/agents")
	{
		agents.GET("", agentH.List)
		agents.POST("", middleware.RequireAdmin(), agentH.Create)
		agents.DELETE("/:id", middleware.RequireAdmin(), agentH.Delete)
		agents.POST("/:id/rotate", middleware.RequireAdmin(), agentH.Rotate)
	}

	// PT监护室：聚合 savept.icu 站点健康数据
	saveptH := v1.NewSaveptHandler(d.Savept)
	guarded.GET("/savept", saveptH.Get)

	// 全局通知中心（站内收件箱）
	noticeH := v1.NewNoticeHandler(d.Notice)
	ntc := guarded.Group("/notices")
	{
		ntc.GET("", noticeH.List)
		ntc.POST("/read-all", noticeH.ReadAll)
		ntc.POST("/clear", noticeH.Clear)
		ntc.POST("/:id/read", noticeH.Read)
		ntc.DELETE("/:id", noticeH.Delete)
	}
}
