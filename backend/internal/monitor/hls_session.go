package monitor

import (
	"bytes"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts"
	tscodecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mpegts/codecs"
)

const (
	hlsMaxSegs = 5     // ring：保留最近 N 个 ts 段（sliding window）
	hlsClock   = 90000 // RTP 时钟频率（Hz），PTS/DTS 单位。H264/H265 同为 90kHz。
	hlsMinDur  = 1.0   // 段时长下限（秒），防 0/负
)

// hlsSegment 是一个已完成的 MPEG-TS 段。
type hlsSegment struct {
	seq      uint64
	duration float64 // 秒
	data     []byte  // 完整 ts 段字节
}

// hlsSink 把 ProxySession 解出的视频 AU 流转成 MPEG-TS 分片 + live HLS 播放列表，
// 供标准播放器（media_kit/libmpv）消费。与 jessibuca 的裸 ES over WS 链路完全独立。
//
// 分段策略：随机访问帧（IDR/CRA）作边界；每个新段新建 mpegts.Writer 并先写
// PAT/PMT（WriteTables），保证每段可独立解码（HLS 规范要求）。ring 保留最近
// hlsMaxSegs 段。H264 与 H265 共用本 sink，差异（track codec、DTS 提取器、
// 参数集个数）全部收敛在各自的小函数里。
type hlsSink struct {
	codec VideoCodec
	vps   []byte
	sps   []byte
	pps   []byte

	mu          sync.Mutex
	dtsH264     *h264.DTSExtractor
	dtsH265     *h265.DTSExtractor
	track       *mpegts.Track
	curBuf      *bytes.Buffer
	curWriter   *mpegts.Writer
	segStartPTS int64
	haveSegment bool
	segs        []*hlsSegment
	mediaSeq    uint64
	lastAccess  time.Time
}

func newHLSSink(codec VideoCodec, vps, sps, pps []byte) *hlsSink {
	if codec == "" {
		codec = CodecH264
	}
	return &hlsSink{codec: codec, vps: vps, sps: sps, pps: pps, lastAccess: time.Now()}
}

// writeAU 喂一个 access unit（AVCC NALU 列表，已剥离参数集），pts 为 90kHz 单位。
func (s *hlsSink) writeAU(pts int64, codec VideoCodec, au [][]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastAccess = time.Now()
	if codec != "" && codec != s.codec {
		// 编码中途切换：extractor/track 全部作废重置，下一个 IDR 重新起段。
		s.codec, s.dtsH264, s.dtsH265 = codec, nil, nil
		s.curBuf, s.curWriter, s.haveSegment = nil, nil, false
	}

	idr := false
	for _, n := range au {
		if codecOfNalu(s.codec, n) == naluIDR {
			idr = true
			break
		}
	}
	if len(au) == 0 {
		return
	}

	// DTS 提取器需要 AU 内自带参数集（H264/H265 均如此：解析 POC 与参考帧集要读
	// SPS/PPS）。故先补参数集再提取，之后同一条 AU 直接写出。
	au = append(s.paramSets(), au...)
	dts, err := s.extractDTS(pts, au, idr)
	if err != nil {
		return
	}

	// 分段边界：遇随机访问帧先收尾上一段、再开新段。
	if idr {
		if s.haveSegment {
			s.finalizeSegment(pts)
		}
		s.startSegment(pts)
	}
	if s.curWriter == nil {
		return
	}
	if s.codec == CodecH265 {
		_ = s.curWriter.WriteH265(s.track, pts, dts, au)
	} else {
		_ = s.curWriter.WriteH264(s.track, pts, dts, au)
	}
}

// paramSets 返回该编码需要的参数集列表（H265: VPS,SPS,PPS；H264: SPS,PPS）。
// 缺任一项返回 nil——半套参数集既不能解码也不该写进段里。
func (s *hlsSink) paramSets() [][]byte {
	if s.codec == CodecH265 {
		if len(s.vps) == 0 || len(s.sps) == 0 || len(s.pps) == 0 {
			return nil
		}
		return [][]byte{s.vps, s.sps, s.pps}
	}
	if len(s.sps) == 0 || len(s.pps) == 0 {
		return nil
	}
	return [][]byte{s.sps, s.pps}
}

// extractDTS 提 DTS；extractor 需在首个随机访问帧初始化，之前静默丢弃。
func (s *hlsSink) extractDTS(pts int64, au [][]byte, idr bool) (int64, error) {
	if s.codec == CodecH265 {
		if s.dtsH265 == nil {
			if !idr {
				return 0, fmt.Errorf("h265 dts extractor not initialized")
			}
			s.dtsH265 = &h265.DTSExtractor{}
			s.dtsH265.Initialize()
		}
		return s.dtsH265.Extract(au, pts)
	}
	if s.dtsH264 == nil {
		if !idr {
			return 0, fmt.Errorf("h264 dts extractor not initialized")
		}
		s.dtsH264 = &h264.DTSExtractor{}
		s.dtsH264.Initialize()
	}
	return s.dtsH264.Extract(au, pts)
}

// startSegment 开新段：新 buffer + 新 Writer，先写 PAT/PMT。
func (s *hlsSink) startSegment(pts int64) {
	s.curBuf = new(bytes.Buffer)
	if s.codec == CodecH265 {
		s.track = &mpegts.Track{Codec: &tscodecs.H265{}}
	} else {
		s.track = &mpegts.Track{Codec: &tscodecs.H264{}}
	}
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
