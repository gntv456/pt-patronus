package monitor

import (
	"context"
	"fmt"
	"net"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/pion/rtp"
)

// startTrack 完成 RTSP 拉流的握手与收流接线：DESCRIBE → 选轨 → SETUP → Play → RTP 回调。
// 从 rtsp_session.go 拆出，让那个文件专注会话/观众生命周期，行数守住 400 门禁。
func (s *ProxySession) startTrack(ctx context.Context) error {
	u, err := base.ParseURL(s.rtspURL)
	if err != nil {
		return err
	}
	c := &gortsplib.Client{
		Scheme:      u.Scheme,
		Host:        u.Host,
		DialContext: (&net.Dialer{Timeout: streamDialTimeout}).DialContext,
	}
	if err := c.Start(); err != nil {
		return err
	}
	desc, _, err := c.Describe(u)
	if err != nil {
		c.Close()
		return err
	}
	track, err := pickVideoTrack(desc)
	if err != nil {
		c.Close()
		return fmt.Errorf("%w（该摄像头只支持 MJPEG/音频等本系统不解码的编码）", err)
	}
	if _, err := c.Setup(desc.BaseURL, track.media, 0, 0); err != nil {
		c.Close()
		return err
	}

	// 先登记编码再缓存参数集：addViewer 的 ready 信令要读 codec，
	// 而观众可能在 Play 之后才接入，顺序反了会发出错的 codec。
	s.mu.Lock()
	s.codec = track.codec
	s.mu.Unlock()
	s.cacheParams(track.annexBParams(), track.vps, track.sps, track.pps)

	c.OnPacketRTP(track.media, track.forma, func(pkt *rtp.Packet) {
		nalus, err := track.dec.Decode(pkt)
		if err != nil || len(nalus) == 0 {
			return
		}
		s.dispatch(pkt.Timestamp, track.codec, nalus)
	})
	if _, err := c.Play(nil); err != nil {
		c.Close()
		return err
	}
	safego.Go("rtsp_stream:69", func() {
		defer close(s.done)
		defer c.Close()
		<-ctx.Done()
	})
	return nil
}

// cacheParams 缓存参数集（SDP 带的 sprop-*，以及流内捕获到的更新）。
// annexB 非 nil 时同时广播给已在线观众——编码器中途切分辨率会重发参数集，
// 不补发的话，已打开的播放器会一直花屏直到刷新。
func (s *ProxySession) cacheParams(annexB, vps, sps, pps []byte) {
	if annexB == nil {
		return
	}
	s.mu.Lock()
	s.paramsAnnexB = annexB
	s.vpsRaw, s.spsRaw, s.ppsRaw = vps, sps, pps
	s.mu.Unlock()
	s.broadcast(frameMessage(0, annexB))
}
