package plugins

import (
	"context"
	"fmt"
	"time"

	"github.com/hxpt/ptpatronus/internal/notice"
)

type pluginRuntime struct {
	manager  *Manager
	pluginID string
	config   map[string]any
}

func (r *pluginRuntime) Config() map[string]any {
	out := make(map[string]any, len(r.config))
	for k, v := range r.config {
		out[k] = v
	}
	return out
}

func (r *pluginRuntime) Log(level, eventName, message string, data map[string]any) {
	r.manager.log(r.pluginID, level, eventName, message, data)
}

func (r *pluginRuntime) Notice(ctx context.Context, in NoticeInput) error {
	if r.manager.notice == nil {
		return nil
	}
	return r.manager.notice.Notice(ctx, notice.Input{
		Source: "plugin",
		Key:    fmt.Sprintf("%s:%d", r.pluginID, time.Now().UnixNano()),
		Title:  in.Title,
		Body:   in.Body,
		Level:  in.Level,
	}, false)
}

func (r *pluginRuntime) Publish(ctx context.Context, eventType string, data map[string]any) {
	r.manager.PublishEvent(ctx, eventType, data)
}
