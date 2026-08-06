package notification

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"github.com/hxpt/ptpatronus/internal/core/safego"
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

// sanitizeNotifyErr 脱敏通知发送错误：*url.Error 的 URL 常含 bot token / 密钥
// （telegram/gotify/serverchan/bark/apprise 把凭据放 URL path 或 query），
// 仅保留 host + 底层错误，避免明文进日志 / 返回前端。
func sanitizeNotifyErr(err error) string {
	if err == nil {
		return ""
	}
	if ue, ok := err.(*url.Error); ok {
		host := ue.URL
		if u, perr := url.Parse(ue.URL); perr == nil {
			host = u.Host
		}
		return fmt.Sprintf("请求 %s 失败: %v", host, ue.Err)
	}
	return err.Error()
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
		safego.Go("manager:87", func() {
			i, c := i, c // capture loop vars
			defer wg.Done()
			r := SendResult{Channel: c.Name()}
			if err := c.Send(ctx, msg); err != nil {
				safe := sanitizeNotifyErr(err)
				r.Error = safe
				slog.Warn("notify failed", "channel", c.Name(), "err", safe)
			} else {
				r.Ok = true
			}
			results[i] = r
		})
	}
	wg.Wait()
	return results
}
