package nodes

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/workflow"
)

// ConditionNode 条件判断：求值后设 ecx.Next（["true"] 或 ["false"]），executor 据此激活分支边。
// 条件节点的两条出边 SourcePort 分别为 "true"/"false"。
type ConditionNode struct{}

func NewConditionNode() *ConditionNode { return &ConditionNode{} }

func (n *ConditionNode) Type() string        { return "condition" }
func (n *ConditionNode) Name() string        { return "条件判断" }
func (n *ConditionNode) Category() string    { return "逻辑" }
func (n *ConditionNode) Description() string { return "按条件二分，走 true 或 false 分支" }
func (n *ConditionNode) Icon() string        { return "condition" }
func (n *ConditionNode) DefaultParams() map[string]any {
	return map[string]any{
		"left": "torrents", "variable": "", "operator": "gt", "value": "0",
	}
}
func (n *ConditionNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "left", Label: "判断对象", Type: "select", Options: []workflow.ParamOption{
			{Label: "资源数量", Value: "torrents"},
			{Label: "变量值", Value: "variable"},
		}},
		{Name: "variable", Label: "变量名", Type: "text", Help: "判断对象选「变量值」时填写"},
		{Name: "operator", Label: "运算符", Type: "select", Options: []workflow.ParamOption{
			{Label: ">", Value: "gt"}, {Label: ">=", Value: "ge"}, {Label: "=", Value: "eq"},
			{Label: "!=", Value: "ne"}, {Label: "<", Value: "lt"}, {Label: "<=", Value: "le"},
			{Label: "包含", Value: "contains"},
		}},
		{Name: "value", Label: "比较值", Type: "text", Required: true},
	}
}

func (n *ConditionNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	left := workflow.ParamString(params, "left")
	op := workflow.ParamString(params, "operator")
	val := workflow.ParamString(params, "value")

	matched := false
	switch left {
	case "torrents":
		lv := len(ecx.Torrents)
		rv, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil {
			return fmt.Errorf("资源数量比较值需为整数")
		}
		matched = cmpInt(lv, rv, op)
	case "variable":
		name := strings.TrimSpace(workflow.ParamString(params, "variable"))
		var lv string
		if ecx.Variables != nil {
			lv = fmt.Sprintf("%v", ecx.Variables[name])
		}
		if op == "contains" {
			matched = strings.Contains(lv, val)
		} else {
			// 尝试数值比较，失败回退字符串相等
			if ln, e1 := strconv.ParseFloat(lv, 64); e1 == nil {
				if rn, e2 := strconv.ParseFloat(val, 64); e2 == nil {
					matched = cmpFloat(ln, rn, op)
					break
				}
			}
			matched = (lv == val && (op == "eq" || op == "ge" || op == "le")) ||
				(lv != val && op == "ne")
		}
	}
	if matched {
		ecx.Next = []string{"true"}
	} else {
		ecx.Next = []string{"false"}
	}
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("条件判断 → %v", matched))
	return nil
}

func cmpInt(a, b int, op string) bool {
	switch op {
	case "gt":
		return a > b
	case "ge":
		return a >= b
	case "eq":
		return a == b
	case "ne":
		return a != b
	case "lt":
		return a < b
	case "le":
		return a <= b
	}
	return false
}

func cmpFloat(a, b float64, op string) bool {
	switch op {
	case "gt":
		return a > b
	case "ge":
		return a >= b
	case "eq":
		return a == b
	case "ne":
		return a != b
	case "lt":
		return a < b
	case "le":
		return a <= b
	}
	return false
}

// DelayNode 延时等待（上限 60s）。
type DelayNode struct{}

func NewDelayNode() *DelayNode { return &DelayNode{} }

func (n *DelayNode) Type() string        { return "delay" }
func (n *DelayNode) Name() string        { return "延时" }
func (n *DelayNode) Category() string    { return "逻辑" }
func (n *DelayNode) Description() string { return "等待若干秒（上限 60）" }
func (n *DelayNode) Icon() string        { return "delay" }
func (n *DelayNode) DefaultParams() map[string]any {
	return map[string]any{"seconds": float64(5)}
}
func (n *DelayNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "seconds", Label: "秒数", Type: "number", Required: true, Default: float64(5)},
	}
}

func (n *DelayNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	sec := workflow.ParamInt(params, "seconds")
	if sec <= 0 {
		return nil
	}
	if sec > 60 {
		sec = 60
	}
	select {
	case <-time.After(time.Duration(sec) * time.Second):
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// NoteNode 备注（no-op，仅记一条日志）。
type NoteNode struct{}

func NewNoteNode() *NoteNode { return &NoteNode{} }

func (n *NoteNode) Type() string        { return "note" }
func (n *NoteNode) Name() string        { return "备注" }
func (n *NoteNode) Category() string    { return "逻辑" }
func (n *NoteNode) Description() string { return "仅做标注，不执行任何操作" }
func (n *NoteNode) Icon() string        { return "note" }
func (n *NoteNode) DefaultParams() map[string]any {
	return map[string]any{"text": ""}
}
func (n *NoteNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "text", Label: "备注内容", Type: "textarea"},
	}
}

func (n *NoteNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	if t := workflow.ParamString(params, "text"); t != "" {
		ecx.Messages = append(ecx.Messages, "备注："+t)
	}
	return nil
}

// SetVariableNode 设置 ecx.Variables[key]（值支持模板渲染）。
type SetVariableNode struct{}

func NewSetVariableNode() *SetVariableNode { return &SetVariableNode{} }

func (n *SetVariableNode) Type() string        { return "set_variable" }
func (n *SetVariableNode) Name() string        { return "设置变量" }
func (n *SetVariableNode) Category() string    { return "流程" }
func (n *SetVariableNode) Description() string { return "设置一个变量供后续节点引用" }
func (n *SetVariableNode) Icon() string        { return "variable" }
func (n *SetVariableNode) DefaultParams() map[string]any {
	return map[string]any{"key": "", "value": ""}
}
func (n *SetVariableNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "key", Label: "变量名", Type: "text", Required: true},
		{Name: "value", Label: "值", Type: "text", Help: "支持 {{var.x}} {{count}} {{torrents.0.title}} 模板"},
	}
}

func (n *SetVariableNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	key := strings.TrimSpace(workflow.ParamString(params, "key"))
	if key == "" {
		return fmt.Errorf("变量名为空")
	}
	if ecx.Variables == nil {
		ecx.Variables = map[string]any{}
	}
	ecx.Variables[key] = render(workflow.ParamString(params, "value"), ecx)
	return nil
}

// StopFlowNode 置 ecx.Stop=true，成功短路终止整条流（区别于 fail-fast 的错误终止）。
type StopFlowNode struct{}

func NewStopFlowNode() *StopFlowNode { return &StopFlowNode{} }

func (n *StopFlowNode) Type() string        { return "stop_flow" }
func (n *StopFlowNode) Name() string        { return "终止流程" }
func (n *StopFlowNode) Category() string    { return "流程" }
func (n *StopFlowNode) Description() string { return "立即成功终止整条工作流（短路）" }
func (n *StopFlowNode) Icon() string        { return "stop" }
func (n *StopFlowNode) DefaultParams() map[string]any {
	return map[string]any{"reason": ""}
}
func (n *StopFlowNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "reason", Label: "原因", Type: "text", Help: "记录到日志，支持 {{var.x}}"},
	}
}

func (n *StopFlowNode) Execute(_ context.Context, params map[string]any, ecx *workflow.Context) error {
	ecx.Stop = true
	reason := strings.TrimSpace(render(workflow.ParamString(params, "reason"), ecx))
	if reason == "" {
		reason = "手动终止"
	}
	ecx.Messages = append(ecx.Messages, "终止流程："+reason)
	return nil
}

// RandomPickNode 从资源集合中随机抽取 N 条（打乱后取前 N）。
type RandomPickNode struct{}

func NewRandomPickNode() *RandomPickNode { return &RandomPickNode{} }

func (n *RandomPickNode) Type() string        { return "random_pick" }
func (n *RandomPickNode) Name() string        { return "随机抽取" }
func (n *RandomPickNode) Category() string    { return "过滤" }
func (n *RandomPickNode) Description() string { return "从资源集合中随机抽取 N 条" }
func (n *RandomPickNode) Icon() string        { return "random" }
func (n *RandomPickNode) DefaultParams() map[string]any {
	return map[string]any{"count": float64(1)}
}
func (n *RandomPickNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "count", Label: "抽取 N 条", Type: "number", Default: float64(1)},
	}
}

func (n *RandomPickNode) Execute(_ context.Context, params map[string]any, ecx *workflow.Context) error {
	count := workflow.ParamInt(params, "count")
	if count <= 0 {
		count = 1
	}
	rand.Shuffle(len(ecx.Torrents), func(i, j int) {
		ecx.Torrents[i], ecx.Torrents[j] = ecx.Torrents[j], ecx.Torrents[i]
	})
	if len(ecx.Torrents) > count {
		ecx.Torrents = ecx.Torrents[:count]
	}
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("随机抽取 %d 条", len(ecx.Torrents)))
	return nil
}

// FormatSizeNode 把字节数（参数或变量）格式化为人类可读（如 1.50 GiB），写入变量。
type FormatSizeNode struct{}

func NewFormatSizeNode() *FormatSizeNode { return &FormatSizeNode{} }

func (n *FormatSizeNode) Type() string        { return "format_size" }
func (n *FormatSizeNode) Name() string        { return "格式化字节" }
func (n *FormatSizeNode) Category() string    { return "流程" }
func (n *FormatSizeNode) Description() string { return "把字节数格式化为人类可读（如 1.50 GiB）" }
func (n *FormatSizeNode) Icon() string        { return "format" }
func (n *FormatSizeNode) DefaultParams() map[string]any {
	return map[string]any{"value": "", "output_var": "size_text"}
}
func (n *FormatSizeNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "value", Label: "字节数", Type: "text", Required: true, Help: "支持 {{var.uploaded}} 等模板"},
		{Name: "output_var", Label: "输出变量", Type: "text", Default: "size_text"},
	}
}

func (n *FormatSizeNode) Execute(_ context.Context, params map[string]any, ecx *workflow.Context) error {
	raw := strings.TrimSpace(render(workflow.ParamString(params, "value"), ecx))
	if raw == "" {
		return fmt.Errorf("字节数为空")
	}
	b, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fmt.Errorf("无法解析为字节数: %s", raw)
	}
	out := workflow.ParamString(params, "output_var")
	if out == "" {
		out = "size_text"
	}
	if ecx.Variables == nil {
		ecx.Variables = map[string]any{}
	}
	ecx.Variables[out] = humanBytes(b)
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("格式化字节：%s → %s", raw, humanBytes(b)))
	return nil
}

// humanBytes 把字节数格式化为二进制人类可读（1024 进制）。
func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}
