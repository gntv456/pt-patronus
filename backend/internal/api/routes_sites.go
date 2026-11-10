// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/api/middleware"
	"github.com/hxpt/ptpatronus/internal/api/v1"
	"github.com/hxpt/ptpatronus/internal/sitedefs"
)

// registerSiteRoutes 注册站点相关路由：站点 CRUD/搜索/签到 + 免费促销/公告/后宫/考核/勋章/私信聚合。
func registerSiteRoutes(guarded *gin.RouterGroup, d *Deps) {
	siteH := v1.NewSiteHandler(d.SiteOp, d.SiteCh, d.Reg, d.SiteHC)
	sites := guarded.Group("/sites")
	{
		sites.GET("", siteH.List)
		sites.GET("/definitions", siteH.Definitions)
		// 站点定义远端源手动同步（cron 每 6h 自动跑；此口给「加站不用等」）。
		// Syncer 由 app 装配期经 sitedefs.Wire 挂到包级 Default（避免 v1→app 环）；
		// nil（safe-mode/未装配）返回 503。
		sites.POST("/definitions/sync", middleware.RequireAdmin(),
			func(c *gin.Context) {
				syncer := sitedefs.Default()
				if syncer == nil {
					errcode.Respond(c, errcode.AuthServiceUnavailable)
					return
				}
				n, err := syncer.Sync(c.Request.Context())
				if err != nil {
					errcode.Respond(c, errcode.SaveFailed.WithErr(err))
					return
				}
				c.JSON(http.StatusOK, gin.H{"ok": true, "definitions": n})
			})
		// 站点定义诊断（四类「配置被静默忽略」）：界面要能读到，否则用户只看得到「抓不到东西」。
		sites.GET("/definition-diagnostics", siteH.DefinitionDiagnostics)
		sites.POST("/inspect", siteH.Inspect)
		sites.GET("/:id", siteH.Get)
		// 写操作（含改密钥）与 DELETE 同级收敛 admin：站点配置影响全局抓取行为。
		sites.POST("", middleware.RequireAdmin(), siteH.Create)
		sites.PUT("/:id", middleware.RequireAdmin(), siteH.Update)
		sites.POST("/:id/cookie", middleware.RequireAdmin(), siteH.UpdateCookie)
		sites.DELETE("/:id", middleware.RequireAdmin(), siteH.Delete)
		sites.POST("/batch-delete", middleware.RequireAdmin(), siteH.BatchDelete)
		sites.GET("/status", siteH.Status)
		sites.GET("/status/stream", siteH.StatusStream)
		sites.POST("/:id/test", siteH.Test)
		sites.POST("/:id/checkin", siteH.Checkin)
		sites.POST("/checkin", siteH.CheckinAll)
		sites.GET("/checkin/records", siteH.CheckinRecords)
		sites.POST("/:id/search", siteH.Search)
		sites.GET("/:id/torrent/detail", siteH.TorrentDetail)
		// 站点 Cookie 台账（浏览器扩展对比/同步用）：指纹全员可读（脱敏），
		// 明文（reveal=1，覆盖到浏览器侧用）与写入同级收敛 admin。
		sites.GET("/cookies", siteH.GetCookies)
		sites.GET("/cookies/reveal", middleware.RequireAdmin(), siteH.GetCookiesReveal)
	}
	guarded.POST("/search", siteH.SearchAll)
	guarded.GET("/search/stream", siteH.SearchAllStream)

	// 站点信息聚合：全站免费/促销 + 公告（套用站点 capability，per-site 缓存）
	flH := v1.NewFreeleechHandler(d.SiteCh, d.SiteOp)
	fl := guarded.Group("/freeleech")
	{
		fl.GET("", flH.All)
		fl.GET("/stream", flH.AllStream) // 流式：每站一行 NDJSON，前端逐站显示（加载一个显示一个）
		fl.POST("/sync/:id", flH.SyncOne)
	}
	annH := v1.NewAnnouncementHandler(d.SiteCh, d.SiteOp)
	ann := guarded.Group("/announcements")
	{
		ann.GET("", annH.All)
		ann.GET("/stream", annH.AllStream) // 流式：每站一行 NDJSON，前端逐站显示
		ann.POST("/sync/:id", annH.SyncOne)
	}

	// 站点求种请求（viewsrequests.php；NexusPHP 系，对标 MSaber「站点请求」）
	reqH := v1.NewSiteRequestHandler(d.SiteCh)
	guarded.GET("/site-requests", reqH.All)

	// 后宫：邀请库存 + 被邀请人花名册
	haremH := v1.NewHaremHandler(d.SiteCh, d.SiteOp)
	harem := guarded.Group("/harem")
	{
		harem.GET("", haremH.All)
		harem.GET("/stream", haremH.AllStream)
		harem.POST("/sync/:id", haremH.SyncOne)
		harem.GET("/export", haremH.Export)
	}

	// 站点考核：per-platform 适配（MTeam JSON / NexusPHP HTML / TNode 不支持）
	assessH := v1.NewAssessmentHandler(d.Assessment, d.SiteOp)
	assess := guarded.Group("/assessment")
	{
		assess.GET("", assessH.All)
		assess.GET("/stream", assessH.AllStream)
		assess.POST("/sync/:id", assessH.SyncOne)
	}

	// 勋章墙：跨站勋章聚合 + 购买/佩戴/摘下
	medalH := v1.NewMedalHandler(d.Medal, d.SiteOp)
	medal := guarded.Group("/medalwall")
	{
		medal.GET("", medalH.All)
		medal.GET("/stream", medalH.AllStream)
		medal.POST("/buy", medalH.Buy)
		medal.POST("/wear", medalH.Wear)
		medal.POST("/takeoff", medalH.TakeOff)
	}

	// 站点私信：列表 + 正文 + 回复/删除/标记已读
	msgH := v1.NewMessageHandler(d.SiteCh, d.SiteOp)
	msg := guarded.Group("/messages")
	{
		msg.GET("", msgH.All)
		msg.GET("/stream", msgH.Stream) // 流式：每站一行 NDJSON，前端逐站显示
		msg.POST("/sync/:id", msgH.SyncOne)
		msg.GET("/:id/content", msgH.Content)
		msg.POST("/:id/reply", msgH.Reply)
		msg.POST("/:id/delete", msgH.Delete)
		msg.POST("/:id/markread", msgH.MarkRead)
	}
}
