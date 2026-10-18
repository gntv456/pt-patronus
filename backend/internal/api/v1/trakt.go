package v1

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/chain"
)

// TraktHandler 处理 Trakt 同步配置 / 设备授权 / 同步触发。
type TraktHandler struct {
	ch *chain.TraktChain
}

// NewTraktHandler 构造。
func NewTraktHandler(ch *chain.TraktChain) *TraktHandler { return &TraktHandler{ch: ch} }

// traktCodeFor 把链层错误分成四档，返回要发出去的那条 *Error。
// 判档与文案只在接口层写一份，DeviceStatus 要在响应里多带 authorized/terminal 两个字段，
// 所以这里返回码本身而不是直接 Respond。
func traktCodeFor(err error) *errcode.Error {
	switch {
	case errors.Is(err, chain.ErrTraktNotConfigured):
		// WithDetail：链层那句「去哪儿填」原样抵达界面。
		return errcode.TraktNotConfigured.WithDetail(err)
	case errors.Is(err, chain.ErrTraktNotAuthorized):
		return errcode.TraktNotAuthorized.WithDetail(err)
	case errors.Is(err, chain.ErrTraktCodeExpired):
		return errcode.TraktCodeExpired.WithDetail(err)
	default:
		// 其余才是真上游问题（网络 / Trakt 返回非 2xx）：原因只进日志，别把英文原文推给人。
		return errcode.TraktUpstreamFailed.WithErr(err)
	}
}

func traktErr(c *gin.Context, err error) { errcode.Respond(c, traktCodeFor(err)) }

// traktConfigResp 是对外脱敏的配置视图（不回传 client_secret/access_token/refresh_token）。
type traktConfigResp struct {
	Enabled                bool   `json:"enabled"`
	ClientID               string `json:"client_id"`
	ClientSecretConfigured bool   `json:"client_secret_configured"`
	RedirectURI            string `json:"redirect_uri"`
	SyncMovies             bool   `json:"sync_movies"`
	SyncShows              bool   `json:"sync_shows"`
	Resolution             string `json:"resolution"`
	DownloaderID           uint   `json:"downloader_id"`
	Authorized             bool   `json:"authorized"`
	LastSyncAt             int64  `json:"last_sync_at"`
	LastSyncCount          int    `json:"last_sync_count"`
	LastError              string `json:"last_error"`
	PendingUserCode        string `json:"pending_user_code"`
	PendingVerificationURL string `json:"pending_verification_url"`
	PendingExpiresAt       int64  `json:"pending_expires_at"`
}

// GetConfig 返回脱敏配置 + 授权/同步状态。
func (h *TraktHandler) GetConfig(c *gin.Context) {
	s := h.ch.GetSettings()
	c.JSON(http.StatusOK, traktConfigResp{
		Enabled:                s.Enabled,
		ClientID:               s.ClientID,
		ClientSecretConfigured: s.ClientSecret != "",
		RedirectURI:            s.RedirectURI,
		SyncMovies:             s.SyncMovies,
		SyncShows:              s.SyncShows,
		Resolution:             s.Resolution,
		DownloaderID:           s.DownloaderID,
		Authorized:             s.AccessToken != "",
		LastSyncAt:             s.LastSyncAt,
		LastSyncCount:          s.LastSyncCount,
		LastError:              s.LastError,
		PendingUserCode:        s.PendingUserCode,
		PendingVerificationURL: s.PendingVerificationURL,
		PendingExpiresAt:       s.PendingExpiresAt,
	})
}

// traktConfigReq 是管理员提交的配置（密钥类字段空串=保留旧值）。
type traktConfigReq struct {
	Enabled      bool   `json:"enabled"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
	SyncMovies   bool   `json:"sync_movies"`
	SyncShows    bool   `json:"sync_shows"`
	Resolution   string `json:"resolution"`
	DownloaderID uint   `json:"downloader_id"`
}

// SetConfig 保存配置（密钥空串保留；改 client_id/secret 会作废既有令牌，需重新授权）。
func (h *TraktHandler) SetConfig(c *gin.Context) {
	var req traktConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	// 读-改-写整体交给 chain：handler 自己 Get 再 Save 的话，中间插入的令牌轮转
	// 会被这份旧快照写回库里（Trakt 的 refresh_token 一次一换，写回旧值就得重新授权）。
	if err := h.ch.UpdateConfig(chain.TraktConfigInput{
		Enabled:      req.Enabled,
		ClientID:     req.ClientID,
		ClientSecret: req.ClientSecret,
		RedirectURI:  req.RedirectURI,
		SyncMovies:   req.SyncMovies,
		SyncShows:    req.SyncShows,
		Resolution:   req.Resolution,
		DownloaderID: req.DownloaderID,
	}); err != nil {
		// #353 带上原因：底层是系统配置写库（store.Set），失败原因具体却被丢掉，界面只剩「保存失败」。
		errcode.Respond(c, errcode.SaveFailed.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeviceCode 发起设备授权（管理员触发）：返回 user_code + verification_url，用户在浏览器完成。
func (h *TraktHandler) DeviceCode(c *gin.Context) {
	s, err := h.ch.StartDeviceAuth(c.Request.Context())
	if err != nil {
		traktErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user_code":        s.PendingUserCode,
		"verification_url": s.PendingVerificationURL,
		"expires_in":       int(maxInt64(s.PendingExpiresAt-nowUnix(), 0)),
		"interval":         s.PendingInterval,
		"device_url":       "/api/v1/trakt/device",
	})
}

// DeviceStatus 轮询设备授权状态：返 authorized +（未完成时）pending 提示。
func (h *TraktHandler) DeviceStatus(c *gin.Context) {
	authorized, err := h.ch.PollDeviceStatus(c.Request.Context())
	if err != nil {
		// 面板每 3s 打一次这个 GET。授权码过期 / 凭据没填是「再等也不会亮」的终端态，
		// 所以除了 code 还要带 terminal，前端才有停下来把话说出来的依据；
		// 这里不走 errcode.Respond（它只发标准信封），但仍复用同一份判档，两处不各抄一遍。
		e := traktCodeFor(err)
		c.JSON(e.HTTP, gin.H{
			"authorized": false,
			"code":       e.Code,
			"message":    e.Message,
			"error":      e.Message,
			"terminal":   e.Code != errcode.TraktUpstreamFailed.Code,
		})
		return
	}
	s := h.ch.GetSettings()
	c.JSON(http.StatusOK, gin.H{
		"authorized":       authorized,
		"user_code":        s.PendingUserCode,
		"verification_url": s.PendingVerificationURL,
	})
}

// Sync 手动触发一次 WatchList → 订阅同步，返新增数。
func (h *TraktHandler) Sync(c *gin.Context) {
	n, err := h.ch.SyncNow(c.Request.Context())
	if err != nil {
		traktErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "created": n})
}

// History 返回最近同步状态（页面展示用，与 GetConfig 的状态字段等价的精简版）。
func (h *TraktHandler) History(c *gin.Context) {
	s := h.ch.GetSettings()
	c.JSON(http.StatusOK, gin.H{
		"authorized":      s.AccessToken != "",
		"last_sync_at":    s.LastSyncAt,
		"last_sync_count": s.LastSyncCount,
		"last_error":      s.LastError,
	})
}

// DevicePage 返回一个极简 HTML 引导页（公开路由）：展示 user_code + verification_url，
// 并用 JS 轮询 /api/v1/trakt/device-status，授权完成后提示用户回到 App。
// 只展示、不发起（P1 修复）：device code 由登录用户经 guarded 的 /trakt/device-code 发起；
// 此前匿名访问即 StartDeviceAuth 会覆盖 pending code（劫持/混淆 device-flow 绑定——
// 攻击者可抢先用自己的 Trakt 账号完成授权，轮询落库的就是攻击者 token）。
// 无 pending code 时提示回到 App 发起，并自动跳转 guarded 端点触发（302 登录后回跳）。
func (h *TraktHandler) DevicePage(c *gin.Context) {
	s := h.ch.GetSettings()
	if s.PendingUserCode == "" {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(
			`<!doctype html><html lang="zh"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">`+
				`<title>Trakt 授权</title><style>body{font-family:system-ui;max-width:480px;margin:40px auto;padding:0 16px;color:#222}</style></head>`+
				`<body><h2>Trakt 设备授权</h2><p>尚未发起授权。请先在 PTPatronus App 中点击「发起授权」，再回到本页。</p>`+
				`<p class="hint">为防止授权劫持，本页不再代为发起授权请求。</p></body></html>`))
		return
	}
	html := `<!doctype html><html lang="zh"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>Trakt 授权</title><style>body{font-family:system-ui;max-width:480px;margin:40px auto;padding:0 16px;color:#222}` +
		`code{font-size:28px;font-weight:700;letter-spacing:2px;background:#f0f0f0;padding:6px 12px;border-radius:6px}` +
		`.ok{color:#16a34a;font-weight:700}</style></head><body>` +
		`<h2>Trakt 设备授权</h2><p>1. 打开：<a id="vurl" href="#">授权页</a></p>` +
		`<p>2. 输入授权码：<br><code id="uc">` + htmlEscape(s.PendingUserCode) + `</code></p>` +
		`<p id="st">等待授权完成…</p><p id="done" class="ok" hidden>✓ 已授权，请回到 PTPatronus App。</p>` +
		`<script>const u=` + "`" + jsStringEscape(s.PendingVerificationURL) + "`" + `;` +
		`document.getElementById('vurl').href=u;document.getElementById('vurl').textContent=u;` +
		`async function poll(){try{const r=await fetch('../trakt/device-status');const j=await r.json();` +
		`if(j.authorized){document.getElementById('st').hidden=true;document.getElementById('done').hidden=false;return;}` +
		`}catch(e){}setTimeout(poll,3000);}poll();</script></body></html>`
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// htmlEscape 最小 HTML 转义（user_code 进 <code> 文本位）。
func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")
	return r.Replace(s)
}

// jsStringEscape JS 模板字面量转义（URL 进反引号字符串：截断反引号/`${`/反斜杠）。
func jsStringEscape(s string) string {
	r := strings.NewReplacer("`", "\\`", `\`, `\\`, "$", "\\$")
	return r.Replace(s)
}

// nowUnix / maxInt64 小工具。
func nowUnix() int64 { return time.Now().Unix() }
func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
