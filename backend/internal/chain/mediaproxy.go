package chain

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/cloud"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/mediaserver"
)

// ---- 媒体代理（Emby/JF 反代 + 302 直链 + 失败回退） ----
//
// 对标 MSaber 媒体代理：用户经本端 `/api/v1/mediaproxy/:serverId/*` 访问 Emby。
// 普通请求原样反向转发；原始流播放请求（original.mkv/stream）→ 查条目文件路径 →
// ① .strm：读取内容；若指向本端 /cloud/direct 端点则直接执行该解析逻辑；
// ② 网盘挂载路径（路径位于某网盘 RootPath/CloudBase 约定下）：按路径查 StrmRecord →
// 直链解析 → 302。任一步失败：原样转发回 Emby（播放器无感回退）。

// MediaProxyService 反代 + 直链改写。
type MediaProxyService struct {
	ms    *MediaServerChain
	cloud *CloudChain
	hc    *http.Client // 转发用普通客户端（内网直连，无 SSRF 守卫；目标为管理员配置的服务器）

	// pathCache PlaybackInfo 条目→媒体路径缓存（对标 MSaber「影片条目与媒体源路径复用」：
	// 同片每次播放/进度上报都查一次 PlaybackInfo 是上游往返浪费；命中省一次 API）。
	pathCache sync.Map // key "serverID:itemID" → pathCacheEntry{path, exp}
}

// mediaPathCacheTTL 路径缓存有效期：媒体库刷新周期内条目路径基本不变，10 分钟
// 平衡新鲜度与命中率；直链失败自动回退保证陈旧条目最多多一次回源。
const mediaPathCacheTTL = 10 * time.Minute

type pathCacheEntry struct {
	path string
	exp  time.Time
}

// cachedMediaPath 带缓存的 MediaPath（负结果不缓存——失败常见于临时网络问题）。
func (p *MediaProxyService) cachedMediaPath(ctx context.Context, serverID uint, itemID string) (string, error) {
	key := fmt.Sprintf("%d:%s", serverID, itemID)
	if v, ok := p.pathCache.Load(key); ok {
		if e, ok := v.(pathCacheEntry); ok && time.Now().Before(e.exp) {
			return e.path, nil
		}
		p.pathCache.Delete(key)
	}
	path, err := p.ms.MediaPath(ctx, serverID, itemID)
	if err != nil {
		return "", err
	}
	p.pathCache.Store(key, pathCacheEntry{path: path, exp: time.Now().Add(mediaPathCacheTTL)})
	return path, nil
}

// NewMediaProxyService 构造。
func NewMediaProxyService(ms *MediaServerChain, cl *CloudChain) *MediaProxyService {
	return &MediaProxyService{ms: ms, cloud: cl, hc: &http.Client{}}
}

// ServeHTTP gin 入口：/mediaproxy/:id/*rest。
func (p *MediaProxyService) Handle(w http.ResponseWriter, r *http.Request, serverID uint, rest string) {
	if p == nil || p.ms == nil {
		http.Error(w, "proxy not configured", http.StatusBadGateway)
		return
	}
	m, err := p.ms.Get(serverID)
	if err != nil {
		http.Error(w, "media server not found", http.StatusNotFound)
		return
	}
	// 播放拦截（Emby/JF 及其兼容网关 ugos/fnos/zspace——同一 REST；Plex 协议不同，保守全转发）
	switch m.Type {
	case "emby", "jellyfin", "ugos", "fnos", "zspace":
		if tgt, ok := mediaserver.MatchPlaybackEmby(rest); ok {
			if p.tryDirectLink(w, r, serverID, tgt) {
				return
			}
			// 直链失败：落到转发（回退原服务器）
		}
	}
	p.forward(w, r, m.URL, rest)
}

// tryDirectLink 尝试把播放请求 302 到云盘直链；成功已写响应返回 true。
func (p *MediaProxyService) tryDirectLink(w http.ResponseWriter, r *http.Request, serverID uint, tgt mediaserver.PlaybackTarget) bool {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Warn("mediaproxy: direct link panic", "err", rec)
		}
	}()
	mediaPath, err := p.cachedMediaPath(r.Context(), serverID, tgt.ItemID)
	if err != nil {
		slog.Debug("mediaproxy: media path failed", "item", tgt.ItemID, "err", err)
		return false
	}
	link, ok := p.resolveLink(r.Context(), mediaPath, r.UserAgent())
	if !ok {
		return false
	}
	// 302 前把直链所需头透给客户端（夸克等直链需 Cookie/Referer/UA 绑定）。
	for k, v := range link.Header {
		w.Header().Set("X-Direct-"+k, v)
	}
	http.Redirect(w, r, link.URL, http.StatusFound)
	slog.Info("mediaproxy: 302 direct link", "item", tgt.ItemID, "url", redactDirectURL(link.URL))
	return true
}

// resolveLink 从媒体路径解析云盘直链：.strm 内容 / StrmRecord 按云盘路径。
func (p *MediaProxyService) resolveLink(ctx context.Context, mediaPath, ua string) (*cloud.DirectLink, bool) {
	// ① 本地 .strm：内容若指向 /api/v1/cloud/direct/:driveId/*，复用端点解析
	if strings.HasSuffix(strings.ToLower(mediaPath), ".strm") {
		if _, driveID, cp, ok := parseDirectStrmFile(mediaPath); ok {
			if p.cloud != nil {
				if link, err := p.cloud.ResolveDirectLink(ctx, driveID, cp, ua); err == nil {
					return link, true
				}
			}
		}
		return nil, false
	}
	// ② StrmRecord：按「网盘 CloudBase + 相对路径」约定反查。上传链路登记的
	// CloudPath = CloudBase/库内相对路径；Emby 看到的 STRM 库根 = StrmOutputDir。
	// 尝试各网盘 RootPath/记录 CloudPath 与 mediaPath 的尾部匹配（最多 4 段）。
	if p.cloud == nil {
		return nil, false
	}
	for _, rec := range p.cloud.candidateRecords(mediaPath) {
		if link, err := p.cloud.ResolveDirectLink(ctx, rec.DriveID, rec.CloudPath, ua); err == nil {
			return link, true
		}
	}
	return nil, false
}

// parseDirectStrmFile 读取 .strm 内容，识别 /api/v1/cloud/direct/:driveId/<path>。
func parseDirectStrmFile(strmPath string) (urlStr string, driveID uint, cloudPath string, ok bool) {
	b, err := os.ReadFile(strmPath) // .strm 单行小文件（≤4KB 防御性上限）
	if err != nil || len(b) > 4096 {
		return "", 0, "", false
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", 0, "", false
	}
	const marker = "/cloud/direct/"
	i := strings.Index(s, marker)
	if i < 0 {
		return "", 0, "", false // 非 .strm 指向本端的情形（挂载路径）→ 走记录匹配
	}
	rest := s[i+len(marker):]
	slash := strings.Index(rest, "/")
	if slash <= 0 {
		return "", 0, "", false
	}
	id, cp := rest[:slash], rest[slash:]
	var d uint
	if _, err := fmt.Sscanf(id, "%d", &d); err != nil || d == 0 {
		return "", 0, "", false
	}
	return s, d, cp, true
}

// hopByHop 逐跳头（代理必须剥离，不能透传）。
var hopByHop = map[string]bool{
	"Connection": true, "Proxy-Connection": true, "Keep-Alive": true,
	"Proxy-Authenticate": true, "Proxy-Authorization": true, "Te": true,
	"Trailer": true, "Transfer-Encoding": true, "Upgrade": true,
}

// forward 原样反向转发到媒体服务器（透明代理，流式双向拷贝；支持 WebSocket Upgrade）。
func (p *MediaProxyService) forward(w http.ResponseWriter, r *http.Request, base, rest string) {
	target := strings.TrimRight(base, "/") + "/" + strings.TrimLeft(rest, "/")
	// WebSocket：hijack 后双向 pipe（Emby/JF 的 socket.io 实时通知用）
	if isUpgrade(r) {
		p.forwardWS(w, r, target)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, r.Body)
	if err != nil {
		http.Error(w, "proxy: bad target", http.StatusBadGateway)
		return
	}
	for k, vs := range r.Header {
		if hopByHop[k] || strings.EqualFold(k, "Host") {
			continue
		}
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	if u, err := url.Parse(target); err == nil {
		req.Host = u.Host
	}
	resp, err := p.hc.Do(req)
	if err != nil {
		slog.Warn("mediaproxy: forward failed", "target", redactDirectURL(target), "err", err)
		http.Error(w, "proxy: upstream unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		if hopByHop[k] {
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// isUpgrade WebSocket 升级请求判定。
func isUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// forwardWS WebSocket 透传：拨上游 → 回写 101 → 双向拷贝直至任一侧关闭。
func (p *MediaProxyService) forwardWS(w http.ResponseWriter, r *http.Request, target string) {
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "proxy: websocket unsupported", http.StatusInternalServerError)
		return
	}
	// http.Client 不支持 Upgrade：换原生拨号复用请求头
	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		http.Error(w, "proxy: bad ws target", http.StatusBadGateway)
		return
	}
	for k, vs := range r.Header {
		if hopByHop[k] || strings.EqualFold(k, "Host") {
			continue
		}
		for _, v := range vs {
			upReq.Header.Add(k, v)
		}
	}
	if u, err := url.Parse(target); err == nil {
		upReq.Host = u.Host
	}
	// 强制 HTTP/1.1（websocket 握手必需）
	conn, err := netDialContext(r.Context(), "tcp", wsAddr(target))
	if err != nil {
		http.Error(w, "proxy: ws dial failed", http.StatusBadGateway)
		return
	}
	defer conn.Close()
	if err := upReq.Write(conn); err != nil {
		http.Error(w, "proxy: ws handshake failed", http.StatusBadGateway)
		return
	}
	clientConn, clientBuf, err := hj.Hijack()
	if err != nil {
		return
	}
	defer clientConn.Close()
	// 双向 pipe：上游→客户端 由 bufio flush；客户端→上游 直接拷
	done := make(chan struct{}, 2)
	go func() { // 上游 → 客户端
		defer func() { done <- struct{}{} }()
		br := bufio.NewReader(conn)
		for {
			if _, err := br.Peek(1); err != nil {
				return
			}
			if _, err := io.Copy(clientBuf, br); err != nil {
				return
			}
			if err := clientBuf.Flush(); err != nil {
				return
			}
		}
	}()
	go func() { // 客户端 → 上游
		defer func() { done <- struct{}{} }()
		_, _ = io.Copy(conn, clientBuf.Reader)
	}()
	select {
	case <-done:
	case <-r.Context().Done():
	}
}

// netDialContext TCP 拨号（测试可替换）。
var netDialContext = (&net.Dialer{}).DialContext

// wsAddr 从 URL 提取 host:port（缺省按 scheme 补 80/443）。
func wsAddr(target string) string {
	u, err := url.Parse(target)
	if err != nil {
		return ""
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		if u.Scheme == "https" {
			host += ":443"
		} else {
			host += ":80"
		}
	}
	return host
}

// redactDirectURL 隐藏直链 query（日志防泄露签名）。
func redactDirectURL(raw string) string {
	if i := strings.Index(raw, "?"); i >= 0 {
		return raw[:i] + "?…"
	}
	return raw
}

// candidateRecords CloudChain 辅助：按媒体路径尾部枚举 StrmRecord 候选。
// 上传链路的 .strm 库结构 = StrmOutputDir + CloudPath 相对段，故用路径尾部（最多
// 保留 4 段）与记录 CloudPath 尾部比对。返回驱动与云盘路径。
func (c *CloudChain) candidateRecords(mediaPath string) []models.StrmRecord {
	recs, err := c.ruleOp.ListRecords(0)
	if err != nil || len(recs) == 0 {
		return nil
	}
	name := path.Base(mediaPath)
	baseNoExt := strings.TrimSuffix(name, path.Ext(name))
	var out []models.StrmRecord
	for _, rec := range recs {
		rn := path.Base(rec.CloudPath)
		if rn == name || strings.TrimSuffix(rn, path.Ext(rn)) == baseNoExt {
			out = append(out, rec)
			if len(out) >= 5 {
				break // 候选上限：同名多盘时逐一尝试，5 个足够
			}
		}
	}
	return out
}
