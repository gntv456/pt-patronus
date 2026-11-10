// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"fmt"
	"github.com/hxpt/ptpatronus/internal/httpclient"
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
// 四轮 P2 修复：过期条目此前仅在"再次被访问"时删除，未被回访的条目永驻——媒体库
// 几十万条目时按 PlaybackInfo 调用量单调涨内存。每次写入后顺手清扫全表过期项
// （条目带 exp，Range 一遍 Delete 即可，成本低；清扫与写入并发安全——sync.Map）。
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
	p.sweepPathCache()
	return path, nil
}

// sweepPathCache 清扫过期缓存项（写后触发，均摊成本；限制单次最多删 1024 项防长暂停）。
func (p *MediaProxyService) sweepPathCache() {
	now := time.Now()
	n := 0
	p.pathCache.Range(func(k, v any) bool {
		if e, ok := v.(pathCacheEntry); ok && now.After(e.exp) {
			p.pathCache.Delete(k)
			n++
			if n >= 1024 {
				return false
			}
		}
		return true
	})
}

// NewMediaProxyService 构造。
// 超时说明：整体 Timeout 只约束到响应头返回；媒体流正文经 ResponseHeaderTimeout
// 后由 Transport 接管（无整体上限，避免长视频被掐断），慢上游不会再无限挂起 handler。
func NewMediaProxyService(ms *MediaServerChain, cl *CloudChain) *MediaProxyService {
	return &MediaProxyService{ms: ms, cloud: cl, hc: &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}}
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

// directHeaderAllowed 302 时允许以 X-Direct-* 透给客户端的直链请求头白名单。
// 只放行播放器实际需要的 Referer/User-Agent 等；Cookie/Authorization 等凭据类头
// （如夸克 DirectLink.Header["Cookie"] = 账号会话、115 开放平台 Bearer token）
// 绝不外泄——白名单制而非黑名单，新增凭据头默认被挡。
func allowedDirectHeader(k string) bool {
	switch k {
	case "Referer", "referer", "User-Agent", "user-agent",
		"Range", "range", "Accept", "accept", "Content-Type", "content-type":
		return true
	}
	return false
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
	// 302 前把直链所需头透给客户端（夸克等直链需 Referer/UA 绑定）。
	// 白名单制：DirectLink.Header 可能携带账号凭据（如夸克 drive.Token 即 Cookie），
	// 原样 X-Direct-* 回写会向匿名调用者泄露网盘会话——凭据类头绝不透出。
	for k, v := range link.Header {
		if !allowedDirectHeader(k) {
			continue
		}
		w.Header().Set("X-Direct-"+k, v)
	}
	http.Redirect(w, r, link.URL, http.StatusFound)
	slog.Info("mediaproxy: 302 direct link", "item", tgt.ItemID, "url", redactDirectURL(link.URL))
	return true
}

// resolveLink 从媒体路径解析云盘直链：.strm 内容 / StrmRecord 按云盘路径。
// 二轮 P1-2（mediaproxy 三连修）：
//  1. .strm 内容含 ?sign= 时剥离 query 再取路径（签名模式下路径必带 query，不剥则
//     statCloud 按 "a.mkv?sign=..." 找文件必失败）；
//  2. 解析前先过 VerifyDirectLinkSign——mediaproxy 是公开路由，此前直接调
//     ResolveDirectLink 完全绕过了 /cloud/direct 端点的签名校验（匿名枚举 Emby
//     itemId 换直链的口子）；兼容模式（无密钥）保持放行；
//  3. .strm 解析失败不再提前 return——落回 ② StrmRecord 匹配（挂载型/畸形 strm
//     的拦截此前静默失效）。
func (p *MediaProxyService) resolveLink(ctx context.Context, mediaPath, ua string) (*cloud.DirectLink, bool) {
	// ① 本地 .strm：内容若指向 /api/v1/cloud/direct/:driveId/*，复用端点解析
	if strings.HasSuffix(strings.ToLower(mediaPath), ".strm") {
		if _, driveID, cp, sign, ok := parseDirectStrmFile(mediaPath); ok {
			if p.cloud != nil && p.cloud.VerifyDirectLinkSign(driveID, cp, sign) {
				if link, err := p.cloud.ResolveDirectLink(ctx, driveID, cp, ua); err == nil {
					return link, true
				}
			}
		}
		// 签名不符/解析失败/网盘解析失败：落到 ② 记录匹配（而非直接放弃）
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

// parseDirectStrmFile 读取 .strm 内容，识别 /api/v1/cloud/direct/:driveId/<path>[?sign=…]。
// 返回的 cloudPath 已剥离 query；sign 为签名串（无签名为空串，由调用方决定校验策略）。
func parseDirectStrmFile(strmPath string) (urlStr string, driveID uint, cloudPath, sign string, ok bool) {
	b, err := os.ReadFile(strmPath) // .strm 单行小文件（≤4KB 防御性上限）
	if err != nil || len(b) > 4096 {
		return "", 0, "", "", false
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", 0, "", "", false
	}
	const marker = "/cloud/direct/"
	i := strings.Index(s, marker)
	if i < 0 {
		return "", 0, "", "", false // 非 .strm 指向本端的情形（挂载路径）→ 走记录匹配
	}
	rest := s[i+len(marker):]
	// 二轮修复 1：先剥 query（签名模式下 URL 带 ?sign=，不剥则路径污染）
	if q := strings.IndexAny(rest, "?#"); q >= 0 {
		if eq := strings.Index(rest[q:], "sign="); eq >= 0 {
			sign = rest[q+eq+len("sign="):]
		}
		rest = rest[:q]
	}
	slash := strings.Index(rest, "/")
	if slash <= 0 {
		return "", 0, "", "", false
	}
	id, cp := rest[:slash], rest[slash:]
	var d uint
	if _, err := fmt.Sscanf(id, "%d", &d); err != nil || d == 0 {
		return "", 0, "", "", false
	}
	return s, d, cp, sign, true
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
	httpclient.CopyBody(w, resp.Body, "mediaproxy", resp.ContentLength, "target", redactDirectURL(target), "status", resp.StatusCode)
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
		if strings.EqualFold(k, "Host") {
			continue
		}
		// 握手头例外：Connection/Upgrade 虽是逐跳头，但升级握手本身依赖它们
		//（此前一并按 hopByHop 剥掉 → 上游看到的是普通 GET，永远不回 101）。
		if hopByHop[k] && !strings.EqualFold(k, "Connection") && !strings.EqualFold(k, "Upgrade") {
			continue
		}
		for _, v := range vs {
			upReq.Header.Add(k, v)
		}
	}
	// 对齐 net/http/httputil.ReverseProxy：过滤后显式补回规范化握手头
	//（防客户端 Connection 值夹带 close/keep-alive 等 token 干扰上游判定）。
	upReq.Header.Set("Connection", "Upgrade")
	upReq.Header.Set("Upgrade", "websocket")
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
	// 双向 pipe：上游→客户端直写 clientConn——绝不经过 bufio.Writer（io.Copy 到
	// EOF 才返回，缓冲未满时 101 握手与小帧永远滞留客户端收不到，WS 全链路静默失效）；
	// 客户端→上游从 hijack 的 bufio.Reader 读（已预读的请求字节不能丢）。
	done := make(chan struct{}, 2)
	go func() { // 上游 → 客户端
		defer func() { done <- struct{}{} }()
		_, _ = io.Copy(clientConn, conn)
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
