package nodes

import (
	"context"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/notification"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

// SendMessageNode 发送通知（站内通知中心 + 外发渠道），并追加到 ecx.Messages。
type SendMessageNode struct {
	notice *notice.Service
}

func NewSendMessageNode(ns *notice.Service) *SendMessageNode { return &SendMessageNode{notice: ns} }

func (n *SendMessageNode) Type() string     { return "send_message" }
func (n *SendMessageNode) Name() string     { return "发送通知" }
func (n *SendMessageNode) Category() string { return "通知" }
func (n *SendMessageNode) Description() string {
	return "推送一条通知（站内 + bark/tg/webhook）"
}
func (n *SendMessageNode) Icon() string { return "message" }
func (n *SendMessageNode) DefaultParams() map[string]any {
	return map[string]any{"title": "工作流通知", "body": "", "level": "info"}
}
func (n *SendMessageNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "title", Label: "标题", Type: "text", Required: true},
		{Name: "body", Label: "内容", Type: "textarea", Help: "支持 {{var.x}} {{count}} 等模板"},
		{Name: "level", Label: "级别", Type: "select", Options: []workflow.ParamOption{
			{Label: "信息", Value: "info"}, {Label: "成功", Value: "success"},
			{Label: "警告", Value: "warning"}, {Label: "危险", Value: "danger"},
		}},
	}
}

func (n *SendMessageNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	title := strings.TrimSpace(render(workflow.ParamString(params, "title"), ecx))
	body := render(workflow.ParamString(params, "body"), ecx)
	level := workflow.ParamString(params, "level")
	if level == "" {
		level = "info"
	}
	if title == "" {
		title = "工作流通知"
	}
	if n.notice != nil {
		_ = n.notice.Notice(ctx, notice.Input{
			Source: notice.SourceWorkflow,
			Key:    "", // 一次性通知（不去重）
			Title:  title, Body: body, Level: level,
		}, false)
	}
	ecx.Messages = append(ecx.Messages, "已发送通知："+title)
	return nil
}

// SendNotificationNode 经 NotificationChain 直发外发渠道（bark/TG/Webhook）。
// 与 SendMessageNode 区别：不写站内通知中心，纯外发，并在消息中汇总各通道投递结果。
type SendNotificationNode struct {
	notif *chain.NotificationChain
}

func NewSendNotificationNode(notif *chain.NotificationChain) *SendNotificationNode {
	return &SendNotificationNode{notif: notif}
}

func (n *SendNotificationNode) Type() string     { return "send_notification" }
func (n *SendNotificationNode) Name() string     { return "多通道通知" }
func (n *SendNotificationNode) Category() string { return "通知" }
func (n *SendNotificationNode) Description() string {
	return "直接推送外发渠道（bark/TG/Webhook），不入站内通知中心"
}
func (n *SendNotificationNode) Icon() string { return "notify" }
func (n *SendNotificationNode) DefaultParams() map[string]any {
	return map[string]any{"title": "工作流通知", "body": "", "level": "info"}
}
func (n *SendNotificationNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "title", Label: "标题", Type: "text", Required: true},
		{Name: "body", Label: "内容", Type: "textarea", Help: "支持 {{var.x}} {{count}} 等模板"},
		{Name: "level", Label: "级别", Type: "select", Options: []workflow.ParamOption{
			{Label: "信息", Value: "info"}, {Label: "警告", Value: "warning"},
			{Label: "危险", Value: "danger"}, {Label: "庆典", Value: "celebration"},
		}},
	}
}

func (n *SendNotificationNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	if n.notif == nil {
		return fmt.Errorf("通知服务未初始化")
	}
	title := strings.TrimSpace(render(workflow.ParamString(params, "title"), ecx))
	body := render(workflow.ParamString(params, "body"), ecx)
	level := workflow.ParamString(params, "level")
	if level == "" {
		level = "info"
	}
	if title == "" {
		title = "工作流通知"
	}
	results := n.notif.Notify(ctx, notification.Message{Title: title, Body: body, Level: level})
	ok := 0
	for _, r := range results {
		if r.Ok {
			ok++
		}
	}
	if len(results) == 0 {
		ecx.Messages = append(ecx.Messages, "多通道通知：未配置任何外发渠道")
	} else {
		ecx.Messages = append(ecx.Messages, fmt.Sprintf("多通道通知「%s」：%d/%d 通道成功", title, ok, len(results)))
	}
	return nil
}
