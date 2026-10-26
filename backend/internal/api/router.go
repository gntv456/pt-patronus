package api

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/api/middleware"
	"github.com/hxpt/ptpatronus/internal/api/v1"
	"github.com/hxpt/ptpatronus/internal/ocr"
	"github.com/hxpt/ptpatronus/internal/web"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	// Prometheus 指标（/metrics 端点注册见下；此处采集全部业务请求）
	engine.Use(middleware.Prometheus())
	// 标准 /metrics 端点：Prometheus 抓取格式（go_*/process_*/ptpatronus_http_*）。
	// 鉴权（收权改造）：Bearer METRICS_TOKEN（缺省回退 API_KEY）。抓取器配置
	//   - Authorization: Bearer <token>（或 X-API-Key: <token>）
	// release 模式下未配置任何令牌 → 端点整体禁用（404）：指标含路径/延迟分布，
	// 不应匿名公开；debug 模式保持无鉴权便于本地开发。
	metricsToken := d.Cfg.MetricsToken
	if !d.Cfg.IsRelease() || metricsToken != "" {
		engine.GET("/metrics", middleware.MetricsAuth(metricsToken), gin.WrapH(promhttp.Handler()))
	}
	// /api/v1/ws：鉴权走一次性 ticket（query）。浏览器 WS 升级请求既无法带
	// Authorization 头、也无法省略自动附带的 ptp_token cookie —— 对该路径跳过
	// cookie 通道，避免过期残留 cookie 在 ticket 校验前抢先 401（实时推送停摆 +
	// 前端重连风暴）。Bearer/API-Key 通道不受影响。
	// 建会话的入口同样要跳过 cookie 通道（脏 httpOnly cookie 不能抢在登录前判 401，
	// 理由见 routes_public.go 的 loginEntryPaths 注释）。
	engine.Use(middleware.JWTAuth(d.JWT, d.Cfg.APIKey, d.TVCache,
		append([]string{"/api/v1/ws"}, loginEntryPaths...)...))
	// 全局限流（每 IP 每秒）
	globalRPS := d.Cfg.GlobalRatePerSec
	if globalRPS <= 0 {
		globalRPS = 20
	}
	engine.Use(middleware.NewRateLimiter(float64(globalRPS), globalRPS).Middleware())

	authH := v1.NewAuthHandler(d.DB, d.JWT, d.Act, d.HxptAuth, d.Crypto, d.TVCache, d.Tickets, d.Cfg.DemoMode, d.OIDC,
		strings.HasPrefix(d.Cfg.AppBaseURL, "https://"))
	// OCR 验证码自动识别（SystemConfig 热更；未配置 = 人工输入，不影响登录链路）。
	if d.CfgOp != nil {
		cfgOp := d.CfgOp
		authH.SetOCRConfig(func() ocr.Config {
			get := func(k string) string { v, _ := cfgOp.Get(k); return v }
			return ocr.Config{Endpoint: get("ocr.endpoint"), APIKey: get("ocr.api_key"), Mode: get("ocr.mode")}
		})
	}
	sysH := v1.NewSystemHandler(d.Update, d.DB)
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

	// 摄像头预览的 jessibuca 资源：不在镜像里（GPL-3.0），由后端按需下载到
	// DataDir/jessibuca 后直出。必须注册在 NoRoute 之前——SPA fallback 对任何未命中
	// 路径都返回 index.html，浏览器拿到 200+HTML 会当成 wasm 报解码错误。
	// 未登录也可达：预览发生在登录后的页面里，但资源请求本身不应要求鉴权
	// （jessibuca 内部还会 fetch decoder.js 作为 Worker 脚本）。
	engine.GET("/jessibuca/*file", gin.WrapH(web.AssetHandler(web.NewAssetStore(d.Cfg.DataDir))))

	// 非匹配路径回退到内嵌的 Web UI（go:embed），由 vue-router 接管。
	// /api/ 前缀的未命中必须返回 JSON 404：否则前端会把 SPA 的 index.html 当 JSON 解析，
	// 报 "Unexpected token '<'"，掩盖真正的端点拼写 / HTTP 方法错误（如把 PUT 误用为 POST）。
	spa := web.Handler() // http.Handler：文件存在直出，否则回退 index.html
	engine.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			// 统一 errcode 信封（{code, message, error}）：前端拦截器按 code 编程判断，
			// 文案走 i18n；此前裸 {"error":"not found"} 是信封闭环的最后一个缺口。
			errcode.Respond(c, errcode.NotFound)
			return
		}
		spa.ServeHTTP(c.Writer, c.Request)
	})
	return engine
}
