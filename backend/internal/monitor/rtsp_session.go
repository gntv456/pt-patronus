// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"context"
	"encoding/binary"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	startCode         = "\x00\x00\x00\x01"
	viewerSendBuf     = 64               // 每观众发送队列（满则丢帧，视频容忍）
	viewerKeepWindow  = 30 * time.Second // 最后一个观众离开后保活窗口，期间无人则关流
	streamDialTimeout = 10 * time.Second
	hlsLease          = 30 * time.Second // HLS 租约：客户端轮询间隔通常 1-3s，30s 无请求视为离开
)

// wsViewer 一个 WebSocket 观众。send 为带缓冲帧队列（背压丢帧）。
type wsViewer struct {
	conn      *websocket.Conn
	send      chan []byte
	closeOnce sync.Once
}

// ProxySession 一路摄像头拉流，被多个 ws 观众复用（一对多）。stream 生命周期与 request 解耦。
type ProxySession struct {
	rtspURL string
	mu      sync.RWMutex
	viewers map[*wsViewer]struct{}
	codec   VideoCodec // 实际选中的编码（H264 优先，其次 H265），ready 信令要上报
	// 参数集：annexB 串供新观众补发/段头；raw 三件套供 hlsSink 按 codec 分派。
	paramsAnnexB []byte
	vpsRaw       []byte
	spsRaw       []byte
	ppsRaw       []byte
	hls          *hlsSink
	hlsTimer     *time.Timer // HLS 租约：每次 HLS 请求刷新；超 hlsLease 无请求则释放 sink
	cancel       context.CancelFunc
	startOnce    sync.Once
	startErr     error
	done         chan struct{}
	stopped      bool // stop() 后置 true，供 RTSPProxy.getOrCreate 识别僵尸 session 并重建
}

func newProxySession(rtspURL string) *ProxySession {
	return &ProxySession{rtspURL: rtspURL, viewers: map[*wsViewer]struct{}{}, done: make(chan struct{})}
}

// addViewer 注册观众，发送 ready 信令（含实际编码）+ 补发参数集。
func (s *ProxySession) addViewer(v *wsViewer) error {
	s.mu.RLock()
	codec, params := s.codec, s.paramsAnnexB
	s.mu.RUnlock()
	// 拉流尚未建轨时 codec 为空，回退 h264：此时还没有任何帧可播，
	// 前端拿到的一定是后续的「连不上」而非错误解码。
	if codec == "" {
		codec = CodecH264
	}
	ready := `{"type":"ready","codec":"` + string(codec) + `"}`
	if err := v.conn.WriteMessage(websocket.TextMessage, []byte(ready)); err != nil {
		return err
	}
	if params != nil {
		_ = v.conn.WriteMessage(websocket.BinaryMessage, frameMessage(0, params))
	}
	s.mu.Lock()
	s.viewers[v] = struct{}{}
	s.mu.Unlock()
	return nil
}

// removeViewer 注销观众（先出 map 再关 chan，避免与 broadcast 竞争写已关闭 chan）。
func (s *ProxySession) removeViewer(v *wsViewer) {
	s.mu.Lock()
	delete(s.viewers, v)
	empty := !s.anyViewerLocked()
	s.mu.Unlock()
	v.closeOnce.Do(func() { close(v.send) })
	if empty {
		// 保活窗口：期间有新观众加入则 anyViewer 为真，此处仍空才停拉流。
		time.AfterFunc(viewerKeepWindow, func() {
			if !s.anyViewer() {
				s.stop()
			}
		})
	}
}

// ensureStarted 懒启动拉流（首个观众触发，仅一次）。streamCtx 独立于 request。
func (s *ProxySession) ensureStarted() error {
	s.startOnce.Do(func() {
		streamCtx, cancel := context.WithCancel(context.Background())
		s.startErr = s.startTrack(streamCtx)
		if s.startErr != nil {
			cancel()
		} else {
			s.cancel = cancel
		}
	})
	return s.startErr
}

// broadcast 把帧发给所有观众（队列满则丢帧）。
func (s *ProxySession) broadcast(msg []byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for v := range s.viewers {
		select {
		case v.send <- msg:
		default:
		}
	}
}

// dispatch 把一个 access unit 同时喂给 ws 观众（jessibuca 帧）和 hls sink（若存在）。
// 参数集在此剥离并缓存——它们不属于任何一帧，但新观众/新段都需要。
func (s *ProxySession) dispatch(ts uint32, codec VideoCodec, nalus [][]byte) {
	s.mu.RLock()
	hls := s.hls
	s.mu.RUnlock()
	splitAccessUnits(codec, nalus,
		func(vps, sps, pps []byte) {
			s.cacheParams(paramsAnnexB(codec, vps, sps, pps), vps, sps, pps)
		},
		func(au [][]byte, _ bool) {
			s.broadcast(frameMessage(ts, makeAnnexB(au)))
			if hls != nil {
				hls.writeAU(int64(ts), codec, au)
			}
		})
}

// anyViewerLocked 报告是否还有任意类型观众（调用方持锁）：ws 观众或 HLS 租约未过期。
func (s *ProxySession) anyViewerLocked() bool {
	return len(s.viewers) > 0 || s.hlsTimer != nil
}

// anyViewer 报告是否还有任意类型观众。
func (s *ProxySession) anyViewer() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.anyViewerLocked()
}

// touchHLS 标记 HLS 在用：懒建 sink + 刷新租约定时器。HTTP 无状态，靠客户端轮询续约；
// hlsLease 内无 HLS 请求则释放 sink（releaseHLS）。
func (s *ProxySession) touchHLS() {
	s.mu.Lock()
	if s.hls == nil {
		s.hls = newHLSSink(s.codec, s.vpsRaw, s.spsRaw, s.ppsRaw)
	}
	if s.hlsTimer != nil {
		s.hlsTimer.Stop()
	}
	s.hlsTimer = time.AfterFunc(hlsLease, s.releaseHLS)
	s.mu.Unlock()
}

// releaseHLS 租约到期：释放 sink；若再无任何观众，触发保活停流。
func (s *ProxySession) releaseHLS() {
	s.mu.Lock()
	s.hlsTimer = nil
	s.hls = nil
	empty := !s.anyViewerLocked()
	s.mu.Unlock()
	if empty {
		time.AfterFunc(viewerKeepWindow, func() {
			if !s.anyViewer() {
				s.stop()
			}
		})
	}
}

// hlsPlaylist 返回当前 m3u8；sink 未建或无段返回空串（未就绪）。
func (s *ProxySession) hlsPlaylist() string {
	s.mu.RLock()
	hls := s.hls
	s.mu.RUnlock()
	if hls == nil {
		return ""
	}
	return hls.playlist()
}

// hlsSegment 返回指定 seq 的 ts 段；不存在/已淘汰返回 nil。
func (s *ProxySession) hlsSegment(seq uint64) []byte {
	s.mu.RLock()
	hls := s.hls
	s.mu.RUnlock()
	if hls == nil {
		return nil
	}
	return hls.segment(seq)
}

// stop 取消拉流并等待结束。
func (s *ProxySession) stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	timer := s.hlsTimer
	s.hlsTimer = nil
	s.stopped = true
	s.mu.Unlock()
	if timer != nil {
		timer.Stop()
	}
	if cancel != nil {
		cancel()
		<-s.done
	}
}

// isStopped 报告拉流是否已停止（保活窗口超时）。供 RTSPProxy.getOrCreate 识别僵尸 session，
// 避免 startOnce 已触发的新观众拿到已 Close 的 client（永远不出帧）。
func (s *ProxySession) isStopped() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stopped
}

// viewerCount 当前观众数（ws + 活跃 HLS 租约计 1）。
func (s *ProxySession) viewerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := len(s.viewers)
	if s.hlsTimer != nil {
		n++
	}
	return n
}

// frameMessage = 4 字节时间戳(LE) + annexB payload（jessibuca 兼容封装）。
func frameMessage(ts uint32, payload []byte) []byte {
	out := make([]byte, 4+len(payload))
	binary.LittleEndian.PutUint32(out, ts)
	copy(out[4:], payload)
	return out
}

// makeAnnexB 给每个 NAL 前加 00 00 00 01 起始码。
func makeAnnexB(nalus [][]byte) []byte {
	var out []byte
	for _, n := range nalus {
		out = append(out, startCode...)
		out = append(out, n...)
	}
	return out
}
