package api

import (
	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/middleware"
	"github.com/hxpt/ptpatronus/internal/api/v1"
)

// registerSystemRoutes 注册仪表盘、日志/诊断、注册、订阅、工作流、插件、刷流与保种契约路由。
// 插件静态资源（/plugins/:id/assets/*path）注册在 api 上以绕过 guarded 鉴权（插件前端无法带 JWT）。
func registerSystemRoutes(api, guarded *gin.RouterGroup, d *Deps, sysH *v1.SystemHandler) {
	dashH := v1.NewDashboardHandler(d.Dashboard)
	guarded.GET("/dashboard", dashH.Overview)
	guarded.POST("/dashboard/sync", dashH.Sync)
	guarded.POST("/dashboard/sync/:id", dashH.SyncOne)
	guarded.GET("/dashboard/history/:id", dashH.History)
	guarded.GET("/dashboard/trend", dashH.Trend)
	// 工作台聚合（对标 MSaber 天序工作台）：今日流转/待办/动态/摘要 + 媒体库态势 + 体检分
	guarded.GET("/dashboard/workbench", v1.NewDashboardHandler2(d.Dashboard, d.MediaServer).Workbench)

	// 运行日志（环形缓冲；级别筛选，排查用）
	guarded.GET("/system/logs", middleware.RequireAdmin(), sysH.Logs)
	guarded.DELETE("/system/logs", middleware.RequireAdmin(), sysH.ClearLogs)
	// 本机系统资源快照（CPU/内存/磁盘/负载；仪表盘用，需管理员）
	guarded.GET("/system/metrics", middleware.RequireAdmin(), sysH.Metrics)

	// 诊断面板（safe-mode 下仍注册：自救用途）。
	if d.Doctor != nil {
		guarded.GET("/system/doctor", middleware.RequireAdmin(), d.Doctor.Run)
		guarded.POST("/system/doctor/fix", middleware.RequireAdmin(), d.Doctor.Fix)
	}

	// OCR 服务配置（登录验证码自动识别；未配置=人工输入）。
	if d.CfgOp != nil {
		ocrH := v1.NewOCRHandler(d.CfgOp)
		guarded.GET("/system/ocr", ocrH.GetConfig)
		guarded.PUT("/system/ocr", middleware.RequireAdmin(), ocrH.SetConfig)
		guarded.POST("/system/ocr/test", middleware.RequireAdmin(), ocrH.Test)
	}

	regH := v1.NewRegistrationHandler(d.RegOp, d.RegCh)
	regs := guarded.Group("/registrations")
	{
		regs.GET("", regH.List)
		regs.POST("", regH.Create)
		regs.PUT("/:id", regH.Update)
		regs.DELETE("/:id", regH.Delete)
		regs.POST("/:id/check", regH.Check)
	}

	subH := v1.NewSubscribeHandler(d.Subscribe)
	subs := guarded.Group("/subscriptions")
	{
		subs.GET("", subH.List)
		subs.POST("", subH.Create)
		subs.PUT("/:id", subH.Update)
		subs.DELETE("/:id", subH.Delete)
		subs.POST("/:id/check", subH.Check)
		subs.POST("/:id/reset", subH.ResetFields) // 清空指定字段回退全局默认
		// 批量操作：update（patch 字段级编辑）/ check（重新搜索）/ delete（取消订阅）。
		// 静态段 batch 须置于 /:id 参数路由之前注册顺序无关（Gin radix 自动区分静态与参数）。
		subs.POST("/batch", subH.Batch)
	}
	// 全局订阅默认设置：独立前缀避 /subscriptions/:id 参数位冲突（Gin radix 静态+参数同层会 panic）。
	// 未单独设置的订阅在 Check 时回退到此。
	guarded.GET("/subscribe/settings", subH.GetGlobalSettings)
	guarded.PUT("/subscribe/settings", subH.UpdateGlobalSettings)

	// 工作流：可视化节点图引擎。静态路由 /actions、/runs 置于 /:id 前避免冲突。
	wfH := v1.NewWorkflowHandler(d.Workflow)
	wfs := guarded.Group("/workflows")
	{
		wfs.GET("", wfH.List)
		wfs.POST("", wfH.Create)
		wfs.GET("/actions", wfH.Catalog)
		wfs.GET("/templates", wfH.Templates)
		wfs.GET("/stats", wfH.Stats)
		wfs.GET("/:id", wfH.Get)
		wfs.PUT("/:id", wfH.Update)
		wfs.DELETE("/:id", wfH.Delete)
		wfs.POST("/:id/run", wfH.Run)
		wfs.POST("/:id/enable", wfH.Enable)
		wfs.POST("/:id/disable", wfH.Disable)
		wfs.POST("/:id/reset", wfH.Reset)
	}
	wfRuns := guarded.Group("/workflow-runs")
	{
		wfRuns.GET("", wfH.ListRuns)
		wfRuns.GET("/:rid", wfH.RunDetail)
	}

	pluginH := v1.NewPluginHandler(d.Plugins, d.Tickets)
	api.GET("/plugins/:id/assets/*path", pluginH.Asset)
	plugins := guarded.Group("/plugins")
	{
		plugins.GET("", pluginH.List)
		plugins.GET("/logs", pluginH.Logs)
		plugins.POST("/events", middleware.RequireAdmin(), pluginH.PublishEvent)
		plugins.GET("/market", middleware.RequireAdmin(), pluginH.Market)
		plugins.GET("/market-sources", middleware.RequireAdmin(), pluginH.MarketSources)
		plugins.PUT("/market-sources", middleware.RequireAdmin(), pluginH.SetMarketSources)
		plugins.GET("/trust-config", middleware.RequireAdmin(), pluginH.TrustConfig)
		plugins.PUT("/trust-config", middleware.RequireAdmin(), pluginH.SetTrustConfig)
		plugins.POST("/market/install", middleware.RequireAdmin(), pluginH.Install)
		// 市场运营层（对标 MSaber）：风云榜 / 梦萦阁收藏 / 分神实例 / 天序排序。
		// 静态段 charts/favorites 置于 /:id 参数路由之前（Gin radix 区分静态与参数，顺序无碍但集中放置可读）。
		plugins.GET("/market/charts", pluginH.MarketCharts)
		plugins.GET("/market/favorites", pluginH.MarketFavorites)
		plugins.POST("/market/favorites/:id", middleware.RequireAdmin(), pluginH.FavoritePlugin)
		plugins.DELETE("/market/favorites/:id", middleware.RequireAdmin(), pluginH.UnfavoritePlugin)
		plugins.GET("/:id/instances", pluginH.PluginInstances)
		plugins.POST("/:id/instances", middleware.RequireAdmin(), pluginH.CreatePluginInstance)
		plugins.PUT("/:id/instances/reorder", middleware.RequireAdmin(), pluginH.ReorderPluginInstances)
		plugins.PUT("/:id/instances/:key", middleware.RequireAdmin(), pluginH.SavePluginInstanceConfig)
		plugins.DELETE("/:id/instances/:key", middleware.RequireAdmin(), pluginH.DeletePluginInstance)
		plugins.POST("/:id/assets/ticket", middleware.RequireAdmin(), pluginH.AssetTicket)
		plugins.GET("/:id/releases", pluginH.Releases)
		plugins.POST("/:id/bridge", middleware.RequireAdmin(), pluginH.Bridge)
		plugins.POST("/:id/rollback", middleware.RequireAdmin(), pluginH.Rollback)
		plugins.GET("/:id", pluginH.Get)
		plugins.PUT("/:id/config", middleware.RequireAdmin(), pluginH.SaveConfig)
		plugins.POST("/:id/enable", middleware.RequireAdmin(), pluginH.Enable)
		plugins.POST("/:id/disable", middleware.RequireAdmin(), pluginH.Disable)
		plugins.POST("/:id/action", middleware.RequireAdmin(), pluginH.Action)
		plugins.POST("/:id/reload", middleware.RequireAdmin(), pluginH.Reload)
		plugins.DELETE("/:id", middleware.RequireAdmin(), pluginH.Uninstall)
	}

	// 刷流引擎：任务 CRUD + 手动选种/扫删 + 种子加入历史
	bfH := v1.NewBrushFlowHandler(d.BrushFlow)
	bf := guarded.Group("/brushflow")
	{
		bf.GET("", bfH.List)
		bf.POST("", bfH.Create)
		bf.GET("/:id", bfH.Get)
		bf.PUT("/:id", bfH.Update)
		bf.DELETE("/:id", middleware.RequireAdmin(), bfH.Delete)
		bf.POST("/:id/select", bfH.Select)
		bf.POST("/:id/sweep", bfH.Sweep)
		bf.GET("/:id/seeds", bfH.Seeds)
	}

	// 保种契约：规则 CRUD + 现场检查
	ctH := v1.NewContractHandler(d.Contract)
	ct := guarded.Group("/contract")
	{
		ct.GET("/rules", ctH.List)
		ct.POST("/rules", ctH.Upsert)
		ct.DELETE("/rules/:id", middleware.RequireAdmin(), ctH.Delete)
		ct.POST("/rules/:id/check", ctH.Check)
	}
}
