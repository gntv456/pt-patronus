// Package notification 实现通知通道抽象与多通道管理。
// 通道：Bark / Telegram / Webhook。Manager 并发扇出、错误隔离。
package notification

import "context"

// Message 是统一通知负载。
type Message struct {
	Title string         `json:"title"`
	Body  string         `json:"body"`
	Level string         `json:"level"` // info | warning | danger | celebration
	Data  map[string]any `json:"data,omitempty"`
}

// Channel 是一个通知通道。
type Channel interface {
	Name() string
	Send(ctx context.Context, msg Message) error
}
