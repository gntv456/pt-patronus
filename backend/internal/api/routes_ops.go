package api

import (
	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/middleware"
	"github.com/hxpt/ptpatronus/internal/api/v1"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// registerOpsRoutes 注册文件存储、备份、凭据、运维配置与命令运行路由（均限管理员）。
func registerOpsRoutes(guarded *gin.RouterGroup, d *Deps) {
	fsH := v1.NewFileStorageHandler(d.FileOp, d.FileCh, d.FileMgr)
	storages := guarded.Group("/storages")
	storages.Use(middleware.RequireAdmin()) // 限管理员：local 后端可访问宿主磁盘
	{
		storages.GET("", fsH.List)
		storages.POST("", fsH.Create)
		storages.PUT("/:id", fsH.Update)
		storages.DELETE("/:id", fsH.Delete)
		storages.POST("/:id/test", fsH.Test)
		storages.GET("/:id/files", fsH.Files)
		storages.POST("/:id/upload", fsH.Upload)
		storages.GET("/:id/download", fsH.Download)
		storages.POST("/:id/mkdir", fsH.Mkdir)
		storages.POST("/:id/delete", fsH.DeleteFile)
		storages.POST("/:id/rename", fsH.Rename)
		storages.POST("/:id/move", fsH.Move)
		storages.POST("/:id/copy", fsH.Copy)
	}

	bkH := v1.NewBackupHandler(d.Backup, d.Notice)
	bk := guarded.Group("/backup")
	{
		bk.GET("/export", middleware.RequireAdmin(), bkH.Export)
		bk.POST("/import", middleware.RequireAdmin(), bkH.Import)
		bk.POST("/import-moviepilot", middleware.RequireAdmin(), bkH.ImportMoviePilot)
		bk.POST("/import-ptd", middleware.RequireAdmin(), bkH.ImportPTD)
	}

	credH := v1.NewCredentialHandler(oper.NewCredentialOper(d.DB, d.Crypto))
	creds := guarded.Group("/ops/credentials")
	creds.Use(middleware.RequireAdmin())
	{
		creds.GET("", credH.List)
		creds.POST("", credH.Create)
		creds.PUT("/:id", credH.Update)
		creds.DELETE("/:id", credH.Delete)
	}
	cfgH := v1.NewOpsConfigHandler(oper.NewOpsConfigOper(d.DB))
	opsCfg := guarded.Group("/ops/configs")
	opsCfg.Use(middleware.RequireAdmin())
	{
		opsCfg.GET("", cfgH.List)
		opsCfg.POST("", cfgH.Create)
		opsCfg.PUT("/:id", cfgH.Update)
		opsCfg.DELETE("/:id", cfgH.Delete)
		opsCfg.GET("/:id/history", cfgH.History)
	}
	cmdRunH := v1.NewCommandRunHandler(d.MonOp, oper.NewCommandRunOper(d.DB), d.MonSvc)
	cmdRuns := guarded.Group("/ops/command-runs")
	cmdRuns.Use(middleware.RequireAdmin())
	{
		cmdRuns.GET("", cmdRunH.List)
		cmdRuns.POST("", cmdRunH.Create)
		cmdRuns.GET("/:id", cmdRunH.Get)
	}
}
