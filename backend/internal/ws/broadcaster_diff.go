// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package ws

import (
	"context"
	"reflect"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hxpt/ptpatronus/internal/downloader"
)

// aggregate 拉所有下载器的当前种子（单下载器失败填 error，不影响其余）。
// 并发抓取 + 单下载器独立超时：原串行实现里任一台不可达（qB 客户端单请求 30s
// HTTP 超时）会把每轮 2s 的广播 tick 卡满 30s——仪表盘实时种子列表/速率全部
// 冻结，下载完成检测（detectFromFresh 同走这里）也一并延迟。并发有界（8），
// 与 chain 一键签到同口径；单台预算 5s 足够正常拉取，不可达时快速失败不拖垮整轮。
func (b *Broadcaster) aggregate(ctx context.Context) ([]TorrentSnapshot, error) {
	dls, err := b.dlOp.List()
	if err != nil {
		return nil, err
	}
	out := make([]TorrentSnapshot, len(dls)) // 保序：按入参下标写回，快照序列稳定
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i := range dls {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// 广播循环是长驻 goroutine，单台实现 panic 不能带走整个进程；
			// 吞掉 panic 并把该台标为 error，与抓取失败同语义。
			defer func() {
				if r := recover(); r != nil {
					out[i] = TorrentSnapshot{
						DownloaderID: dls[i].ID, Name: dls[i].Name,
						Error: "downloader fetch panic", Torrents: []downloader.Torrent{},
					}
				}
			}()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				out[i] = TorrentSnapshot{
					DownloaderID: dls[i].ID, Name: dls[i].Name,
					Error: ctx.Err().Error(), Torrents: []downloader.Torrent{},
				}
				return
			}
			d := &dls[i]
			snap := TorrentSnapshot{DownloaderID: d.ID, Name: d.Name}
			// 显式空数组：避免 nil → JSON "torrents":null 致前端崩溃
			snap.Torrents = []downloader.Torrent{}
			cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			c, err := b.mgr.Get(cctx, d.ID)
			if err != nil {
				snap.Error = err.Error()
				out[i] = snap
				return
			}
			ts, err := c.ListTorrents(cctx)
			if err != nil {
				snap.Error = err.Error()
				out[i] = snap
				return
			}
			snap.Torrents = ts
			if sc, ok := c.(downloader.StatsCacher); ok {
				snap.Stats = sc.CachedStats()
			}
			out[i] = snap
		}(i)
	}
	wg.Wait()
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
