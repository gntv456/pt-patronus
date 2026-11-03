package api

import (
	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/middleware"
	"github.com/hxpt/ptpatronus/internal/api/v1"
)

// registerGuardedRoutes 创建受保护路由组（需登录）并注册其全部子路由。
func registerGuardedRoutes(api *gin.RouterGroup, d *Deps, authH *v1.AuthHandler, sysH *v1.SystemHandler, schedH *v1.ScheduleHandler) {
	// 受保护路由：需登录即可。好学 VIP 准入已随站点关闭移除（#<issue>）：
	// 旧 RequireVIP 闸门退役，登录身份即完整授权；VIP 字段仅作历史快照保留。
	guarded := api.Group("")
	guarded.Use(middleware.RequireAuth())
	guarded.POST("/auth/logout", authH.Logout)
	guarded.POST("/auth/ws-ticket", authH.WsTicket)
	// 本地密码修改（登录即用，不要求管理员——单用户实例主人自助改密）
	guarded.POST("/auth/local/password", authH.ChangePassword)
	// 用户界面偏好（跨浏览器同步）：ui/dashboard 等域，登录即拉取
	guarded.GET("/auth/preferences/:key", authH.GetPreferences)
	guarded.PUT("/auth/preferences/:key", authH.SetPreferences)
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
