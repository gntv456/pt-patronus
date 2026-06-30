// Package ws 提供实时推送的 WebSocket 基建：Hub 维护连接池并广播消息。
package ws

import "sync"

// Hub 维护活跃 WS 连接，向它们广播消息。零连接时广播为空操作。
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
}

// NewHub 构造 Hub。
func NewHub() *Hub {
	return &Hub{clients: map[*Client]struct{}{}}
}

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send) // 通知 writePump 退出
	}
	h.mu.Unlock()
}

// Broadcast 向所有连接广播一条消息（非阻塞：某连接发不出去则跳过，
// 靠 ping 超时兜底关闭僵死连接，下个 tick 自然补齐）。
func (h *Hub) Broadcast(payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- payload:
		default:
		}
	}
}

// Count 返回当前连接数（观测/「无连接则跳过聚合」优化用）。
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
