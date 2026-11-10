// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package nodes

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

// SwitchNode 多分支条件（switch/case）：按 rules 顺序求值，首个命中分支激活（ecx.Next=[label]）。
// 输出边 SourcePort = 分支标签（来自 branches 参数，前端按此渲染多输出桩）。
type SwitchNode struct{}

func NewSwitchNode() *SwitchNode { return &SwitchNode{} }

func (n *SwitchNode) Type() string     { return "switch" }
func (n *SwitchNode) Name() string     { return "多分支" }
func (n *SwitchNode) Category() string { return "逻辑" }
func (n *SwitchNode) Description() string {
	return "多路分支（switch/case），命中即走对应分支"
}
func (n *SwitchNode) Icon() string { return "switch" }
func (n *SwitchNode) DefaultParams() map[string]any {
	return map[string]any{
		"branches":       "大,中,小",
		"rules":          "大,count,ge,10\n中,count,ge,3",
		"default_branch": "小",
	}
}
func (n *SwitchNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "branches", Label: "分支标签", Type: "text", Required: true, Help: "逗号分隔，如 大,中,小；每个标签一个输出桩"},
		{Name: "rules", Label: "规则", Type: "textarea", Required: true,
			Help: "每行 label,left,op,right；left 用 count 或 var.变量名；op ∈ gt/ge/eq/ne/lt/le/contains；按序首个命中即走"},
		{Name: "default_branch", Label: "默认分支", Type: "text", Required: true, Help: "无规则命中时走此分支（须在分支标签中）"},
	}
}

func (n *SwitchNode) Execute(_ context.Context, params map[string]any, ecx *workflow.Context) error {
	defaultB := strings.TrimSpace(workflow.ParamString(params, "default_branch"))
	rules := workflow.ParamString(params, "rules")
	for _, line := range strings.Split(rules, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) != 4 {
			continue
		}
		label := strings.TrimSpace(parts[0])
		leftTok := strings.TrimSpace(parts[1])
		op := strings.TrimSpace(parts[2])
		right := strings.TrimSpace(parts[3])
		if evalRule(ecx, leftTok, op, right) {
			ecx.Next = []string{label}
			ecx.Messages = append(ecx.Messages, "多分支 → "+label)
			return nil
		}
	}
	ecx.Next = []string{defaultB}
	ecx.Messages = append(ecx.Messages, "多分支 → "+defaultB+"（默认）")
	return nil
}

// evalRule 求值 left op right。left ∈ {count, var.NAME}；数值用 cmp，contains 用字符串包含。
func evalRule(ecx *workflow.Context, left, op, right string) bool {
	lv := resolveLeft(ecx, left)
	if op == "contains" {
		return strings.Contains(fmt.Sprintf("%v", lv), right)
	}
	if ln, e1 := strconv.ParseFloat(fmt.Sprintf("%v", lv), 64); e1 == nil {
		if rn, e2 := strconv.ParseFloat(right, 64); e2 == nil {
			return cmpFloat(ln, rn, op)
		}
	}
	// 回退字符串比较
	switch op {
	case "eq":
		return fmt.Sprintf("%v", lv) == right
	case "ne":
		return fmt.Sprintf("%v", lv) != right
	}
	return false
}

func resolveLeft(ecx *workflow.Context, left string) any {
	left = strings.TrimSpace(left)
	if left == "count" {
		return len(ecx.Torrents)
	}
	if strings.HasPrefix(left, "var.") {
		if ecx.Variables != nil {
			return ecx.Variables[strings.TrimPrefix(left, "var.")]
		}
	}
	return left
}

// WebhookSendNode 发送出站 HTTP（webhook 通知外部系统）。
type WebhookSendNode struct {
	hc *httpclient.Client
}

func NewWebhookSendNode(hc *httpclient.Client) *WebhookSendNode { return &WebhookSendNode{hc: hc} }

func (n *WebhookSendNode) Type() string     { return "webhook_send" }
func (n *WebhookSendNode) Name() string     { return "发送 Webhook" }
func (n *WebhookSendNode) Category() string { return "通知" }
func (n *WebhookSendNode) Description() string {
	return "向外部 URL 发送 HTTP 请求（webhook 通知）"
}
func (n *WebhookSendNode) Icon() string { return "webhook" }
func (n *WebhookSendNode) DefaultParams() map[string]any {
	return map[string]any{"url": "", "method": "POST", "body": "", "headers": map[string]any{}}
}
func (n *WebhookSendNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "url", Label: "URL", Type: "text", Required: true},
		{Name: "method", Label: "方法", Type: "select", Options: []workflow.ParamOption{
			{Label: "POST", Value: "POST"}, {Label: "GET", Value: "GET"}, {Label: "PUT", Value: "PUT"},
		}},
		{Name: "headers", Label: "请求头", Type: "kv", Help: "可加 Authorization 等认证头"},
		{Name: "body", Label: "请求体", Type: "textarea", Help: "支持 {{var.x}} 模板"},
	}
}

func (n *WebhookSendNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	rawURL := strings.TrimSpace(workflow.ParamString(params, "url"))
	if rawURL == "" {
		return fmt.Errorf("URL 为空")
	}
	method := strings.ToUpper(workflow.ParamString(params, "method"))
	if method == "" {
		method = "POST"
	}
	var body []byte
	if b := workflow.ParamString(params, "body"); b != "" {
		body = []byte(render(b, ecx))
	}
	headers := workflow.ParamKV(params, "headers")
	resp, err := n.hc.Do(ctx, method, rawURL, body, headers, "", true)
	if err != nil {
		return fmt.Errorf("webhook 发送失败: %w", err)
	}
	if ecx.Variables == nil {
		ecx.Variables = map[string]any{}
	}
	ecx.Variables["_webhook_status"] = resp.StatusCode
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("Webhook %s → HTTP %d", method, resp.StatusCode))
	return nil
}

// MathNode 变量运算/拼接：对两个操作数做加减乘除或字符串拼接，结果存入变量。
type MathNode struct{}

func NewMathNode() *MathNode { return &MathNode{} }

func (n *MathNode) Type() string     { return "math" }
func (n *MathNode) Name() string     { return "变量运算" }
func (n *MathNode) Category() string { return "流程" }
func (n *MathNode) Description() string {
	return "对两个值做加减乘除或拼接，结果存入变量"
}
func (n *MathNode) Icon() string { return "math" }
func (n *MathNode) DefaultParams() map[string]any {
	return map[string]any{"operation": "add", "left": "0", "right": "0", "result_var": "result"}
}
func (n *MathNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "operation", Label: "运算", Type: "select", Options: []workflow.ParamOption{
			{Label: "加 (+)", Value: "add"}, {Label: "减 (-)", Value: "sub"},
			{Label: "乘 (×)", Value: "mul"}, {Label: "除 (÷)", Value: "div"},
			{Label: "拼接", Value: "concat"},
		}},
		{Name: "left", Label: "左值", Type: "text", Help: "支持 {{var.x}} {{count}}"},
		{Name: "right", Label: "右值", Type: "text"},
		{Name: "result_var", Label: "结果变量名", Type: "text", Required: true},
	}
}

func (n *MathNode) Execute(_ context.Context, params map[string]any, ecx *workflow.Context) error {
	op := workflow.ParamString(params, "operation")
	left := render(workflow.ParamString(params, "left"), ecx)
	right := render(workflow.ParamString(params, "right"), ecx)
	key := strings.TrimSpace(workflow.ParamString(params, "result_var"))
	if key == "" {
		return fmt.Errorf("结果变量名为空")
	}
	if ecx.Variables == nil {
		ecx.Variables = map[string]any{}
	}
	if op == "concat" {
		ecx.Variables[key] = left + right
		return nil
	}
	ln, e1 := strconv.ParseFloat(strings.TrimSpace(left), 64)
	rn, e2 := strconv.ParseFloat(strings.TrimSpace(right), 64)
	if e1 != nil || e2 != nil {
		return fmt.Errorf("运算需要数值，得到 %q / %q", left, right)
	}
	var r float64
	switch op {
	case "add":
		r = ln + rn
	case "sub":
		r = ln - rn
	case "mul":
		r = ln * rn
	case "div":
		if rn == 0 {
			return fmt.Errorf("除数为零")
		}
		r = ln / rn
	default:
		return fmt.Errorf("未知运算: %s", op)
	}
	// 整数结果存为 int，否则 float
	if r == float64(int64(r)) {
		ecx.Variables[key] = int64(r)
	} else {
		ecx.Variables[key] = r
	}
	return nil
}
