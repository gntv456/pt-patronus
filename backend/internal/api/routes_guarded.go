package api

import (
	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/middleware"
	"github.com/hxpt/ptpatronus/internal/api/v1"
	"github.com/hxpt/ptpatronus/internal/auth"
)

// registerGuardedRoutes 创建受保护路由组（需登录 + VIP）并注册其全部子路由。
func registerGuardedRoutes(api *gin.RouterGroup, d *Deps, authH *v1.AuthHandler, sysH *v1.SystemHandler, schedH *v1.ScheduleHandler) {
	// 受保护路由：需登录 + 好学 VIP（按用户，宽限期内放行）。
	guarded := api.Group("")
	guarded.Use(middleware.RequireAuth())
	vipGrace := d.VIPGraceDays
	if vipGrace <= 0 {
		vipGrace = 3
	}
	guarded.Use(auth.RequireVIP(d.DB, vipGrace))
	guarded.POST("/auth/logout", authH.Logout)
	guarded.POST("/auth/ws-ticket", authH.WsTicket)
	// OIDC/SSO 配置（管理员；start/callback 在公开区）
	guarded.GET("/auth/oidc/config", middleware.RequireAdmin(), authH.GetOIDCConfig)
	guarded.PUT("/auth/oidc/config", middleware.RequireAdmin(), authH.SetOIDCConfig)
	// 在线更新：客户端经本后端中转检查（不直连官方源）；管理员写自托管清单覆盖官方源
	guarded.GET("/system/update/check", sysH.UpdateCheck)
	guarded.POST("/admin/update/manifest", middleware.RequireAdmin(), sysH.SetManifest)

	registerSiteRoutes(guarded, d)
	registerDownloaderRoutes(guarded, d, schedH)
	registerMediaRoutes(guarded, d)
	registerOpsRoutes(guarded, d)
	registerEntertainmentRoutes(api, guarded, d)
	registerSystemRoutes(api, guarded, d, sysH)
	registerMonitorRoutes(guarded, d)
}
