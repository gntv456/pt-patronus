package monitor

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"github.com/hxpt/ptpatronus/internal/auth"
	"github.com/hxpt/ptpatronus/internal/core/security"
)

// RTSPProxy 管理每摄像头一路 RTSP 拉流的 WebSocket 观众复用。
// WS 鉴权用一次性 ticket（?ticket=，与 ws.go 同构）；HLS 鉴权仍用 query JWT（?token=，media_kit 多段拉取 + 不能设 header）。
// 路由放 guarded 外：WS/HLS 无法走 header 鉴权。
type RTSPProxy struct {
	mu             sync.Mutex
	sessions       map[uint]*ProxySession
	jwt            *security.JWT           // HLS playlist/segment 的 query token 校验
	tickets        *auth.TicketStore       // WS /stream 的一次性 ticket 校验
	tvCache        *auth.TokenVersionCache // HLS token 的 TokenVersion 校验（登出/改绑后旧 token 立即失效）
	allowedOrigins map[string]bool
	maxViewers     int                           // 全局并发观众上限（防 CPU/带宽耗尽）
	resolveURL     func(id uint) (string, error) // 按 :id 从 DB 取 RTSPURL（服务端控制，防 SSRF/流劫持）
}

// NewRTSPProxy 构造。maxViewers<=0 时默认 8。resolveURL 按 id 返回该摄像头配置的 RTSP 地址。
func NewRTSPProxy(jwt *security.JWT, tickets *auth.TicketStore, tvCache *auth.TokenVersionCache, origins []string, maxViewers int, resolveURL func(id uint) (string, error)) *RTSPProxy {
	if maxViewers <= 0 {
		maxViewers = 8
	}
	set := make(map[string]bool, len(origins))
	for _, o := range origins {
		if o = strings.TrimSpace(o); o != "" && o != "*" {
			set[o] = true
		}
	}
	return &RTSPProxy{
		sessions: map[uint]*ProxySession{}, jwt: jwt, tickets: tickets, tvCache: tvCache, allowedOrigins: set, maxViewers: maxViewers,
		resolveURL: resolveURL,
	}
}

var rtspUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096, // 视频帧比文本大，buffer 调大
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true }, // 由 HandleWS 的 checkOrigin 精确判定
}

// HandleWS 是 WebSocket 升级入口（GET /api/v1/monitor/cameras/:id/stream?ticket=）。
// url 由服务端按 :id 从 DB 取（MonitorTarget.RTSPURL），禁止客户端传入——
// 防 SSRF（拉任意 rtsp://内网地址）与跨用户流劫持（getOrCreate 按 id 缓存）。
func (p *RTSPProxy) HandleWS(c *gin.Context) {
	if !p.checkOrigin(c.Request) {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "origin not allowed"})
		return
	}
	if _, ok := p.tickets.Consume(c.Query("ticket")); !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired ticket"})
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid camera id"})
		return
	}
	// url 由服务端按 :id 从 DB 取，禁止客户端 query 传入（防 SSRF + 流劫持）。
	rtspURL, err := p.resolveURL(uint(id))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "摄像头不存在"})
		return
	}
	if !strings.HasPrefix(strings.ToLower(rtspURL), "rtsp") {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "该摄像头未配置有效的 RTSP 地址"})
		return
	}

	// 全局观众上限 + 取/建 session（复用同摄像头拉流）。
	sess, ok := p.getOrCreate(uint(id), rtspURL)
	if !ok {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "too many viewers"})
		return
	}

	conn, err := rtspUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	v := &wsViewer{conn: conn, send: make(chan []byte, viewerSendBuf)}
	if err := sess.addViewer(v); err != nil {
		_ = conn.Close()
		return
	}
	if err := sess.ensureStarted(); err != nil {
		sess.removeViewer(v)
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"rtsp stream failed"}`))
		_ = conn.Close()
		return
	}
	go viewerWritePump(v)
	viewerReadPump(v) // 阻塞至客户端断开
	sess.removeViewer(v)
}

// getOrCreate 取或建 session；超全局观众上限返回 ok=false。
func (p *RTSPProxy) getOrCreate(id uint, rtspURL string) (*ProxySession, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	total := 0
	for _, s := range p.sessions {
		total += s.viewerCount()
	}
	if total >= p.maxViewers {
		return nil, false
	}
	sess := p.sessions[id]
	if sess == nil || sess.isStopped() { // 已停止的 session（保活窗口后 stop）重建，避免新观众拿到死流
		sess = newProxySession(rtspURL)
		p.sessions[id] = sess
	}
	return sess, true
}

// session 取已存在的 session（不创建）；不存在或已停止返回 nil。
func (p *RTSPProxy) session(id uint) *ProxySession {
	p.mu.Lock()
	defer p.mu.Unlock()
	sess := p.sessions[id]
	if sess == nil || sess.isStopped() {
		return nil
	}
	return sess
}

// authHLS 校验 ?token=<jwt>（HLS 走 HTTP，无需 CheckOrigin）；失败写 401 并返回 false。
func (p *RTSPProxy) authHLS(c *gin.Context) bool {
	token := c.Query("token")
	if token == "" {
		errcode.Respond(c, errcode.UnauthorizedOp)
		return false
	}
	claims, err := p.jwt.Parse(token)
	if err != nil {
		errcode.Respond(c, errcode.InvalidToken)
		return false
	}
	// TokenVersion 校验：登出/改绑后旧 token 立即失效（与 JWTAuth 中间件一致；
	// WS 走一次性 ticket 已天然防重放，HLS 走长效 query token 需补此校验）。
	if p.tvCache != nil {
		cur, err := p.tvCache.Get(claims.UserID)
		// 与 JWTAuth 同一套分档：读不出来 ≠ 会话已更新（后者该重新登录，前者不该）。
		switch {
		case err == nil:
			if cur != claims.TokenVersion {
				errcode.Respond(c, errcode.TokenMismatch)
				return false
			}
		case errors.Is(err, auth.ErrUserMissing):
			slog.Warn("rtsp auth rejected: token points at a missing user",
				"uid", claims.UserID)
			errcode.Respond(c, errcode.TokenMismatch)
			return false
		default:
			slog.Warn("rtsp auth rejected: token version unreadable",
				"uid", claims.UserID, "err", err)
			errcode.Respond(c, errcode.Unavailable.WithErr(err))
			return false
		}
	}
	return true
}

// ServePlaylist 返回摄像头 HLS 播放列表（GET /monitor/cameras/:id/hls/playlist.m3u8?token=）。
// 鉴权复用 ?token=<jwt>（与 stream 同构，media_kit HTTP 不能设 header）；首拉未出段返 503+Retry-After。
func (p *RTSPProxy) ServePlaylist(c *gin.Context) {
	if !p.authHLS(c) {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid camera id"})
		return
	}
	rtspURL, err := p.resolveURL(uint(id))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "摄像头不存在"})
		return
	}
	if !strings.HasPrefix(strings.ToLower(rtspURL), "rtsp") {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "该摄像头未配置有效的 RTSP 地址"})
		return
	}
	sess, ok := p.getOrCreate(uint(id), rtspURL)
	if !ok {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "too many viewers"})
		return
	}
	if err := sess.ensureStarted(); err != nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "rtsp stream failed"})
		return
	}
	sess.touchHLS()
	playlist := sess.hlsPlaylist()
	if playlist == "" {
		c.Header("Retry-After", "2")
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "stream starting"})
		return
	}
	// 段 URL 是相对路径（segment/<seq>.ts）：按 RFC 3986 相对解析会丢弃 playlist 的
	// ?token= query，标准播放器逐段请求会 401。把 token 显式写进本响应的段 URL
	// （仅本响应副本；session ring 里仍是干净 URL）。
	playlist = rewriteSegmentToken(playlist, c.Query("token"))
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "application/vnd.apple.mpegurl", []byte(playlist))
}

// rewriteSegmentToken 把 query token 注入 playlist 的每个段 URL。
// 仅匹配 ".ts\n"（段行），不碰 EXTINF/TARGETDURATION 等标签行。
func rewriteSegmentToken(playlist, token string) string {
	return strings.ReplaceAll(playlist, ".ts\n", ".ts?token="+url.QueryEscape(token)+"\n")
}

// ServeSegment 返回一个 HLS ts 段（GET /monitor/cameras/:id/hls/segment/:seq?token=）。
func (p *RTSPProxy) ServeSegment(c *gin.Context) {
	if !p.authHLS(c) {
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid camera id"})
		return
	}
	seq, err := strconv.ParseUint(strings.TrimSuffix(c.Param("seq"), ".ts"), 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid segment"})
		return
	}
	sess := p.session(uint(id))
	if sess == nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "stream not found"})
		return
	}
	sess.touchHLS()
	data := sess.hlsSegment(seq)
	if data == nil {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"error": "segment expired"})
		return
	}
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "video/mp2t", data)
}

// viewerWritePump 消费 send 队列写 ws。
func viewerWritePump(v *wsViewer) {
	defer v.conn.Close()
	for msg := range v.send {
		if err := v.conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
			return
		}
	}
}

// viewerReadPump 读取客户端消息（浏览器通常不发包），用于检测断开。
func viewerReadPump(v *wsViewer) {
	defer v.conn.Close()
	for {
		if _, _, err := v.conn.ReadMessage(); err != nil {
			return
		}
	}
}

// checkOrigin 防 CSWSH：配置白名单时浏览器来源必须命中；非浏览器（无 Origin）放行。
func (p *RTSPProxy) checkOrigin(r *http.Request) bool {
	if len(p.allowedOrigins) == 0 {
		return true
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	return p.allowedOrigins[origin]
}
