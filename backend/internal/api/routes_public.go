package api

import (
	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/middleware"
	"github.com/hxpt/ptpatronus/internal/api/v1"
	"github.com/hxpt/ptpatronus/internal/chain"
)

// registerPublicRoutes 注册无需登录的公开路由（系统健康、鉴权登录、WebSocket、流媒体、MCP 等）。
func registerPublicRoutes(api *gin.RouterGroup, d *Deps, authH *v1.AuthHandler, sysH *v1.SystemHandler) {
	api.GET("/system/health", sysH.Health)
	api.GET("/system/info", sysH.Info)

	// 公开路由：激活前可达。登录/激活端点加更严限流防爆破。
	loginLimiter := middleware.NewRateLimiter(loginRPS(d.Cfg), 5)
	authRoutes := api.Group("/auth")
	authRoutes.Use(loginLimiter.Middleware())
	{
		authRoutes.POST("/setup", authH.Setup)
		authRoutes.POST("/login", authH.Login)
		authRoutes.POST("/hxpt/captcha", authH.HxptCaptcha)
		authRoutes.POST("/hxpt/login", authH.HxptLogin)
		authRoutes.POST("/hxpt/cookie-login", authH.HxptCookieLogin)
		// JWT 滑动续期：JWTAuth 已验签（过期 token 到不了这），无需再进 guarded/限流收紧。
		authRoutes.POST("/refresh", authH.Refresh)
		authRoutes.POST("/demo-login", authH.DemoLogin)
		authRoutes.POST("/activate", authH.Activate)
		authRoutes.GET("/status", authH.Status)
		authRoutes.POST("/verify-vip", authH.VerifyVIP)
		// OIDC/SSO：start/callback 为公开 302（浏览器跳转）；config 为管理员配置。
		authRoutes.GET("/oidc/start", authH.OIDCStart)
		authRoutes.GET("/oidc/callback", authH.OIDCCallback)
	}

	// WebSocket 实时推送（?ticket=<一次性> 鉴权；不进 guarded 的 header 鉴权）。
	wsH := v1.NewWSHandler(d.WS, d.Tickets, splitOrigins(d.Cfg.CORSAllowedOrigins))
	api.GET("/ws", wsH.Handle)

	// RTSP 视频流（?token=<jwt> 鉴权；浏览器不能直播 RTSP，由 RTSPProxy 转 WS）。
	streamH := v1.NewMonitorStreamHandler(d.RTSPProxy)
	api.GET("/monitor/cameras/:id/stream", streamH.Stream)
	terminalH := v1.NewMonitorTerminalHandler(d.MonSvc, d.Tickets, d.DB, splitOrigins(d.Cfg.CORSAllowedOrigins))
	api.GET("/monitors/:id/terminal", terminalH.Handle)
	// Agent 上报长连接（?agent_id=&secret= 鉴权；生产强制 wss://，secret 走 query 非 TLS 会被嗅探）。
	api.GET("/monitor/agents/ws", d.AgentHub.HandleWS)
	// Agent 一键安装脚本（公开，curl | bash；secret 由 env 传入不入 URL）。
	api.GET("/monitor/agents/install.sh", v1.ServeAgentInstall)
	// 摄像头 HLS（标准协议，供移动端 media_kit 等消费；同 ?token=<jwt> 鉴权）。
	api.GET("/monitor/cameras/:id/hls/playlist.m3u8", streamH.Playlist)
	api.GET("/monitor/cameras/:id/hls/segment/:seq", streamH.Segment)
	// 进站 Webhook 公开接收（token 即鉴权；静态 /in 前缀避开管理端 :id 参数名冲突）。
	api.POST("/webhooks/in/:token", d.Webhook.Receive)
	// 媒体服务器播放回调公开接收（服务器无法带 JWT；按 :id 路由，仅 NotifyOnPlay 开启时通知）。
	api.POST("/mediaserver/:id/webhook", v1.NewMediaServerHandler(d.MediaServer).Webhook)
	// Emby 302 直链反代（公开：播放器不能带 JWT）。读取服务器 DirectLinkBase 配置静态改写，
	// 不泄露 Token；未开启直链的服务器返 404。
	api.GET("/mediaserver/:id/direct/*path", v1.NewMediaServerHandler(d.MediaServer).Direct)
	// STRM 302 直链（公开：.strm 内容指向本端，播放器不能带 JWT）。按网盘+云盘路径
	// 实时解析限时直链后 302；路径经 RootPath 钳制，不泄露凭据。
	api.GET("/cloud/direct/:driveId/*path", v1.NewCloudHandler(d.Cloud).Direct)
	// 媒体代理（公开：Emby 客户端把服务器地址指向本端）。播放请求拦截→云盘直链 302，
	// 其余透明反代回源服务器；直链失败自动回退转发（播放器无感）。
	mpH := v1.NewMediaProxyHandler(chain.NewMediaProxyService(d.MediaServer, d.Cloud))
	api.Any("/mediaproxy/:id/*rest", mpH.Handle)
	// MCP 服务端（公开：MCP 客户端不能走 JWT。由 mcp.Server 自身校验 enable + Bearer token）。
	api.GET("/mcp", gin.WrapH(d.MCP))
	api.POST("/mcp", gin.WrapH(d.MCP))
	// LLM 兼容代理（公开：外部客户端不能走 JWT；handler 自验 Bearer/x-api-key 访问令牌）。
	// safe-mode 下 d.LLMGateway 为 nil（agent 未构造）时不注册。
	if d.LLMGateway != nil {
		api.POST("/openai/v1/chat/completions", d.LLMGateway.OpenAIChat)
		api.POST("/anthropic/v1/messages", d.LLMGateway.AnthropicMessages)
	}
	// Trakt OAuth Device 授权页（公开：授权时用户尚未登录，浏览器直接打开）。
	api.GET("/trakt/device", v1.NewTraktHandler(d.Trakt).DevicePage)

	// 内置 CookieCloud 服务端（公开：浏览器扩展无登录会话；uuid+密码即鉴权，数据端到端
	// 加密服务端只存密文）。与 guarded 组 /cookiecloud/{config,sync,push} 并存不冲突：
	// gin 按方法+完整路径精确匹配，公开组只挂 get/:uuid 与 update 两个子路径。
	ccSrvH := v1.NewCookieCloudServerHandler(d.CookieCloud)
	api.GET("/cookiecloud/get/:uuid", ccSrvH.Get)
	api.POST("/cookiecloud/update", ccSrvH.Update)
}
