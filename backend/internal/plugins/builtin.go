package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type eventAuditPlugin struct{}

func NewEventAuditPlugin() Plugin { return eventAuditPlugin{} }

func (eventAuditPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "event-audit",
		Name:        "事件审计",
		Version:     "1.0.0",
		Author:      "PTPatronus",
		Description: "记录插件事件总线中的关键事件，便于排查 Webhook、测试事件和后续业务事件。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"event:read", "log:write"},
		Events: []EventSpec{
			{Type: "plugin.test", Description: "插件系统测试事件"},
			{Type: "webhook", Description: "进站 Webhook 事件"},
		},
		Actions: []ActionSpec{
			{
				Name:        "echo",
				Label:       "回显输入",
				Description: "返回提交的 JSON，用于验证动作调用链路。",
				InputSchema: []ConfigField{{Key: "message", Label: "消息", Type: "text"}},
			},
			{
				Name:        "emit_test_event",
				Label:       "发出测试事件",
				Description: "向事件总线发布 plugin.test。",
				InputSchema: []ConfigField{{Key: "message", Label: "消息", Type: "text"}},
			},
		},
		ConfigSchema: []ConfigField{
			{Key: "include_payload", Label: "记录事件数据", Type: "switch", Default: true},
		},
	}
}

func (eventAuditPlugin) DefaultConfig() map[string]any {
	return map[string]any{"include_payload": true}
}

func (eventAuditPlugin) HandleEvent(ctx context.Context, rt Runtime, e Event) error {
	cfg := rt.Config()
	data := map[string]any(nil)
	if asBool(cfg["include_payload"]) {
		data = e.Data
	}
	rt.Log(logInfo, e.Type, "audit event received", data)
	return nil
}

func (eventAuditPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	switch action {
	case "echo":
		return map[string]any{"input": input, "at": time.Now()}, nil
	case "emit_test_event":
		data := map[string]any{
			"source":  "event-audit",
			"message": fmt.Sprint(input["message"]),
			"at":      time.Now().Format(time.RFC3339),
		}
		rt.Publish(ctx, "plugin.test", data)
		return map[string]any{"published": true, "event_type": "plugin.test", "data": data}, nil
	default:
		return nil, unknownActionErr(action)
	}
}

type noticeForwarderPlugin struct{}

func NewNoticeForwarderPlugin() Plugin { return noticeForwarderPlugin{} }

func (noticeForwarderPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "notice-forwarder",
		Name:        "事件通知桥",
		Version:     "1.0.0",
		Author:      "PTPatronus",
		Description: "将指定事件转成站内通知，并复用系统通知渠道外发。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"event:read", "notice:write"},
		Events: []EventSpec{
			{Type: "plugin.test", Description: "插件系统测试事件"},
			{Type: "webhook", Description: "进站 Webhook 事件"},
		},
		Actions: []ActionSpec{
			{
				Name:        "send_test_notice",
				Label:       "发送测试通知",
				Description: "通过全局通知中心发送一条插件测试通知。",
				InputSchema: []ConfigField{
					{Key: "title", Label: "标题", Type: "text", Default: "插件测试通知"},
					{Key: "body", Label: "正文", Type: "textarea", Default: "事件通知桥工作正常。"},
				},
			},
		},
		ConfigSchema: []ConfigField{
			{Key: "event_types", Label: "监听事件", Type: "textarea", Default: "plugin.test", Help: "逗号或换行分隔，留空表示所有已声明事件。"},
			{Key: "title_prefix", Label: "标题前缀", Type: "text", Default: "[插件]"},
			{Key: "level", Label: "通知级别", Type: "select", Default: "info", Options: []ConfigOption{
				{Label: "信息", Value: "info"},
				{Label: "成功", Value: "success"},
				{Label: "警告", Value: "warning"},
				{Label: "危险", Value: "danger"},
			}},
			{Key: "include_payload", Label: "附带事件数据", Type: "switch", Default: false},
		},
	}
}

func (noticeForwarderPlugin) DefaultConfig() map[string]any {
	return map[string]any{
		"event_types":     "plugin.test",
		"title_prefix":    "[插件]",
		"level":           "info",
		"include_payload": false,
	}
}

func (noticeForwarderPlugin) HandleEvent(ctx context.Context, rt Runtime, e Event) error {
	cfg := rt.Config()
	if !eventAllowed(fmt.Sprint(cfg["event_types"]), e.Type) {
		return nil
	}
	body := "事件已触发：" + e.Type
	if asBool(cfg["include_payload"]) && len(e.Data) > 0 {
		if raw, err := json.MarshalIndent(e.Data, "", "  "); err == nil {
			body += "\n" + truncate(string(raw), 2000)
		}
	}
	return rt.Notice(ctx, NoticeInput{
		Title: strings.TrimSpace(fmt.Sprint(cfg["title_prefix"])) + " " + e.Type,
		Body:  body,
		Level: fmt.Sprint(cfg["level"]),
	})
}

func (noticeForwarderPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	if action != "send_test_notice" {
		return nil, unknownActionErr(action)
	}
	title := strings.TrimSpace(fmt.Sprint(input["title"]))
	if title == "" {
		title = "插件测试通知"
	}
	body := strings.TrimSpace(fmt.Sprint(input["body"]))
	if body == "" {
		body = "事件通知桥工作正常。"
	}
	if err := rt.Notice(ctx, NoticeInput{Title: title, Body: body, Level: "info"}); err != nil {
		return nil, err
	}
	return map[string]any{"sent": true, "title": title}, nil
}

func asBool(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true" || x == "1" || strings.EqualFold(x, "yes")
	default:
		return false
	}
}

func eventAllowed(raw, eventType string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ';'
	})
	for _, p := range parts {
		if strings.TrimSpace(p) == eventType {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
