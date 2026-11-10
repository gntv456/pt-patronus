// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"fmt"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph264"
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph265"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/pion/rtp"
)

// VideoCodec 表示一路 RTSP 会话选中的视频编码。抽象掉 H264/H265 差异：
// ProxySession（WS 广播）与 hlsSink（MPEG-TS 封装）只面向本接口，
// 新增编码只需在此追加一个分支，不动上游的轨道选择与下游的封装逻辑。
type VideoCodec string

const (
	CodecH264 VideoCodec = "h264"
	CodecH265 VideoCodec = "h265"
)

// rtpDecoder 是 H264/H265 RTP 拆包器的共同外观（两者 Decode 签名一致）。
type rtpDecoder interface {
	Decode(pkt *rtp.Packet) ([][]byte, error)
}

// videoTrack 是从 SDP 中选中的视频轨 + 其参数集 + RTP 拆包器。
type videoTrack struct {
	codec VideoCodec
	media *description.Media
	forma format.Format
	dec   rtpDecoder

	vps []byte // 仅 H265；H264 为 nil
	sps []byte
	pps []byte
}

// annexBParams 把参数集拼成 annexB 串（H265 为 VPS,SPS,PPS；H264 为 SPS,PPS）。
// 参数集缺失时返回 nil——调用方据此判定「尚不可播」，不硬凑半套。
func (t *videoTrack) annexBParams() []byte {
	if len(t.sps) == 0 || len(t.pps) == 0 {
		return nil
	}
	if t.codec == CodecH265 && len(t.vps) > 0 {
		return makeAnnexB([][]byte{t.vps, t.sps, t.pps})
	}
	return makeAnnexB([][]byte{t.sps, t.pps})
}

// pickVideoTrack 在 SDP 媒体描述中选一路可解码的视频轨：H264 优先，其次 H265。
//
// 选 H264 优先的理由：既有摄像头与前端 jessibuca 默认解码器对 H264 支持最稳；
// 同一路 RTSP 同时提供两种编码（少数厂商的主/子码流）时，应优先落在 H264。
// H265 是「本来完全拉不到流」的兜底——小米经 Miloco/Micam 转推默认输出 hevc。
func pickVideoTrack(desc *description.Session) (*videoTrack, error) {
	var h264M *description.Media
	var h264F *format.H264
	var h265M *description.Media
	var h265F *format.H265
	for _, m := range desc.Medias {
		for _, f := range m.Formats {
			switch v := f.(type) {
			case *format.H264:
				if h264F == nil {
					h264M, h264F = m, v
				}
			case *format.H265:
				if h265F == nil {
					h265M, h265F = m, v
				}
			}
		}
	}
	switch {
	case h264F != nil:
		d, err := h264F.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return &videoTrack{
			codec: CodecH264, media: h264M, forma: h264F, dec: d,
			sps: h264F.SPS, pps: h264F.PPS,
		}, nil
	case h265F != nil:
		d, err := h265F.CreateDecoder()
		if err != nil {
			return nil, err
		}
		return &videoTrack{
			codec: CodecH265, media: h265M, forma: h265F, dec: d,
			vps: h265F.VPS, sps: h265F.SPS, pps: h265F.PPS,
		}, nil
	}
	return nil, fmt.Errorf("no h264/h265 video track in stream")
}

// naluKind 是一个 NALU 在码流中的角色归类。
type naluKind int

const (
	naluSkip     naluKind = iota // AUD 等可丢弃 NAL
	naluParamSet                 // VPS/SPS/PPS：缓存参数，剥离出码流
	naluIDR                      // 随机访问帧（HLS 分段边界）
	naluNonIDR                   // 普通帧
	naluOther                    // SEI 等随帧保留但不参与判断
)

// codecOfNalu 归类单个 NALU。两个编码的 NAL header 位宽不同
// （H264 取低 5 位、H265 取高 6 位），故按编码分别判断。
func codecOfNalu(codec VideoCodec, n []byte) naluKind {
	if codec == CodecH265 {
		if len(n) < 2 {
			return naluSkip
		}
		switch h265.NALUType((n[0] >> 1) & 0b111111) {
		case h265.NALUType_VPS_NUT, h265.NALUType_SPS_NUT, h265.NALUType_PPS_NUT:
			return naluParamSet
		case h265.NALUType_AUD_NUT:
			return naluSkip
		case h265.NALUType_EOS_NUT, h265.NALUType_EOB_NUT, h265.NALUType_FD_NUT:
			return naluSkip
		}
		// 随机访问点判定交给 mediacommon 的权威实现（含 BLA/IDR/CRA 及保留 IRAP）。
		if h265.IsRandomAccess([][]byte{n}) {
			return naluIDR
		}
		return naluNonIDR
	}
	if len(n) < 1 {
		return naluSkip
	}
	switch h264.NALUType(n[0] & 0x1F) {
	case h264.NALUTypeSPS, h264.NALUTypePPS:
		return naluParamSet
	case h264.NALUTypeAccessUnitDelimiter:
		return naluSkip
	case h264.NALUTypeIDR:
		return naluIDR
	case h264.NALUTypeNonIDR:
		return naluNonIDR
	default:
		return naluOther // SEI / filler 等：随帧保留，但不参与 IDR 判断
	}
}

// splitAccessUnits 把一个解码后的 NALU 序列拆成「参数集」与「帧」两路回调。
//
// 参数集（SPS/PPS/VPS）不进码流，而是回调 onParams 交调用方缓存——它们属于整个
// access unit 而非某一帧，新观众补发与 HLS 段头都要用。onFrame 的 au 因此是
// 剥离参数集后的帧 NALU 列表；idr 表示该帧含随机访问点。
func splitAccessUnits(codec VideoCodec, nalus [][]byte,
	onParams func(vps, sps, pps []byte),
	onFrame func(au [][]byte, idr bool)) {
	var vps, sps, pps []byte
	var kept [][]byte
	idr := false
	for _, n := range nalus {
		switch codecOfNalu(codec, n) {
		case naluSkip:
			continue
		case naluParamSet:
			switch codec {
			case CodecH265:
				switch h265.NALUType((n[0] >> 1) & 0b111111) {
				case h265.NALUType_VPS_NUT:
					vps = n
				case h265.NALUType_SPS_NUT:
					sps = n
				case h265.NALUType_PPS_NUT:
					pps = n
				}
			default:
				if h264.NALUType(n[0]&0x1F) == h264.NALUTypeSPS {
					sps = n
				} else {
					pps = n
				}
			}
			continue
		case naluIDR:
			idr = true
		}
		kept = append(kept, n)
	}
	if onParams != nil && (vps != nil || sps != nil || pps != nil) {
		onParams(vps, sps, pps)
	}
	if onFrame != nil && len(kept) > 0 {
		onFrame(kept, idr)
	}
}

// paramsAnnexB 按编码把参数集拼成 annexB 串（H265 要求 VPS 先于 SPS）。
func paramsAnnexB(codec VideoCodec, vps, sps, pps []byte) []byte {
	if len(sps) == 0 || len(pps) == 0 {
		return nil
	}
	if codec == CodecH265 && len(vps) > 0 {
		return makeAnnexB([][]byte{vps, sps, pps})
	}
	return makeAnnexB([][]byte{sps, pps})
}

// compile-time 断言：两个解码器都满足 rtpDecoder。
var (
	_ rtpDecoder = (*rtph264.Decoder)(nil)
	_ rtpDecoder = (*rtph265.Decoder)(nil)
)
