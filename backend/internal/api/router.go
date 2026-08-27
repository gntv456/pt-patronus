package api

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/middleware"
	"github.com/hxpt/ptpatronus/internal/api/v1"
	"github.com/hxpt/ptpatronus/internal/ocr"
	"github.com/hxpt/ptpatronus/internal/web"
)

// Build 构建并返回装配完整的 Gin 引擎。
func Build(d *Deps) *gin.Engine {
	if d.Cfg.IsRelease() {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	// .torrent 文件上传上限 8MB（Gin 默认 32MB，收紧防滥用）。
	engine.MaxMultipartMemory = 8 << 20
	// 仅信任配置的反向代理；未配置时不信任任何代理（ClientIP 取直连 TCP 对端），
	// 避免伪造 X-Forwarded-For 绕过限流并放大令牌桶内存。
	if tp := splitOrigins(d.Cfg.TrustedProxies); len(tp) > 0 {
		_ = engine.SetTrustedProxies(tp)
	} else {
		_ = engine.SetTrustedProxies(nil)
	}
	engine.Use(gin.Recovery())
	engine.Use(middleware.RequestID())
	engine.Use(middleware.AccessLog())
	engine.Use(middleware.CORS(splitOrigins(d.Cfg.CORSAllowedOrigins)...))
	engine.Use(middleware.SecurityHeaders())
	engine.Use(middleware.MaxBody(64 << 20)) // 全局 body 上限兜底，防超大 JSON 灌内存 DoS（公开端点亦覆盖）
	engine.Use(middleware.JWTAuth(d.JWT, d.Cfg.APIKey, d.TVCache))
	// 全局限流（每 IP 每秒）
	globalRPS := d.Cfg.GlobalRatePerSec
	if globalRPS <= 0 {
		globalRPS = 20
	}
	engine.Use(middleware.NewRateLimiter(float64(globalRPS), globalRPS).Middleware())

	authH := v1.NewAuthHandler(d.DB, d.JWT, d.Act, d.HxptAuth, d.Crypto, d.TVCache, d.Tickets, d.Cfg.DemoMode, d.OIDC)
	// OCR 验证码自动识别（SystemConfig 热更；未配置 = 人工输入，不影响登录链路）。
	if d.CfgOp != nil {
		cfgOp := d.CfgOp
		authH.SetOCRConfig(func() ocr.Config {
			get := func(k string) string { v, _ := cfgOp.Get(k); return v }
			return ocr.Config{Endpoint: get("ocr.endpoint"), APIKey: get("ocr.api_key"), Mode: get("ocr.mode")}
		})
	}
	sysH := v1.NewSystemHandler(d.Update)
	schedH := v1.NewScheduleHandler(d.Automation, d.Cfg.SnapshotCron, d.Cfg.CookieKeepAliveCron)

	api := engine.Group("/api/v1")
	registerPublicRoutes(api, d, authH, sysH)
	registerGuardedRoutes(api, d, authH, sysH, schedH)

	// Servarr 兼容（Sonarr/Radarr v3）：挂 /api/v3 根（非 /api/v1 下），供 Overseerr/Jellyseerr 接入。
	// 公开自鉴权（?apikey= 或 X-Api-Key 头比对全局 API Key）；safe-mode 下 d.Servarr 为 nil 不注册。
	// 注意：lookup 等静态路径须在 :id 之前注册，避免 :id 捕获 "lookup"。
	if d.Servarr != nil {
		v3 := engine.Group("/api/v3")
		v3.GET("/system/status", d.Servarr.SystemStatus)
		v3.GET("/qualityProfile", d.Servarr.StaticList)
		v3.GET("/profile", d.Servarr.StaticList)
		v3.GET("/rootfolder", d.Servarr.StaticList)
		v3.GET("/tag", d.Servarr.StaticList)
		v3.GET("/languageprofile", d.Servarr.StaticList)
		// Radarr (movie)
		v3.GET("/movie", d.Servarr.ListMovies)
		v3.GET("/movie/lookup", d.Servarr.LookupMovies)
		v3.POST("/movie", d.Servarr.AddMovie)
		v3.GET("/movie/:id", d.Servarr.GetMovie)
		v3.DELETE("/movie/:id", d.Servarr.RemoveMovie)
		// Sonarr (series)
		v3.GET("/series", d.Servarr.ListSeries)
		v3.GET("/series/lookup", d.Servarr.LookupSeries)
		v3.POST("/series", d.Servarr.AddSeries)
		v3.PUT("/series", d.Servarr.UpdateSeries)
		v3.GET("/series/:id", d.Servarr.GetSeries)
		v3.DELETE("/series/:id", d.Servarr.RemoveSeries)
	}

	// 非匹配路径回退到内嵌的 Web UI（go:embed），由 vue-router 接管。
	// /api/ 前缀的未命中必须返回 JSON 404：否则前端会把 SPA 的 index.html 当 JSON 解析，
	// 报 "Unexpected token '<'"，掩盖真正的端点拼写 / HTTP 方法错误（如把 PUT 误用为 POST）。
	spa := web.Handler() // http.Handler：文件存在直出，否则回退 index.html
	engine.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		spa.ServeHTTP(c.Writer, c.Request)
	})
	return engine
}
