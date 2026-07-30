package monitor

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/gorilla/websocket"
	"github.com/pion/rtp"
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
	rtspURL   string
	mu        sync.RWMutex
	viewers   map[*wsViewer]struct{}
	client    *gortsplib.Client
	spsPPS    []byte // 缓存 SPS/PPS（annexB），新观众补发
	spsRaw    []byte // 原始 SPS（供 hlsSink 建段头）
	ppsRaw    []byte // 原始 PPS
	hls       *hlsSink
	hlsTimer  *time.Timer // HLS 租约：每次 HLS 请求刷新；超 hlsLease 无请求则释放 sink
	cancel    context.CancelFunc
	startOnce sync.Once
	startErr  error
	done      chan struct{}
	stopped   bool // stop() 后置 true，供 RTSPProxy.getOrCreate 识别僵尸 session 并重建
}

func newProxySession(rtspURL string) *ProxySession {
	return &ProxySession{rtspURL: rtspURL, viewers: map[*wsViewer]struct{}{}, done: make(chan struct{})}
}

// addViewer 注册观众，发送 ready 信令 + 补发 SPS/PPS。
func (s *ProxySession) addViewer(v *wsViewer) error {
	if err := v.conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"ready","codec":"h264"}`)); err != nil {
		return err
	}
	s.mu.Lock()
	s.viewers[v] = struct{}{}
	spsPPS := s.spsPPS
	s.mu.Unlock()
	if spsPPS != nil {
		_ = v.conn.WriteMessage(websocket.BinaryMessage, frameMessage(0, spsPPS))
	}
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
		s.startErr = s.startStream(streamCtx)
		if s.startErr != nil {
			cancel()
		} else {
			s.cancel = cancel
		}
	})
	return s.startErr
}

// startStream 拉流 → 找 H264 轨 → Setup/Play → OnPacketRTP 解码为 annexB 广播。
func (s *ProxySession) startStream(ctx context.Context) error {
	u, err := base.ParseURL(s.rtspURL)
	if err != nil {
		return err
	}
	c := &gortsplib.Client{
		Scheme:      u.Scheme,
		Host:        u.Host,
		DialContext: (&net.Dialer{Timeout: streamDialTimeout}).DialContext,
	}
	s.client = c
	if err := c.Start(); err != nil {
		return err
	}
	desc, _, err := c.Describe(u)
	if err != nil {
		c.Close()
		return err
	}
	var h264Media *description.Media
	var h264Forma *format.H264
	for _, m := range desc.Medias {
		for _, f := range m.Formats {
			if h, ok := f.(*format.H264); ok {
				h264Media, h264Forma = m, h
				break
			}
		}
		if h264Media != nil {
			break
		}
	}
	if h264Media == nil {
		c.Close()
		return fmt.Errorf("no h264 track in stream")
	}
	if _, err := c.Setup(desc.BaseURL, h264Media, 0, 0); err != nil {
		c.Close()
		return err
	}
	dec, err := h264Forma.CreateDecoder()
	if err != nil {
		c.Close()
		return err
	}
	if len(h264Forma.SPS) > 0 && len(h264Forma.PPS) > 0 {
		spsPPS := makeAnnexB([][]byte{h264Forma.SPS, h264Forma.PPS})
		s.mu.Lock()
		s.spsPPS = spsPPS
		s.spsRaw = h264Forma.SPS
		s.ppsRaw = h264Forma.PPS
		s.mu.Unlock()
		s.broadcast(frameMessage(0, spsPPS))
	}
	c.OnPacketRTP(h264Media, h264Forma, func(pkt *rtp.Packet) {
		nalus, err := dec.Decode(pkt)
		if err != nil || len(nalus) == 0 {
			return
		}
		s.dispatch(pkt.Timestamp, nalus)
	})
	if _, err := c.Play(nil); err != nil {
		c.Close()
		return err
	}
	go func() {
		defer close(s.done)
		defer c.Close()
		<-ctx.Done()
	}()
	return nil
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
func (s *ProxySession) dispatch(ts uint32, nalus [][]byte) {
	s.broadcast(frameMessage(ts, makeAnnexB(nalus)))
	s.mu.RLock()
	hls := s.hls
	s.mu.RUnlock()
	if hls != nil {
		hls.writeAU(int64(ts), nalus)
	}
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
		s.hls = newHLSSink(s.spsRaw, s.ppsRaw)
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
