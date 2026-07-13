package entertainment

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// 翻面夺宝 websocket 桥接：连站点 wss://<host>/wss（带 cookie），读 tileFlip 推送，转发前端。
// 每个前端连接 = 一条上游 WS（等同一个浏览器开着翻面页，正常用量，不像机器人/攻击）。

// FlipEvent 站点 wss 推送的翻牌事件（归一化后转发前端；对齐站点 JS 的 d.type==='tileFlip'）。
type FlipEvent struct {
	Type      string `json:"type"`                 // tileFlip | special
	TileIndex int    `json:"tile_index,omitempty"` // 被翻的格子 1..100
	TierID    string `json:"tier_id,omitempty"`    // 奖项 id
	TierName  string `json:"tier_name,omitempty"`  // 奖项名（站点 d.tier.name 或 d.tierId）
	Player    string `json:"player,omitempty"`     // 谁翻的（playerName）
	EndReason string `json:"end_reason,omitempty"` // special=特等奖出现（新局）
}

// flipBridgeDialer 上游 WS 拨号器（浏览器化 UA/Origin，过站点 CSWSH 校验）。
var flipBridgeDialer = websocket.Dialer{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// flipWSURL baseURL → wss://<host>/wss（https→wss / http→ws，路径替换为 /wss）。
func flipWSURL(baseURL string) string {
	s := strings.TrimSpace(baseURL)
	if i := strings.Index(s, "://"); i >= 0 {
		scheme := "wss"
		if strings.HasPrefix(s, "http://") {
			scheme = "ws"
		}
		s = s[i+3:]
		// 去端口外路径：保留 host[:port]
		if j := strings.IndexByte(s, '/'); j >= 0 {
			s = s[:j]
		}
		return scheme + "://" + s + "/wss"
	}
	return strings.TrimRight(s, "/") + "/wss"
}

// flipWSHeaders 构造浏览器化的握手头（Cookie 鉴权 + Origin + UA）。
func flipWSHeaders(cookie, baseURL string) http.Header {
	h := http.Header{}
	if cookie != "" {
		h.Set("Cookie", cookie)
	}
	// Origin 取 http(s)://host（不带路径）
	origin := strings.TrimSpace(baseURL)
	if i := strings.Index(origin, "://"); i >= 0 {
		rest := origin[i+3:]
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			rest = rest[:j]
		}
		origin = origin[:i+3] + rest
	}
	if origin != "" {
		h.Set("Origin", origin)
	}
	h.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	return h
}

// parseFlipEvents 解析一条 wss 文本消息 → 0..n 个事件。
// 站点 JS：d.type==='tileFlip'（tileIndex/tierId/tier.name/playerName）或 d.endReason==='special'。
func parseFlipEvents(msg []byte) []FlipEvent {
	var d map[string]any
	if err := json.Unmarshal(msg, &d); err != nil {
		return nil
	}
	if strField(d, "type") == "tileFlip" {
		idx := int(longptNum(d["tileIndex"]))
		if idx == 0 {
			return nil // 无效或非翻格消息
		}
		tierID := firstNonEmpty(strField(d, "tierId"))
		tierName := ""
		if t, ok := d["tier"].(map[string]any); ok {
			if tierID == "" {
				tierID = strField(t, "id")
			}
			tierName = strField(t, "name")
		}
		return []FlipEvent{{Type: "tileFlip", TileIndex: idx, TierID: tierID, TierName: tierName, Player: strField(d, "playerName")}}
	}
	if strField(d, "endReason") == "special" {
		return []FlipEvent{{Type: "special", EndReason: "special", Player: strField(d, "playerName")}}
	}
	return nil
}

// pumpFlipBridge 单次连接读循环：读到出错返回（由外层重连）。ctx 取消时关连接。
func pumpFlipBridge(ctx context.Context, baseURL, cookie string, ch chan<- FlipEvent) error {
	conn, _, err := flipBridgeDialer.DialContext(ctx, flipWSURL(baseURL), flipWSHeaders(cookie, baseURL))
	if err != nil {
		return err
	}
	defer conn.Close()
	// ctx 取消 → 强关 conn 使 ReadMessage 解阻塞
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-stop:
		}
	}()
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		for _, ev := range parseFlipEvents(msg) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case ch <- ev:
			}
		}
	}
}

// flipBridgeConns 按 host 去重：同一 host 仅维护一条上游 WS 连接，重复调用复用 channel。
var flipBridgeConns sync.Map // key: flipWSURL(baseURL) → value: chan FlipEvent

// ServeFlipBridge 维护一条到站点 host 的 wss 连接，断开后自动重连（指数退避上限 30s），
// 直到 ctx 取消。返回事件 channel（ctx 取消时关闭）。调用方只需读 channel。
// 按 host 去重：同一 host 重复调用复用已有连接的 channel，避免每次都 go func() 启动新 WS。
func ServeFlipBridge(ctx context.Context, baseURL, cookie string) <-chan FlipEvent {
	key := flipWSURL(baseURL)
	if v, ok := flipBridgeConns.Load(key); ok {
		return v.(chan FlipEvent)
	}
	ch := make(chan FlipEvent, 16)
	if actual, loaded := flipBridgeConns.LoadOrStore(key, ch); loaded {
		// 并发下已有赢家建连，复用其 channel，丢弃本次新建的 ch。
		return actual.(chan FlipEvent)
	}
	go func() {
		defer func() {
			flipBridgeConns.Delete(key)
			close(ch)
		}()
		backoff := 2 * time.Second
		for {
			if ctx.Err() != nil {
				return
			}
			err := pumpFlipBridge(ctx, baseURL, cookie, ch)
			if ctx.Err() != nil {
				return
			}
			if err != nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				backoff *= 2
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
			}
		}
	}()
	return ch
}
