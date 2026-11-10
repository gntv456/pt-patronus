// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"strings"
	"testing"

	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
)

// H265 参数集/帧 NAL 取自 mediacommon 官方 h265 DTSExtractor 测试向量
// （"with timing info, IDR" 用例），保证 h265.DTSExtractor.Extract 不报错，
// 从而真正走到 WriteH265 封装路径——与 hls_session_test.go 的 H264 取向量同法。
var (
	h265TestVPS = []byte{
		0x40, 0x01, 0x0c, 0x01, 0xff, 0xff, 0x01, 0x60,
		0x00, 0x00, 0x03, 0x00, 0x90, 0x00, 0x00, 0x03,
		0x00, 0x00, 0x03, 0x00, 0x78, 0x99, 0x98, 0x09,
	}
	h265TestSPS = []byte{
		0x42, 0x01, 0x01, 0x01, 0x60, 0x00, 0x00, 0x03,
		0x00, 0x90, 0x00, 0x00, 0x03, 0x00, 0x00, 0x03,
		0x00, 0x78, 0xa0, 0x03, 0xc0, 0x80, 0x10, 0xe5,
		0x96, 0x66, 0x69, 0x24, 0xca, 0xe0, 0x10, 0x00,
		0x00, 0x03, 0x00, 0x10, 0x00, 0x00, 0x03, 0x01,
		0xe0, 0x80,
	}
	h265TestPPS = []byte{0x44, 0x1, 0xc1, 0x72, 0xb4, 0x62, 0x40}
	h265TestIDR = []byte{
		0x26, 0x1, 0xaf, 0x8, 0x42, 0x23, 0x48, 0x8a, 0x43, 0xe2,
	}
	h265TestNonIDR = []byte{
		0x02, 0x01, 0xd0, 0x19, 0x5f, 0x8c, 0xb4, 0x42,
		0x49, 0x20, 0x40, 0x11, 0x16, 0x92, 0x93, 0xea,
		0x54, 0x57, 0x4e, 0x0a,
	}
)

// TestHLSSinkH265MuxAndPlaylist 验证 H265 全链路：合法 HEVC AU 喂入 → m3u8 + 结构合法的 TS 段。
// 这是小米（Miloco/Micam 默认 hevc）能出画的关键回归。
func TestHLSSinkH265MuxAndPlaylist(t *testing.T) {
	sink := newHLSSink(CodecH265, h265TestVPS, h265TestSPS, h265TestPPS)
	sink.writeAU(0, CodecH265, [][]byte{h265TestIDR})
	sink.writeAU(9000, CodecH265, [][]byte{h265TestNonIDR})
	sink.writeAU(18000, CodecH265, [][]byte{h265TestIDR})
	sink.writeAU(27000, CodecH265, [][]byte{h265TestNonIDR})
	sink.writeAU(36000, CodecH265, [][]byte{h265TestIDR})

	pl := sink.playlist()
	switch {
	case !strings.Contains(pl, "#EXTM3U"):
		t.Fatalf("playlist missing EXTM3U:\n%s", pl)
	case !strings.Contains(pl, "segment/1.ts"):
		t.Fatalf("playlist missing segment entries:\n%s", pl)
	}

	data := sink.segment(0)
	if data == nil {
		t.Fatal("segment 0 nil")
	}
	if len(data)%188 != 0 || len(data) < 188*2 {
		t.Fatalf("segment not 188-aligned / too short: len=%d", len(data))
	}
	for i := 0; i < len(data); i += 188 {
		if data[i] != 0x47 {
			t.Fatalf("packet at %d missing sync 0x47: %x", i, data[i])
		}
	}
	if pid := (uint16(data[1]&0x1F) << 8) | uint16(data[2]); pid != 0 {
		t.Fatalf("first packet PID=%d, want 0 (PAT present)", pid)
	}
}

// TestCodecOfNaluH265 验证 H265 的 NAL header 位宽（高 6 位）解对了，
// 且 VPS/SPS/PPS 被判为参数集、IDR_W_RADL 与 CRA 都被判为随机访问帧。
func TestCodecOfNaluH265(t *testing.T) {
	cases := []struct {
		name string
		nalu []byte
		want naluKind
	}{
		{"VPS", h265TestVPS, naluParamSet},
		{"SPS", h265TestSPS, naluParamSet},
		{"PPS", h265TestPPS, naluParamSet},
		{"IDR_W_RADL", h265TestIDR, naluIDR},
		{"TRAIL_R", h265TestNonIDR, naluNonIDR},
		// CRA（type 21）同样是随机访问点，且是 H265 独立于 IDR 的关键帧形式。
		{"CRA", []byte{21 << 1, 0x01}, naluIDR},
		{"AUD", []byte{35 << 1, 0x01}, naluSkip},
	}
	for _, tc := range cases {
		if got := codecOfNalu(CodecH265, tc.nalu); got != tc.want {
			t.Errorf("%s: codecOfNalu = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestCodecOfNaluH264KeepsBehavior 验证抽象没有改变 H264 原有归类（防回归）。
func TestCodecOfNaluH264KeepsBehavior(t *testing.T) {
	cases := []struct {
		name string
		nalu []byte
		want naluKind
	}{
		{"SPS", hlsTestSPS, naluParamSet},
		{"PPS", hlsTestPPS, naluParamSet},
		{"IDR", hlsTestIDR, naluIDR},
		{"nonIDR", hlsTestNonIDR, naluNonIDR},
		{"AUD", []byte{byte(h264.NALUTypeAccessUnitDelimiter), 0xf0}, naluSkip},
		{"SEI", []byte{byte(h264.NALUTypeSEI), 0x01}, naluOther},
	}
	for _, tc := range cases {
		if got := codecOfNalu(CodecH264, tc.nalu); got != tc.want {
			t.Errorf("%s: codecOfNalu = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestSplitAccessUnits 参数集剥离：H265 的 VPS/SPS/PPS 与 H264 的 SPS/PPS
// 都不应混进帧 NAL 列表（否则每帧都带一份参数，浪费带宽且 HLS 段重复）。
func TestSplitAccessUnits(t *testing.T) {
	t.Run("h265", func(t *testing.T) {
		var gotVPS, gotSPS, gotPPS []byte
		var frame [][]byte
		var idr bool
		splitAccessUnits(CodecH265,
			[][]byte{h265TestVPS, h265TestSPS, h265TestPPS, h265TestIDR, h265TestNonIDR},
			func(vps, sps, pps []byte) { gotVPS, gotSPS, gotPPS = vps, sps, pps },
			func(au [][]byte, i bool) { frame, idr = au, i })
		if !bytesEqual(gotVPS, h265TestVPS) || !bytesEqual(gotSPS, h265TestSPS) || !bytesEqual(gotPPS, h265TestPPS) {
			t.Errorf("param sets misrouted: vps=%x sps=%x pps=%x", gotVPS, gotSPS, gotPPS)
		}
		if !idr {
			t.Error("IDR frame not flagged")
		}
		if len(frame) != 2 {
			t.Fatalf("frame NAL count = %d, want 2 (param sets must be stripped): %d NALs", len(frame), len(frame))
		}
	})

	t.Run("h264", func(t *testing.T) {
		var gotSPS, gotPPS []byte
		var frame [][]byte
		splitAccessUnits(CodecH264,
			[][]byte{hlsTestSPS, hlsTestPPS, hlsTestIDR},
			func(_, sps, pps []byte) { gotSPS, gotPPS = sps, pps },
			func(au [][]byte, _ bool) { frame = au })
		if !bytesEqual(gotSPS, hlsTestSPS) || !bytesEqual(gotPPS, hlsTestPPS) {
			t.Error("param sets misrouted")
		}
		if len(frame) != 1 {
			t.Fatalf("frame NAL count = %d, want 1", len(frame))
		}
	})
}

// TestParamsAnnexB H265 必须 VPS→SPS→PPS 顺序（缺 VPS 的 H265 解码器起不来）；
// H264 只有 SPS/PPS。参数缺失时返回 nil，不拼半套。
func TestParamsAnnexB(t *testing.T) {
	h265Out := paramsAnnexB(CodecH265, h265TestVPS, h265TestSPS, h265TestPPS)
	if len(h265Out) == 0 {
		t.Fatal("h265 annexB empty")
	}
	if idx := indexOf(h265Out, h265TestVPS); idx < 0 ||
		idx > indexOf(h265Out, h265TestSPS) {
		t.Errorf("VPS must precede SPS: vps@%d sps@%d", indexOf(h265Out, h265TestVPS), indexOf(h265Out, h265TestSPS))
	}
	h264Out := paramsAnnexB(CodecH264, nil, hlsTestSPS, hlsTestPPS)
	if len(h264Out) == 0 {
		t.Fatal("h264 annexB empty")
	}
	if paramsAnnexB(CodecH265, h265TestVPS, h265TestSPS, nil) != nil {
		t.Error("missing PPS must yield nil, not a half-built annexB")
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func indexOf(hay, needle []byte) int {
	return strings.Index(string(hay), string(needle))
}
