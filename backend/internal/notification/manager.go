package notification

import (
	"context"
	"log/slog"
	"sync"
)

// Manager 管理多个通知通道，Notify 并发扇出、错误隔离（单通道失败不影响其它）。
type Manager struct {
	mu       sync.RWMutex
	channels []Channel
}

// NewManager 构造。
func NewManager(channels ...Channel) *Manager {
	return &Manager{channels: channels}
}

// Add 追加通道。
func (m *Manager) Add(c Channel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels = append(m.channels, c)
}

// Replace 用新通道切片整体替换已注册通道（写锁）。
// 供 NotificationSettingsProvider 在 ApplySettings/rebuild 后热替换；
// Notify 在读锁下 copy 切片，热替换天然安全。
func (m *Manager) Replace(channels []Channel) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels = channels
}

// Channels 返回通道名列表。
func (m *Manager) Channels() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, 0, len(m.channels))
	for _, c := range m.channels {
		out = append(out, c.Name())
	}
	return out
}

// SendResult 是单通道发送结果。
type SendResult struct {
	Channel string `json:"channel"`
	Ok      bool   `json:"ok"`
	Error   string `json:"error"`
}

// Notify 向所有通道并发发送，返回每个通道的结果。
func (m *Manager) Notify(ctx context.Context, msg Message) []SendResult {
	m.mu.RLock()
	chs := make([]Channel, len(m.channels))
	copy(chs, m.channels)
	m.mu.RUnlock()

	if len(chs) == 0 {
		return nil
	}
	results := make([]SendResult, len(chs))
	var wg sync.WaitGroup
	for i, c := range chs {
		wg.Add(1)
		go func(i int, c Channel) {
			defer wg.Done()
			r := SendResult{Channel: c.Name()}
			if err := c.Send(ctx, msg); err != nil {
				r.Error = err.Error()
				slog.Warn("notify failed", "channel", c.Name(), "err", err)
			} else {
				r.Ok = true
			}
			results[i] = r
		}(i, c)
	}
	wg.Wait()
	return results
}
