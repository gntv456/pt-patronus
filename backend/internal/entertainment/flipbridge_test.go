package entertainment

import (
	"context"
	"testing"
	"time"
)

// TestParseFlipEvents 站点 wss 消息 → FlipEvent 归一化（tileFlip / special）。
func TestParseFlipEvents(t *testing.T) {
	evs := parseFlipEvents([]byte(`{"type":"tileFlip","tileIndex":18,"tierId":"c","tier":{"id":"c","name":"二等奖"},"playerName":"typhoonspy"}`))
	if len(evs) != 1 || evs[0].Type != "tileFlip" || evs[0].TileIndex != 18 || evs[0].TierID != "c" || evs[0].TierName != "二等奖" || evs[0].Player != "typhoonspy" {
		t.Errorf("tileFlip parse = %+v", evs)
	}
	// tierId 缺失时从 tier.id 兜底
	evs = parseFlipEvents([]byte(`{"type":"tileFlip","tileIndex":3,"tier":{"id":"d","name":"三等奖"}}`))
	if len(evs) != 1 || evs[0].TierID != "d" || evs[0].TierName != "三等奖" {
		t.Errorf("tier fallback = %+v", evs)
	}
	// 特等奖：endReason=special
	evs = parseFlipEvents([]byte(`{"endReason":"special","playerName":"someone"}`))
	if len(evs) != 1 || evs[0].Type != "special" || evs[0].EndReason != "special" {
		t.Errorf("special parse = %+v", evs)
	}
	// 非 tileFlip 无 tileIndex → 忽略；无 type → 忽略
	if parseFlipEvents([]byte(`{"type":"tileFlip","playerName":"x"}`)) != nil {
		t.Error("tileFlip without tileIndex should be ignored")
	}
	if parseFlipEvents([]byte(`{"foo":"bar"}`)) != nil {
		t.Error("irrelevant message should be ignored")
	}
}

// TestFlipWSURL https/http → wss/ws + /wss 路径。
func TestFlipWSURL(t *testing.T) {
	cases := map[string]string{
		"https://pandapt.net":          "wss://pandapt.net/wss",
		"https://pandapt.net/flip.php": "wss://pandapt.net/wss",
		"http://test.pandapt.net":      "ws://test.pandapt.net/wss",
		"https://pandapt.net:8443/x":   "wss://pandapt.net:8443/wss",
	}
	for in, want := range cases {
		if got := flipWSURL(in); got != want {
			t.Errorf("flipWSURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestServeFlipBridgeCancel ctx 取消 → channel 关闭（不连真实站点，仅验生命周期）。
func TestServeFlipBridgeCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := ServeFlipBridge(ctx, "https://invalid-host-does-not-exist.invalid", "")
	cancel()
	// ctx 取消后 channel 应关闭（即使拨号失败也在重连循环，cancel 必须能打断）
	select {
	case _, ok := <-ch:
		if ok {
			// 可能先收到关闭：等 drain
		}
	case <-time.After(2 * time.Second):
		t.Error("channel not closed after ctx cancel within 2s")
	}
}
