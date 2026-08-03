package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
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
	Type string `json:"type"`          // torrents | torrents_full | torrents_diff | hr-alert | ...
	Seq  uint64 `json:"seq,omitempty"` // 增量协议序列号（torrents_full/diff 带；其余消息不用）
	Data any    `json:"data"`
}

// 增量推送消息类型。
const (
	MsgTorrentsLegacy = "torrents"      // 旧全量（legacy 客户端兼容）
	MsgTorrentsFull   = "torrents_full" // 带 seq 的全量（inc 首帧 / 周期重同步）
	MsgTorrentsDiff   = "torrents_diff" // 增量（inc 稳态）
)

// fullResyncInterval 每 N 个 diff 强推一次 full，兜底丢帧/状态发散（≈60s@2s 间隔）。
const fullResyncInterval = 30

// 下载完成钩子后台 worker 配置：媒体整理等重副作用异步化，不阻塞广播循环。
const (
	hookWorkers  = 4   // 并发 worker 数（限流，防批量完成瞬时大量整理压垮系统）
	hookQueueCap = 128 // 任务队列容量；满则丢弃 + warn（best-effort，绝不阻塞广播）
)

// doneDetectInterval 无 WS 连接（无人看仪表盘）时，下载完成检测的聚合间隔。
// 有人看时复用广播聚合；无人看时降频到 10s，把闲时下载器轮询从 interval×N 降到 10s×N。
const doneDetectInterval = 10 * time.Second

// DownloaderDiff 是单个下载器本轮增量。
type DownloaderDiff struct {
	DownloaderID uint                    `json:"downloader_id"`
	Name         string                  `json:"name,omitempty"`    // 仅变化时带
	Error        string                  `json:"error"`             // 始终带（空串=无错），覆盖式赋值
	Stats        *downloader.GlobalStats `json:"stats,omitempty"`   // 仅变化时带
	Added        []downloader.Torrent    `json:"added,omitempty"`   // 新 hash
	Changed      []downloader.Torrent    `json:"changed,omitempty"` // 字段变化（整条替换）
	Removed      []string                `json:"removed,omitempty"` // 删除的 hash
}

// TorrentDiff 是本轮所有下载器的增量。
type TorrentDiff struct {
	Added   []TorrentSnapshot `json:"added,omitempty"`   // 新增的整个下载器快照
	Removed []uint            `json:"removed,omitempty"` // 删除的 downloader_id
	Changed []DownloaderDiff  `json:"changed,omitempty"` // 已存在下载器内的变更
}

// dlCache 是单个下载器的上轮快照缓存（diff 基准）。
type dlCache struct {
	torrents map[string]downloader.Torrent
	name     string
	err      string
	stats    *downloader.GlobalStats
}

// hookJob 是一个下载完成钩子任务（投递给后台 worker 异步执行，避免重整理阻塞广播）。
type hookJob struct {
	hk   DoneHook
	dlID uint
	name string
	hash string
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

// EncodeTorrentsLegacy 编码旧式全量（type=torrents，给未协商增量的客户端，保持向后兼容）。
func EncodeTorrentsLegacy(snap []TorrentSnapshot) ([]byte, error) {
	return json.Marshal(Message{Type: MsgTorrentsLegacy, Data: snap})
}

// EncodeTorrentsFull 编码带 seq 的全量（inc 首帧 / 周期重同步）。
func EncodeTorrentsFull(snap []TorrentSnapshot, seq uint64) ([]byte, error) {
	return json.Marshal(Message{Type: MsgTorrentsFull, Seq: seq, Data: snap})
}

// EncodeTorrentsDiff 编码增量（inc 稳态）。
func EncodeTorrentsDiff(diff *TorrentDiff, seq uint64) ([]byte, error) {
	return json.Marshal(Message{Type: MsgTorrentsDiff, Seq: seq, Data: diff})
}

// Broadcaster 每 interval 聚合所有下载器种子快照，广播给所有 WS 连接。
type Broadcaster struct {
	hub       *Hub
	dlOp      *oper.DownloaderOper
	mgr       *manager.Manager
	interval  time.Duration
	notice    *notice.Service // 下载完成通知中心（nil 跳过）；通过 SetNotice 注入（避开构造顺序）
	doneHooks []DoneHook      // 下载完成副作用钩子（媒体库刷新/媒体整理等；fan-out，nil 跳过），通过 AddDownloadDoneHook 注入
	stateMu   sync.Mutex
	dlState   map[string]bool // key=dlID:hash → 是否已完成（下载完成 diff）
	primed    bool            // 冷启动首轮只填充状态不通知，避免把所有已完成种子当新通知
	cacheMu   sync.Mutex
	cache     map[uint]*dlCache // 上一轮快照（diff 基准）；nil=首轮，强制全量
	seq       uint64            // 增量协议序列号（每轮 +1）
	sinceFull uint              // 距上次强推 full 的 diff 数（达 fullResyncInterval 重置）
	hookQ     chan hookJob      // 下载完成钩子任务队列（后台 worker 消费）
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
	return &Broadcaster{hub: hub, dlOp: dlOp, mgr: mgr, interval: interval, hookQ: make(chan hookJob, hookQueueCap)}
}

// Run 启动广播循环与下载完成检测，随 ctx 取消退出。
// 解耦两条关注点：种子快照广播仅在有 WS 连接时按 interval 聚合（无人看不打下载器 API）；
// 下载完成检测（持久化通知，用户上线即可见）走独立慢速 ticker，有人看时复用广播聚合。
func (b *Broadcaster) Run(ctx context.Context) {
	broadcastTick := time.NewTicker(b.interval)
	doneInterval := b.interval
	if doneDetectInterval > doneInterval {
		doneInterval = doneDetectInterval
	}
	doneTick := time.NewTicker(doneInterval)
	defer broadcastTick.Stop()
	defer doneTick.Stop()

	// 下载完成钩子 worker：媒体整理等重副作用异步执行，不阻塞广播。
	var wg sync.WaitGroup
	for i := 0; i < hookWorkers; i++ {
		wg.Add(1)
		go b.hookWorker(ctx, &wg)
	}
	defer func() {
		close(b.hookQ) // 关闭队列让 worker 退出
		wg.Wait()      // 等待在途钩子完成（ctx 已取消，尊重 ctx 的钩子会快速返回）
	}()

	b.detectFromFresh(ctx) // 启动即 prime 完成状态（首轮只填充状态不通知）
	for {
		select {
		case <-ctx.Done():
			return
		case <-broadcastTick.C:
			// 有人看才聚合广播（种子 2s 刷新只在「有人看仪表盘」时需要）。
			if b.hub.Count() > 0 {
				b.broadcastAndDetect(ctx)
			}
		case <-doneTick.C:
			// 无人看时仍低频检测完成（持久化通知）；有人看时已由 broadcastAndDetect 顺带完成。
			if b.hub.Count() == 0 {
				b.detectFromFresh(ctx)
			}
		}
	}
}

// hookWorker 消费下载完成钩子任务（限流 worker 之一），单个 panic 不阻断其余。
func (b *Broadcaster) hookWorker(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	for job := range b.hookQ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("dl-done hook panic", "downloader", job.dlID, "hash", job.hash, "panic", r)
				}
			}()
			job.hk(ctx, job.dlID, job.name, job.hash)
		}()
	}
}

// broadcastAndDetect 聚合一次快照：顺带检测下载完成（复用 snap），再广播给已连接客户端。
// 调用方须保证 hub.Count()>0（无人看时不调，省去无谓的下载器轮询）。
func (b *Broadcaster) broadcastAndDetect(ctx context.Context) {
	snap, err := b.aggregate(ctx)
	if err != nil {
		slog.Warn("ws aggregate list downloaders failed", "err", err)
		return
	}
	b.detectDone(ctx, snap) // 有人看时顺带检测下载完成（复用本轮聚合）

	b.cacheMu.Lock()
	b.seq++
	seq := b.seq
	needFull := b.cache == nil || b.sinceFull >= fullResyncInterval
	legacy := b.hub.HasLegacyClients()
	diff, next := b.buildDiff(snap, b.cache, needFull)
	b.cache = next
	if needFull {
		b.sinceFull = 0
	} else {
		b.sinceFull++
	}
	b.cacheMu.Unlock()

	// legacy 客户端收旧式 torrents 全量（仅在首轮/周期重同步/有 legacy 时序列化）；
	// inc 客户端：needFull 时收 torrents_full（重同步），否则 torrents_diff（稳态增量）。
	var legacyPayload, incPayload []byte
	if needFull || legacy {
		legacyPayload, _ = EncodeTorrentsLegacy(snap)
	}
	if needFull {
		incPayload, _ = EncodeTorrentsFull(snap, seq)
	} else {
		incPayload, _ = EncodeTorrentsDiff(diff, seq)
	}
	b.hub.BroadcastInc(legacyPayload, incPayload)
}

// detectFromFresh 聚合一次快照仅用于下载完成检测（无 WS 连接时的低频路径，不广播）。
func (b *Broadcaster) detectFromFresh(ctx context.Context) {
	snap, err := b.aggregate(ctx)
	if err != nil {
		slog.Warn("ws aggregate list downloaders failed", "err", err)
		return
	}
	b.detectDone(ctx, snap)
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
				// fan-out：钩子投递到后台 worker 异步执行（媒体整理含多次 HTTP/文件操作，
				// 同步会阻塞广播循环）；队列满则丢弃 + warn（best-effort，dlState 已置 true 防重入）。
				for _, hk := range hooks {
					select {
					case b.hookQ <- hookJob{hk: hk, dlID: s.DownloaderID, name: t.Name, hash: t.Hash}:
					default:
						slog.Warn("dl-done hook queue full, skipping", "downloader", s.DownloaderID, "hash", t.Hash)
					}
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
			snap.Torrents = []downloader.Torrent{} // 显式空数组：避免 nil → JSON "torrents":null 致前端崩溃
			out = append(out, snap)
			continue
		}
		ts, err := c.ListTorrents(ctx)
		if err != nil {
			snap.Error = err.Error()
			snap.Torrents = []downloader.Torrent{}
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

func torrentEqual(a, b downloader.Torrent) bool {
	// 标量字段直接 == 短路比较：任一不等立即返回，避免 reflect.DeepEqual 对整个 ~30 字段
	// struct 反射（慢 10-100×）。每轮广播（有人看时 2s）对每个种子调用，是热路径。
	if a.Hash != b.Hash || a.InfoHash != b.InfoHash || a.Name != b.Name ||
		a.Size != b.Size || a.Downloaded != b.Downloaded || a.Uploaded != b.Uploaded ||
		a.Progress != b.Progress || a.Ratio != b.Ratio || a.State != b.State ||
		a.DLRate != b.DLRate || a.UPRate != b.UPRate || a.SavePath != b.SavePath ||
		a.Category != b.Category || a.AddedAt != b.AddedAt ||
		a.NumSeeds != b.NumSeeds || a.NumLeechs != b.NumLeechs ||
		a.NumComplete != b.NumComplete || a.NumIncomplete != b.NumIncomplete ||
		a.Availability != b.Availability || a.RatioLimit != b.RatioLimit ||
		a.SeedingTimeLimit != b.SeedingTimeLimit || a.ForceStart != b.ForceStart ||
		a.AutoTMM != b.AutoTMM || a.Sequential != b.Sequential ||
		a.FirstLastPiece != b.FirstLastPiece || a.Priority != b.Priority ||
		a.CompletedAt != b.CompletedAt || a.LastActivity != b.LastActivity {
		return false
	}
	// 标量全等时再比 Tags 切片（通常 0-3 元素）。
	return reflect.DeepEqual(a.Tags, b.Tags)
}

func statsEqual(a, b *downloader.GlobalStats) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return reflect.DeepEqual(*a, *b)
}

// buildDiff 对比新快照与上次缓存，产出本轮增量与新缓存。纯计算（不持锁、不 I/O），便于单测。
// forceFull=true 时返回 nil diff（调用方走全量路径），但仍返回 next 缓存。
func (b *Broadcaster) buildDiff(snap []TorrentSnapshot, prev map[uint]*dlCache, forceFull bool) (*TorrentDiff, map[uint]*dlCache) {
	next := make(map[uint]*dlCache, len(snap))
	snapByID := make(map[uint]*TorrentSnapshot, len(snap))
	for i := range snap {
		s := &snap[i]
		snapByID[s.DownloaderID] = s
		cur := &dlCache{
			torrents: make(map[string]downloader.Torrent, len(s.Torrents)),
			name:     s.Name, err: s.Error, stats: s.Stats,
		}
		for _, t := range s.Torrents {
			if t.Hash != "" {
				cur.torrents[t.Hash] = t
			}
		}
		next[s.DownloaderID] = cur
	}
	if forceFull || len(prev) == 0 {
		return nil, next
	}
	diff := &TorrentDiff{}
	for id, cur := range next {
		old, ok := prev[id]
		if !ok {
			if s := snapByID[id]; s != nil {
				diff.Added = append(diff.Added, *s)
			}
			continue
		}
		nameCh := cur.name != old.name
		errCh := cur.err != old.err
		statsCh := !statsEqual(cur.stats, old.stats)
		dd := DownloaderDiff{DownloaderID: id, Error: cur.err}
		if nameCh {
			dd.Name = cur.name
		}
		if statsCh {
			dd.Stats = cur.stats
		}
		for h := range old.torrents {
			if _, exists := cur.torrents[h]; !exists {
				dd.Removed = append(dd.Removed, h)
			}
		}
		for h, newT := range cur.torrents {
			oldT, exists := old.torrents[h]
			if !exists {
				dd.Added = append(dd.Added, newT)
			} else if !torrentEqual(oldT, newT) {
				dd.Changed = append(dd.Changed, newT)
			}
		}
		if nameCh || errCh || statsCh || len(dd.Added) > 0 || len(dd.Changed) > 0 || len(dd.Removed) > 0 {
			diff.Changed = append(diff.Changed, dd)
		}
	}
	for id := range prev {
		if _, exists := next[id]; !exists {
			diff.Removed = append(diff.Removed, id)
		}
	}
	return diff, next
}

// Snapshot 导出当前聚合（测试用）。
func (b *Broadcaster) Snapshot(ctx context.Context) ([]TorrentSnapshot, error) {
	return b.aggregate(ctx)
}

// Serve 把一个已升级的连接注册到 Hub、单推首帧、并启动读写泵。
// inc=true 时客户端声明支持增量协议（收 torrents_full/torrents_diff）；否则收旧式 torrents 全量。
// 调用方负责完成 Upgrade 与鉴权；连接生命周期交由泵管理。
func (b *Broadcaster) Serve(ctx context.Context, conn *websocket.Conn, inc bool) {
	client := newClient(conn, inc)
	b.hub.register(client)
	// 首帧：立即单推当前快照（inc 用 torrents_full 带当前 seq；legacy 用旧 torrents），
	// 用当前 seq（不递增）——客户端 lastSeq=seq，下一轮 broadcast seq+1 连号 apply。
	if snap, err := b.aggregate(ctx); err == nil {
		b.cacheMu.Lock()
		seq := b.seq
		b.cacheMu.Unlock()
		var payload []byte
		if inc {
			payload, _ = EncodeTorrentsFull(snap, seq)
		} else {
			payload, _ = EncodeTorrentsLegacy(snap)
		}
		if payload != nil {
			select {
			case client.send <- payload:
			default:
			}
		}
	}
	go client.readPump(b.hub)
	go client.writePump()
}
