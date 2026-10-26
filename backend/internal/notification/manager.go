package notification

import (
	"context"
	"errors"
	"fmt"
	"github.com/hxpt/ptpatronus/internal/core/safego"
	"github.com/hxpt/ptpatronus/internal/errtext"
	"log/slog"
	"net/url"
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

// sanitizeNotifyErr 脱敏通知发送错误：*url.Error 的 URL 常含 bot token / 密钥
// （telegram/gotify/serverchan/bark/apprise 把凭据放 URL path 或 query），
// 仅保留 host + 底层错误，避免明文进日志 / 返回前端。
// sanitizeNotifyLogErr 给日志用：URL 凭据同样脱敏，但**保留底层原文**（no such host /
// connection refused 是排障唯一线索）。界面结果走 sanitizeNotifyErr 翻成中文，
// 两者不能合成一个——合成就会把原文从日志里一起抹掉（TestNoticeLogsUnrecoverableExternalFailure）。
func sanitizeNotifyLogErr(err error) string {
	if err == nil {
		return ""
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		host := ue.URL
		if u, perr := url.Parse(ue.URL); perr == nil {
			host = u.Host
		}
		return fmt.Sprintf("请求 %s 失败: %v", host, ue.Err)
	}
	return err.Error()
}

func sanitizeNotifyErr(err error) string {
	if err == nil {
		return ""
	}
	// *url.Error 的 URL 常含 bot token / 密钥：先只保留 host，再把底层错误交给
	// errtext 单一来源翻译（连不上/超时等给可操作中文；上游自产文案原样透传）。
	// 用 errors.As 是为了覆盖被 fmt.Errorf("...: %w") 包过一层的调用点。
	var ue *url.Error
	if errors.As(err, &ue) {
		host := ue.URL
		if u, perr := url.Parse(ue.URL); perr == nil {
			host = u.Host
		}
		return fmt.Sprintf("请求 %s 失败：%s", host, errtext.Text(ue.Err))
	}
	return errtext.Text(err)
}

// Notify 向所有通道并发发送，返回每个通道的结果。
func (m *Manager) Notify(ctx context.Context, msg Message) []SendResult {
	m.mu.RLock()
	chs := make([]Channel, len(m.channels))
	copy(chs, m.channels)
	m.mu.RUnlock()
	return notifyChannels(ctx, chs, msg)
}

// NotifyChannels 仅向指定名称的通道发送（失败重试专用：只打失败渠道，
// 避免已成功渠道重复收到同一条推送）。names 里的未知渠道名静默忽略。
func (m *Manager) NotifyChannels(ctx context.Context, names []string, msg Message) []SendResult {
	if len(names) == 0 {
		return nil
	}
	keep := make(map[string]bool, len(names))
	for _, n := range names {
		keep[n] = true
	}
	m.mu.RLock()
	chs := make([]Channel, 0, len(names))
	for _, c := range m.channels {
		if keep[c.Name()] {
			chs = append(chs, c)
		}
	}
	m.mu.RUnlock()
	return notifyChannels(ctx, chs, msg)
}

func notifyChannels(ctx context.Context, chs []Channel, msg Message) []SendResult {
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
			// 循环内单项隔离（同 chain/subtitle 的每源 recover）：崩掉的渠道必须自己留下一行失败结果。
			// 从前 safego 在更外层 recover ⇒ 这个 goroutine 一崩，results[i] 从没被写过，槽位停在零值
			// （渠道名与原因都空）：面板上是一行「无名 + 无原因」的失败，而 notice.Service 拿空名字去重试
			// 匹配不到任何渠道 ⇒ 崩过的渠道永远不会被重发，#225 那条「重试仍失败」的终局也不会响。
			defer func() {
				if p := recover(); p != nil {
					r.Error = fmt.Sprintf("渠道异常: %v", p)
					results[i] = r
					slog.Warn("notify channel panicked", "channel", c.Name(), "err", p)
				}
			}()
			if err := c.Send(ctx, msg); err != nil {
				r.Error = sanitizeNotifyErr(err)
				slog.Warn("notify failed", "channel", c.Name(),
					"err", sanitizeNotifyLogErr(err))
			} else {
				r.Ok = true
			}
			results[i] = r
		})
	}
	wg.Wait()
	return results
}
