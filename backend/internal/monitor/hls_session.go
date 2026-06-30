package monitor

import (
	"bytes"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	tscodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
)

const (
	hlsMaxSegs = 5      // ring：保留最近 N 个 ts 段（sliding window）
	hlsClock   = 90000  // H264 RTP 时钟频率（Hz），PTS/DTS 单位
	hlsMinDur  = 1.0    // 段时长下限（秒），防 0/负
)

// hlsSegment 是一个已完成的 MPEG-TS 段。
type hlsSegment struct {
	seq      uint64
	duration float64 // 秒
	data     []byte  // 完整 ts 段字节
}

// hlsSink 把 ProxySession 解出的 H264 AVCC 流转成 MPEG-TS 分片 + live HLS 播放列表，
// 供标准播放器（media_kit/libmpv）消费。与 jessibuca 的裸 ES over WS 链路完全独立。
//
// 分段策略：IDR 作边界；每个新段新建 mpegts.Writer 并先写 PAT/PMT（WriteTables），
// 保证每段可独立解码（HLS 规范要求）。ring 保留最近 hlsMaxSegs 段。
type hlsSink struct {
	sps []byte
	pps []byte

	mu           sync.Mutex
	dtsExtractor *h264.DTSExtractor
	track        *mpegts.Track
	curBuf       *bytes.Buffer
	curWriter    *mpegts.Writer
	segStartPTS  int64
	haveSegment  bool
	segs         []*hlsSegment
	mediaSeq     uint64
	lastAccess   time.Time
}

func newHLSSink(sps, pps []byte) *hlsSink {
	return &hlsSink{sps: sps, pps: pps, lastAccess: time.Now()}
}

// writeAU 喂一个 H264 access unit（AVCC NALU 列表），pts 为 90kHz 单位。
func (s *hlsSink) writeAU(pts int64, au [][]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastAccess = time.Now()

	// 过滤：捕获 SPS/PPS、丢 AUD、识别 IDR。
	var filtered [][]byte
	idr := false
	nonIDR := false
	for _, nalu := range au {
		if len(nalu) == 0 {
			continue
		}
		switch h264.NALUType(nalu[0] & 0x1F) {
		case h264.NALUTypeSPS:
			s.sps = nalu
			continue
		case h264.NALUTypePPS:
			s.pps = nalu
			continue
		case h264.NALUTypeAccessUnitDelimiter:
			continue
		case h264.NALUTypeIDR:
			idr = true
		case h264.NALUTypeNonIDR:
			nonIDR = true
		}
		filtered = append(filtered, nalu)
	}
	au = filtered
	if len(au) == 0 || (!idr && !nonIDR) {
		return
	}

	// DTSExtractor 需在首个 IDR 初始化（在此之前静默丢弃）。
	if s.dtsExtractor == nil {
		if !idr {
			return
		}
		s.dtsExtractor = &h264.DTSExtractor{}
		s.dtsExtractor.Initialize()
	}

	// IDR 前补 SPS/PPS，保证段可独立解码。
	if idr && len(s.sps) > 0 && len(s.pps) > 0 {
		au = append([][]byte{s.sps, s.pps}, au...)
	}

	dts, err := s.dtsExtractor.Extract(au, pts)
	if err != nil {
		return
	}

	// 分段边界：遇 IDR 先收尾上一段、再开新段。
	if idr {
		if s.haveSegment {
			s.finalizeSegment(pts)
		}
		s.startSegment(pts)
	}
	if s.curWriter == nil {
		return
	}
	_ = s.curWriter.WriteH264(s.track, pts, dts, au)
}

// startSegment 开新段：新 buffer + 新 Writer，先写 PAT/PMT。
func (s *hlsSink) startSegment(pts int64) {
	s.curBuf = new(bytes.Buffer)
	s.track = &mpegts.Track{Codec: &tscodecs.H264{}}
	s.curWriter = &mpegts.Writer{W: s.curBuf, Tracks: []*mpegts.Track{s.track}}
	if err := s.curWriter.Initialize(); err != nil {
		s.curWriter = nil
		s.curBuf = nil
		return
	}
	_, _ = s.curWriter.WriteTables() // 段头 PAT/PMT（late-joining 客户端可独立解码）
	s.segStartPTS = pts
	s.haveSegment = true
}

// finalizeSegment 收尾当前段并入 ring（超限丢最旧）。
func (s *hlsSink) finalizeSegment(nextPTS int64) {
	if s.curBuf == nil {
		return
	}
	data := s.curBuf.Bytes()
	if len(data) == 0 {
		s.curBuf = nil
		s.curWriter = nil
		s.haveSegment = false
		return
	}
	dur := float64(nextPTS-s.segStartPTS) / float64(hlsClock)
	if dur < hlsMinDur {
		dur = hlsMinDur
	}
	seg := &hlsSegment{seq: s.mediaSeq, duration: dur, data: append([]byte(nil), data...)}
	s.mediaSeq++
	s.segs = append(s.segs, seg)
	if len(s.segs) > hlsMaxSegs {
		s.segs = s.segs[len(s.segs)-hlsMaxSegs:]
	}
	s.curBuf = nil
	s.curWriter = nil
	s.haveSegment = false
}

// playlist 返回当前 live 播放列表（m3u8）；无段时返回空串（未就绪）。
func (s *hlsSink) playlist() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.segs) == 0 {
		return ""
	}
	maxDur := hlsMinDur
	for _, seg := range s.segs {
		if seg.duration > maxDur {
			maxDur = seg.duration
		}
	}
	target := int(math.Ceil(maxDur))
	var b bytes.Buffer
	b.WriteString("#EXTM3U\n")
	b.WriteString("#EXT-X-VERSION:3\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", target)
	fmt.Fprintf(&b, "#EXT-X-MEDIA-SEQUENCE:%d\n", s.segs[0].seq)
	for _, seg := range s.segs {
		fmt.Fprintf(&b, "#EXTINF:%.3f,\n", seg.duration)
		fmt.Fprintf(&b, "segment/%d.ts\n", seg.seq)
	}
	return b.String()
}

// segment 返回指定序列号的 ts 段字节；不在 ring 内返回 nil。
func (s *hlsSink) segment(seq uint64) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, seg := range s.segs {
		if seg.seq == seq {
			s.lastAccess = time.Now()
			return seg.data
		}
	}
	return nil
}

// idle 报告自 since 以来是否无访问（保活判定用）。
func (s *hlsSink) idle(since time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastAccess.Before(since)
}
