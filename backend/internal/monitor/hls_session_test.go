package monitor

import (
	"strings"
	"testing"
)

// SPS/PPS/IDR/non-IDR 取自 mediacommon 官方 DTSExtractor 测试向量（Log2MaxPicOrderCntLsb=12），
// 保证 DTSExtractor.Extract 不报错，从而真正走到 mpegts 封装路径。
var (
	hlsTestSPS    = []byte{0x67, 0x42, 0xc0, 0x1e, 0x8c, 0x8d, 0x40, 0x50, 0x17, 0xfc, 0xb0, 0x0f, 0x08, 0x84, 0x6a}
	hlsTestPPS    = []byte{0x68, 0xce, 0x3c, 0x80}
	hlsTestIDR    = []byte{0x65, 0x88, 0x80, 0x14, 0x3, 0x00, 0x00, 0x00, 0x00, 0x00}
	hlsTestNonIDR = []byte{0x61, 0x00, 0xf0, 0xe0, 0x00, 0x40, 0x00, 0xbe, 0x47, 0x9b}
)

// TestHLSSinkMuxAndPlaylist 验证：合法 H264 喂入 → 产出正确 m3u8 + 结构合法的 TS 段。
func TestHLSSinkMuxAndPlaylist(t *testing.T) {
	sink := newHLSSink(hlsTestSPS, hlsTestPPS)
	sink.writeAU(0, [][]byte{hlsTestIDR})      // 首帧 IDR：初始化 extractor + 开 seg0
	sink.writeAU(3600, [][]byte{hlsTestNonIDR}) // non-IDR 入 seg0
	sink.writeAU(7200, [][]byte{hlsTestIDR})    // IDR：收尾 seg0、开 seg1
	sink.writeAU(10800, [][]byte{hlsTestNonIDR}) // non-IDR 入 seg1
	sink.writeAU(14400, [][]byte{hlsTestIDR})    // IDR：收尾 seg1、开 seg2

	pl := sink.playlist()
	switch {
	case !strings.Contains(pl, "#EXTM3U"):
		t.Fatalf("playlist missing EXTM3U:\n%s", pl)
	case !strings.Contains(pl, "#EXT-X-MEDIA-SEQUENCE:0"):
		t.Fatalf("playlist missing MEDIA-SEQUENCE:0:\n%s", pl)
	case !strings.Contains(pl, "segment/0.ts") || !strings.Contains(pl, "segment/1.ts"):
		t.Fatalf("playlist missing segment entries:\n%s", pl)
	case !strings.Contains(pl, "#EXT-X-TARGETDURATION:"):
		t.Fatalf("playlist missing TARGETDURATION:\n%s", pl)
	}

	// seg0 的 TS 字节：188 对齐、sync 0x47、首包 PID=0（PAT，段可独立解码）。
	data := sink.segment(0)
	if data == nil {
		t.Fatal("segment 0 nil")
	}
	if len(data)%188 != 0 || len(data) < 188*2 { // 至少 PAT+PMT+一个 PES 包
		t.Fatalf("segment not 188-aligned / too short: len=%d", len(data))
	}
	for i := 0; i < len(data); i += 188 {
		if data[i] != 0x47 {
			t.Fatalf("packet at %d missing sync 0x47: %x", i, data[i])
		}
	}
	pid := (uint16(data[1]&0x1F) << 8) | uint16(data[2])
	if pid != 0 {
		t.Fatalf("first packet PID=%d, want 0 (PAT present)", pid)
	}
}

// TestHLSSinkRingEviction 验证 ring 滑窗：喂超量 IDR 只留最近 hlsMaxSegs 段、序列号前推、旧段淘汰。
func TestHLSSinkRingEviction(t *testing.T) {
	sink := newHLSSink(hlsTestSPS, hlsTestPPS)
	var pts int64
	for i := 0; i < 10; i++ { // 每个非首 IDR 收尾上一段
		pts += 90000
		sink.writeAU(pts, [][]byte{hlsTestIDR})
	}
	pl := sink.playlist()
	if c := strings.Count(pl, "#EXTINF"); c != hlsMaxSegs {
		t.Fatalf("ring not capped at %d: EXTINF count=%d\n%s", hlsMaxSegs, c, pl)
	}
	if !strings.Contains(pl, "segment/8.ts") { // 最新已收尾段
		t.Fatalf("newest segment missing:\n%s", pl)
	}
	if strings.Contains(pl, "segment/0.ts") { // 旧段应已淘汰
		t.Fatalf("oldest segment not evicted:\n%s", pl)
	}
}
