// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package api

import (
	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/middleware"
	"github.com/hxpt/ptpatronus/internal/api/v1"
)

// registerDownloaderRoutes 注册下载器、调度配置与通知渠道路由。
func registerDownloaderRoutes(guarded *gin.RouterGroup, d *Deps, schedH *v1.ScheduleHandler) {
	dlH := v1.NewDownloaderHandler(d.DLOp, d.DLCh, d.DLMgr, d.SiteOp)
	dls := guarded.Group("/downloaders")
	{
		dls.GET("", dlH.List)
		dls.GET("/caps", dlH.Caps) // 各类型能力位矩阵（静态，前端门控 UI 用；替代散落的 type 判断）
		dls.GET("/:id", dlH.Get)
		dls.POST("", dlH.Create)
		dls.PUT("/:id", dlH.Update)
		dls.DELETE("/:id", middleware.RequireAdmin(), dlH.Delete)
		dls.POST("/:id/test", dlH.Test)
		dls.GET("/:id/torrents", dlH.Torrents)
		dls.POST("/:id/torrents", dlH.Add)
		dls.POST("/:id/pause", dlH.Pause)
		dls.POST("/:id/resume", dlH.Resume)
		dls.POST("/:id/remove", dlH.Remove)
		dls.GET("/:id/stats", dlH.Stats)

		// 进阶操作（P0+P1）
		dls.POST("/:id/torrents/upload", dlH.UploadTorrent)   // 上传 .torrent 文件
		dls.GET("/:id/categories", dlH.Categories)            // 列分类
		dls.GET("/:id/tags", dlH.Tags)                        // 列标签
		dls.POST("/:id/torrents/category", dlH.SetCategoryOp) // 设置分类
		dls.POST("/:id/torrents/tags", dlH.TagsOp)            // 标签 set/add/remove
		dls.POST("/:id/torrents/recheck", dlH.Recheck)        // 重新校验
		dls.POST("/:id/torrents/reannounce", dlH.Reannounce)  // 重新汇报
		dls.POST("/:id/torrents/limits", dlH.SetLimits)       // 单种子限速
		dls.POST("/:id/torrents/location", dlH.SetLocation)   // 移动保存路径
		dls.GET("/:id/torrents/files", dlH.Files)             // 文件列表
		dls.POST("/:id/torrents/file-priority", dlH.SetFilePriority)
		dls.GET("/:id/torrents/trackers", dlH.Trackers) // tracker 列表
		dls.POST("/:id/torrents/trackers/add", dlH.AddTrackers)
		dls.POST("/:id/torrents/trackers/remove", dlH.RemoveTrackers)
		dls.GET("/:id/speed-limits", dlH.GetSpeedLimits) // 全局限速
		dls.POST("/:id/speed-limits", dlH.SetSpeedLimits)

		// P2：做种限制 / 队列 / 强制 / 顺序 / Peers / 分类增删
		dls.POST("/:id/torrents/share-limits", dlH.SetShareLimits)
		dls.POST("/:id/torrents/queue", dlH.SetQueuePriority)
		dls.POST("/:id/torrents/force-start", dlH.SetForceStart)
		dls.POST("/:id/torrents/sequential", dlH.ToggleSequential)
		dls.POST("/:id/torrents/first-last", dlH.ToggleFirstLast)
		dls.GET("/:id/torrents/peers", dlH.Peers)
		dls.POST("/:id/categories/create", dlH.CreateCategory)
		dls.POST("/:id/categories/edit", dlH.EditCategory)
		dls.POST("/:id/categories/delete", dlH.DeleteCategories)
	}

	notifH := v1.NewNotificationHandler(d.Notif, d.NotifMgr, d.NotifProvider)
	sched := guarded.Group("/schedule")
	{
		sched.GET("/config", schedH.GetConfig)
		sched.PUT("/config", middleware.RequireAdmin(), schedH.SetConfig)
	}

	notif := guarded.Group("/notification")
	{
		notif.GET("/channels", notifH.Channels)
		notif.POST("/test", middleware.RequireAdmin(), notifH.Test)
		notif.GET("/config", notifH.GetConfig)
		notif.PUT("/config", middleware.RequireAdmin(), notifH.SetConfig)
		notif.POST("/wechat/qrcode", middleware.RequireAdmin(), notifH.WeChatQRCode)
		notif.GET("/wechat/qrcode/status", middleware.RequireAdmin(), notifH.WeChatQRStatus)
	}
}
