package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/downloader"
	"github.com/hxpt/ptpatronus/internal/downloader/manager"
	"github.com/hxpt/ptpatronus/internal/notice"
)

// TorrentSnapshot 是单下载器的实时种子快照（含错误，便于聚合容错展示）。
type TorrentSnapshot struct {
	DownloaderID uint                    `json:"downloader_id"`
	Name         string                  `json:"name"`
	Error        string                  `json:"error,omitempty"`
	Torrents     []downloader.Torrent    `json:"torrents"`
	Stats        *downloader.GlobalStats `json:"stats,omitempty"` // qb 零请求取；tr 留空
}

// Message 是广播给所有连接的消息信封。Data 按 Type 不同承载不同结构。
type Message struct {
	Type string `json:"type"` // "torrents" | "hr-alert"
	Data any    `json:"data"`
}

// HRAlert 是一条 H&R 危险种子（对齐 automation runHRCheck 收集项）。
type HRAlert struct {
	Site     string `json:"site"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Deadline string `json:"deadline"`
}

// EncodeHRAlert 把 H&R 危险列表编码为 hr-alert 消息字节（automation 广播用）。
func EncodeHRAlert(alerts []HRAlert) ([]byte, error) {
	return json.Marshal(Message{Type: "hr-alert", Data: alerts})
}

// EncodeCookieAlert 把 cookie 失效站点名列表编码为 cookie-alert 消息字节。
func EncodeCookieAlert(sites []string) ([]byte, error) {
	return json.Marshal(Message{Type: "cookie-alert", Data: sites})
}

// FreeleechAlert 是一条正在站免通知（对齐 automation runFreeleech 收集项）。
type FreeleechAlert struct {
	Site    string `json:"site"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// EncodeFreeleechAlert 把正在站免列表编码为 freeleech-alert 消息字节。
func EncodeFreeleechAlert(alerts []FreeleechAlert) ([]byte, error) {
	return json.Marshal(Message{Type: "freeleech-alert", Data: alerts})
}

// NoticeMsg 是一条站内通知（WS 推送给前端通知中心铃铛）。分层：ws 不 import db/models，
// 由 NoticeService 从 models.Notice 转换。
type NoticeMsg struct {
	ID        uint      `json:"id"`
	Source    string    `json:"source"`
	Title     string    `json:"title"`
	Body      string    `json:"body,omitempty"`
	Level     string    `json:"level"`
	CreatedAt time.Time `json:"created_at"`
}

// EncodeNotice 把单条通知编码为 notice 消息字节（通知中心实时推送）。
func EncodeNotice(n NoticeMsg) ([]byte, error) {
	return json.Marshal(Message{Type: "notice", Data: n})
}

// Broadcaster 每 interval 聚合所有下载器种子快照，广播给所有 WS 连接。
type Broadcaster struct {
	hub      *Hub
	dlOp     *oper.DownloaderOper
	mgr      *manager.Manager
	interval time.Duration
	notice   *notice.Service     // 下载完成通知中心（nil 跳过）；通过 SetNotice 注入（避开构造顺序）
	doneHooks []DoneHook         // 下载完成副作用钩子（媒体库刷新/媒体整理等；fan-out，nil 跳过），通过 AddDownloadDoneHook 注入
	stateMu  sync.Mutex
	dlState  map[string]bool // key=dlID:hash → 是否已完成（下载完成 diff）
	primed   bool            // 冷启动首轮只填充状态不通知，避免把所有已完成种子当新通知
}

// DoneHook 下载完成副作用钩子：媒体服务器刷新等 best-effort 副作用（不应阻塞广播循环）。
// dlID=下载器 id，name/hash=种子标题/infohash。
type DoneHook func(ctx context.Context, dlID uint, name, hash string)

// SetNotice 注入通知中心（app.go 在 noticeSvc 构造后调用）。
// 经 stateMu 同步：detectDone 在 Run goroutine 内读 b.notice，setter 在主 goroutine 写，
// 复用现有锁建立 happens-before，避免未同步指针读写的数据竞争（race detector 会标记）。
func (b *Broadcaster) SetNotice(ns *notice.Service) {
	b.stateMu.Lock()
	b.notice = ns
	b.stateMu.Unlock()
}

// SetDownloadDoneHook 注入单个下载完成副作用钩子（兼容旧调用；等价于 AddDownloadDoneHook）。
// 同 SetNotice 经 stateMu 同步，detectDone 读 / setter 写均持锁。
func (b *Broadcaster) SetDownloadDoneHook(h DoneHook) {
	b.AddDownloadDoneHook(h)
}

// AddDownloadDoneHook 追加一个下载完成副作用钩子（fan-out：媒体库刷新 + 媒体整理等并存）。
// 单个钩子 panic 不阻断其余（detectDone 内逐个 recover）。
func (b *Broadcaster) AddDownloadDoneHook(h DoneHook) {
	if h == nil {
		return
	}
	b.stateMu.Lock()
	b.doneHooks = append(b.doneHooks, h)
	b.stateMu.Unlock()
}

// NewBroadcaster 构造广播器。interval<=0 时默认 2s。
func NewBroadcaster(hub *Hub, dlOp *oper.DownloaderOper, mgr *manager.Manager, interval time.Duration) *Broadcaster {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	return &Broadcaster{hub: hub, dlOp: dlOp, mgr: mgr, interval: interval}
}

// Run 启动广播循环，随 ctx 取消退出。
func (b *Broadcaster) Run(ctx context.Context) {
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()
	b.broadcast(ctx) // 启动即推一次（此时通常无连接，等价空操作）
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.broadcast(ctx)
		}
	}
}

// broadcast 聚合一次快照并广播；下载完成 diff 始终执行（不依赖 WS 连接）。
func (b *Broadcaster) broadcast(ctx context.Context) {
	snap, err := b.aggregate(ctx)
	if err != nil {
		slog.Warn("ws aggregate list downloaders failed", "err", err)
		return
	}
	b.detectDone(ctx, snap) // 下载完成通知（持久化，用户上线即可见）
	if b.hub.Count() == 0 {
		return // 无连接：不广播 torrent 快照（省不去重，但 diff 已做）
	}
	payload, err := json.Marshal(Message{Type: "torrents", Data: snap})
	if err != nil {
		return
	}
	b.hub.Broadcast(payload)
}

// detectDone 检测种子「下载完成」转换 → 通知中心。冷启动首轮只填充状态不通知。
func (b *Broadcaster) detectDone(ctx context.Context, snap []TorrentSnapshot) {
	b.stateMu.Lock()
	defer b.stateMu.Unlock()
	// notice / doneHooks 读写均经 stateMu：与 setter 建立 happens-before，消除数据竞争。
	if b.notice == nil && len(b.doneHooks) == 0 {
		return
	}
	if b.dlState == nil {
		b.dlState = map[string]bool{}
	}
	hooks := b.doneHooks
	for _, s := range snap {
		for _, t := range s.Torrents {
			if t.Hash == "" {
				continue
			}
			key := fmt.Sprintf("%d:%s", s.DownloaderID, t.Hash)
			nowDone := t.State == "completed" || !t.CompletedAt.IsZero()
			if b.primed && !b.dlState[key] && nowDone {
				if b.notice != nil {
					_ = b.notice.Notice(ctx, notice.Input{
						Source: notice.SourceDLDone,
						Key:    "dl:" + t.Hash,
						Title:  "下载完成：" + t.Name,
						Body:   s.Name,
						Level:  "success",
					}, true)
				}
				// fan-out：逐个钩子调用，单个 panic 不阻断其余（媒体库刷新 / 媒体整理并存）。
				for _, hk := range hooks {
					func(h DoneHook) {
						defer func() { _ = recover() }()
						h(ctx, s.DownloaderID, t.Name, t.Hash)
					}(hk)
				}
			}
			b.dlState[key] = nowDone
		}
	}
	b.primed = true
}

// aggregate 拉所有下载器的当前种子（单下载器失败填 error，不影响其余）。
func (b *Broadcaster) aggregate(ctx context.Context) ([]TorrentSnapshot, error) {
	dls, err := b.dlOp.List()
	if err != nil {
		return nil, err
	}
	out := make([]TorrentSnapshot, 0, len(dls))
	for i := range dls {
		d := &dls[i]
		snap := TorrentSnapshot{DownloaderID: d.ID, Name: d.Name}
		c, err := b.mgr.Get(ctx, d.ID)
		if err != nil {
			snap.Error = err.Error()
			out = append(out, snap)
			continue
		}
		ts, err := c.ListTorrents(ctx)
		if err != nil {
			snap.Error = err.Error()
			out = append(out, snap)
			continue
		}
		snap.Torrents = ts
		if sc, ok := c.(downloader.StatsCacher); ok {
			snap.Stats = sc.CachedStats()
		}
		out = append(out, snap)
	}
	return out, nil
}

// Snapshot 导出当前聚合（测试用）。
func (b *Broadcaster) Snapshot(ctx context.Context) ([]TorrentSnapshot, error) {
	return b.aggregate(ctx)
}

// Serve 把一个已升级的连接注册到 Hub、单推首帧、并启动读写泵。
// 调用方负责完成 Upgrade 与鉴权；连接生命周期交由泵管理。
func (b *Broadcaster) Serve(ctx context.Context, conn *websocket.Conn) {
	client := newClient(conn)
	b.hub.register(client)
	// 首帧：立即单推当前快照，避免新连接等满一个 interval。
	if snap, err := b.aggregate(ctx); err == nil {
		if payload, err := json.Marshal(Message{Type: "torrents", Data: snap}); err == nil {
			select {
			case client.send <- payload:
			default:
			}
		}
	}
	go client.readPump(b.hub)
	go client.writePump()
}
