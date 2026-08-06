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
	// === 运维中心：Docker 容器管理（SSH 远程主机）===
	dockerH := v1.NewDockerHandler(d.MonSvc)
	execH := v1.NewDockerExecHandler()
	composeLocalH := v1.NewLocalComposeHandler()
	docker := guarded.Group("/ops/docker")
	docker.Use(middleware.RequireAdmin())
	{
		docker.GET("/:id/list", dockerH.List)
		docker.GET("/:id/stats", dockerH.Stats)
		docker.GET("/:id/containers/:name/inspect", dockerH.Inspect)
		docker.GET("/:id/containers/:name/logs", dockerH.Logs)
		docker.POST("/:id/containers/:name/start", dockerH.Start)
		docker.POST("/:id/containers/:name/stop", dockerH.Stop)
		docker.POST("/:id/containers/:name/restart", dockerH.Restart)
		docker.POST("/:id/containers/:name/pause", dockerH.Pause)
		docker.POST("/:id/containers/:name/unpause", dockerH.Unpause)
		docker.POST("/:id/containers/:name/remove", dockerH.Remove)
		docker.POST("/:id/images/:ref/remove", dockerH.ImageRemove)
		docker.POST("/:id/compose/up", dockerH.ComposeUp)
		docker.POST("/:id/compose/down", dockerH.ComposeDown)

		// 本机模式（Docker Engine API / docker.sock，无需 SSH）
		docker.GET("/local/status", dockerH.LocalStatus)
		docker.GET("/local/list", dockerH.LocalList)
		docker.GET("/local/stats", dockerH.LocalStats)
		docker.GET("/local/containers/:name/inspect", dockerH.LocalInspect)
		docker.GET("/local/containers/:name/logs", dockerH.LocalLogs)
		// remove 必须注册在 :action 通配之前（静态段优先匹配）
		docker.POST("/local/containers/:name/remove", dockerH.LocalRemove)
		docker.POST("/local/containers/:name/:action", dockerH.LocalContainerAction)
		docker.DELETE("/local/images/:ref", dockerH.LocalRemoveImage)
		// 本机扩展：拉取镜像 / 清理 / 卷网络管理
		docker.POST("/local/images/pull", dockerH.LocalPull)
		docker.POST("/local/prune", dockerH.LocalPrune)
		docker.POST("/local/volumes", dockerH.LocalVolumeCreate)
		docker.DELETE("/local/volumes/:name", dockerH.LocalVolumeRemove)
		docker.POST("/local/networks", dockerH.LocalNetworkCreate)
		docker.DELETE("/local/networks/:name", dockerH.LocalNetworkRemove)
		// 创建容器 + exec 终端
		docker.POST("/local/containers/create", dockerH.LocalCreateContainer)
		docker.GET("/local/containers/:name/exec/ws", execH.Handle)
		// 本机 Compose（容器内 CLI + socket）
		docker.GET("/local/compose/check", composeLocalH.ComposeCheck)
		docker.POST("/local/compose/up", composeLocalH.ComposeUp)
		docker.POST("/local/compose/down", composeLocalH.ComposeDown)
		docker.POST("/local/compose/validate", composeLocalH.ComposeValidate)
	}

	// === 运维中心：磁盘管理（SSH 远程主机，分区/格式化/挂载）===
	diskH := v1.NewDiskHandler(d.MonSvc)
	disks := guarded.Group("/ops/disk")
	disks.Use(middleware.RequireAdmin())
	{
		disks.GET("/:id/list", diskH.List)
		disks.POST("/:id/partition", diskH.Partition)
		disks.POST("/:id/mount", diskH.Mount)
		disks.POST("/:id/umount", diskH.Umount)
	}

	// === 运维中心：进程管理（SSH 远程主机，进程列表/网络连接/结束进程）===
	procH := v1.NewProcHandler(d.MonSvc)
	procs := guarded.Group("/ops/proc")
	procs.Use(middleware.RequireAdmin())
	{
		procs.GET("/:id/list", procH.List)
		procs.GET("/:id/connections", procH.Connections)
		procs.POST("/:id/kill", procH.Kill)
	}

	// === 运维中心：SSH 管理（SSH 服务配置/会话/登录日志）===
	sshH := v1.NewSshAdminHandler(d.MonSvc)
	sshd := guarded.Group("/ops/ssh")
	sshd.Use(middleware.RequireAdmin())
	{
		sshd.GET("/:id/status", sshH.Status)
		sshd.POST("/:id/action", sshH.Action)
		sshd.POST("/:id/enable", sshH.ToggleEnable)
		sshd.POST("/:id/config", sshH.Config)
		sshd.GET("/:id/sessions", sshH.Sessions)
		sshd.DELETE("/:id/sessions/:pid", sshH.KillSession)
		sshd.GET("/:id/logs", sshH.Logs)
	}

	// === 运维中心：数据库可视化管理（受限查询器）===
	dbH := v1.NewDatabaseHandler(d.DB)
	dbAdmin := guarded.Group("/ops/database")
	dbAdmin.Use(middleware.RequireAdmin())
	{
		dbAdmin.GET("/tables", dbH.Tables)
		dbAdmin.GET("/tables/:table/schema", dbH.Schema)
		dbAdmin.POST("/query", dbH.Query)
	}
}
