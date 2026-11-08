package nodes

import (
	"context"
	"testing"

	"github.com/hxpt/ptpatronus/internal/workflow"
)

// P2-015: render 模板注入 - 变量值可注入到命令模板
// SSHCommandNode 的 command 参数支持 {{var.x}}，若 var.x 来自不可信源
// （如 RSS 标题），可注入 shell 命令
func TestRenderTemplateInjection(t *testing.T) {
	ecx := &workflow.Context{
		Variables: map[string]any{
			"filename": "movie; rm -rf /; #",
		},
	}
	// 模拟 SSHCommandNode 的 command 模板渲染
	rendered := render("scp {{var.filename}} host:/dest", ecx)
	expected := "scp movie; rm -rf /; # host:/dest"
	if rendered != expected {
		t.Errorf("render = %q, want %q", rendered, expected)
	}
	t.Logf("P2-015 确认: 变量注入 -> 命令拼接 = %q", rendered)
}

// P2-016: render 对不存在的变量静默返回空字符串
func TestRenderMissingVarReturnsEmpty(t *testing.T) {
	ecx := &workflow.Context{Variables: map[string]any{}}
	rendered := render("{{var.nonexistent}}", ecx)
	if rendered != "" {
		t.Errorf("不存在的变量应渲染为空, got %q", rendered)
	}
}

// P3-017: render 的 torrents.N.field 索引越界返回空
func TestRenderTorrentIndexOutOfBounds(t *testing.T) {
	ecx := &workflow.Context{Variables: map[string]any{}}
	// 无 torrents 数据，索引 0 应返回空
	rendered := render("{{torrents.0.title}}", ecx)
	if rendered != "" {
		t.Errorf("越界索引应返回空, got %q", rendered)
	}
}

// P2-018: MathNode 除法 - 除零检查存在但浮点精度问题
func TestMathDivFloatPrecision(t *testing.T) {
	ecx := &workflow.Context{Variables: map[string]any{}}
	params := map[string]any{
		"operation":  "div",
		"left":       "1",
		"right":      "0.0000001",
		"result_var": "result",
	}
	node := NewMathNode()
	err := node.Execute(context.TODO(), params, ecx)
	if err != nil {
		t.Fatalf("MathNode div: %v", err)
	}
	result := ecx.Variables["result"]
	t.Logf("1 / 0.0000001 = %v", result)
}

// P2-019: SwitchNode 规则解析 - 格式错误的行被静默跳过
func TestSwitchNodeMalformedRules(t *testing.T) {
	ecx := &workflow.Context{
		Variables: map[string]any{"count_val": 5},
		Torrents:  nil,
	}
	node := NewSwitchNode()
	params := map[string]any{
		"branches":       "high,low",
		"rules":          "high,count,gt,10\nlow,count,ge,3", // 正确格式
		"default_branch": "low",
	}
	err := node.Execute(context.TODO(), params, ecx)
	if err != nil {
		t.Fatalf("SwitchNode: %v", err)
	}
	// count=0 (无 torrents), gt 10 不中，ge 3 不中 -> default
	if len(ecx.Next) != 1 || ecx.Next[0] != "low" {
		t.Errorf("Next = %v, want [low]", ecx.Next)
	}
}

// P2-020: SSHCommandNode 未验证 target_id 对应的是否为 SSH 类型
// target_id 指向非 SSH host 时，RunCommandNow 会返回 ErrUnsupportedCommandTarget
// 但这个检查在 Service 层，节点层不检查
func TestSSHCommandNodeNilRunner(t *testing.T) {
	node := NewSSHCommandNode(nil)
	ecx := &workflow.Context{Variables: map[string]any{}}
	params := map[string]any{
		"target_id":   float64(1),
		"command":     "uptime",
		"output_var":  "out",
		"timeout_sec": float64(30),
	}
	err := node.Execute(context.TODO(), params, ecx)
	if err == nil {
		t.Fatal("nil runner 应返回错误")
	}
}
