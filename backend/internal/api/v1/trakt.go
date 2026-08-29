package v1

import (
	"net/http"
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
	cur := h.ch.GetSettings()
	next := cur
	next.Enabled = req.Enabled
	next.SyncMovies = req.SyncMovies
	next.SyncShows = req.SyncShows
	next.Resolution = req.Resolution
	next.DownloaderID = req.DownloaderID
	if req.RedirectURI != "" {
		next.RedirectURI = req.RedirectURI
	}
	// client_id/secret 改变 → 作废既有令牌（凭据已换，旧令牌无效）。
	if req.ClientID != "" && req.ClientID != cur.ClientID {
		next.ClientID = req.ClientID
		next.AccessToken = ""
		next.RefreshToken = ""
		next.PendingDeviceCode = ""
	}
	if req.ClientSecret != "" {
		next.ClientSecret = req.ClientSecret
		if req.ClientSecret != cur.ClientSecret {
			next.AccessToken = ""
			next.RefreshToken = ""
			next.PendingDeviceCode = ""
		}
	}
	if err := h.ch.SaveSettings(next); err != nil {
		errcode.Respond(c, errcode.SaveFailed)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeviceCode 发起设备授权（管理员触发）：返回 user_code + verification_url，用户在浏览器完成。
func (h *TraktHandler) DeviceCode(c *gin.Context) {
	s, err := h.ch.StartDeviceAuth(c.Request.Context())
	if err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
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
		c.JSON(http.StatusBadRequest, gin.H{"authorized": false, "error": err.Error()})
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
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
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
func (h *TraktHandler) DevicePage(c *gin.Context) {
	_, _ = h.ch.StartDeviceAuth(c.Request.Context())
	s := h.ch.GetSettings()
	html := `<!doctype html><html lang="zh"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>Trakt 授权</title><style>body{font-family:system-ui;max-width:480px;margin:40px auto;padding:0 16px;color:#222}` +
		`code{font-size:28px;font-weight:700;letter-spacing:2px;background:#f0f0f0;padding:6px 12px;border-radius:6px}` +
		`.ok{color:#16a34a;font-weight:700}</style></head><body>` +
		`<h2>Trakt 设备授权</h2><p>1. 打开：<a id="vurl" href="#">授权页</a></p>` +
		`<p>2. 输入授权码：<br><code id="uc">` + s.PendingUserCode + `</code></p>` +
		`<p id="st">等待授权完成…</p><p id="done" class="ok" hidden>✓ 已授权，请回到 PTPatronus App。</p>` +
		`<script>const u=` + "`" + s.PendingVerificationURL + "`" + `;` +
		`document.getElementById('vurl').href=u;document.getElementById('vurl').textContent=u;` +
		`async function poll(){try{const r=await fetch('../trakt/device-status');const j=await r.json();` +
		`if(j.authorized){document.getElementById('st').hidden=true;document.getElementById('done').hidden=false;return;}` +
		`}catch(e){}setTimeout(poll,3000);}poll();</script></body></html>`
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// nowUnix / maxInt64 小工具。
func nowUnix() int64 { return time.Now().Unix() }
func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
