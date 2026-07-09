package nodes

import (
	"context"
	"fmt"
	"strings"

	"github.com/hxpt/ptpatronus/internal/monitor"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

// SSHCommandRunner is the monitor service method used by SSHCommandNode.
type SSHCommandRunner interface {
	RunCommandNow(ctx context.Context, id uint, command string, timeoutSec int) (monitor.CommandResult, error)
}

// SSHCommandNode executes a command on a configured SSH host.
type SSHCommandNode struct {
	runner SSHCommandRunner
}

func NewSSHCommandNode(runner SSHCommandRunner) *SSHCommandNode {
	return &SSHCommandNode{runner: runner}
}

func (n *SSHCommandNode) Type() string     { return "ssh_command" }
func (n *SSHCommandNode) Name() string     { return "SSH 命令" }
func (n *SSHCommandNode) Category() string { return "运维" }
func (n *SSHCommandNode) Description() string {
	return "在已配置的 SSH 主机上执行命令，并把输出写入变量"
}
func (n *SSHCommandNode) Icon() string { return "terminal" }
func (n *SSHCommandNode) DefaultParams() map[string]any {
	return map[string]any{
		"target_id":   float64(0),
		"command":     "uptime && df -h",
		"timeout_sec": float64(30),
		"output_var":  "ssh_output",
	}
}
func (n *SSHCommandNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "target_id", Label: "服务器 ID", Type: "number", Required: true, Help: "填写 NAS/服务器页中的 SSH 主机 ID"},
		{Name: "command", Label: "命令", Type: "textarea", Required: true, Help: "支持 {{var.x}}、{{content}} 等工作流模板变量"},
		{Name: "timeout_sec", Label: "超时秒数", Type: "number", Default: float64(30), Help: "最短 5 秒，最长 600 秒"},
		{Name: "output_var", Label: "输出变量名", Type: "text", Default: "ssh_output", Help: "后续节点可用 {{var.ssh_output}} 读取标准输出"},
	}
}

func (n *SSHCommandNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	if n.runner == nil {
		return fmt.Errorf("SSH 命令执行器未初始化")
	}
	targetID := workflow.ParamInt(params, "target_id")
	if targetID <= 0 {
		return fmt.Errorf("未选择服务器")
	}
	command := strings.TrimSpace(render(workflow.ParamString(params, "command"), ecx))
	if command == "" {
		return fmt.Errorf("命令不能为空")
	}
	timeoutSec := workflow.ParamInt(params, "timeout_sec")
	result, err := n.runner.RunCommandNow(ctx, uint(targetID), command, timeoutSec)
	if err != nil {
		return err
	}
	outputVar := strings.TrimSpace(workflow.ParamString(params, "output_var"))
	if outputVar == "" {
		outputVar = "ssh_output"
	}
	if ecx.Variables == nil {
		ecx.Variables = map[string]any{}
	}
	ecx.Variables[outputVar] = result.Stdout
	ecx.Variables[outputVar+"_stderr"] = result.Stderr
	ecx.Variables[outputVar+"_duration_ms"] = result.DurationMS
	ecx.Content = result.Stdout
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("SSH 命令：%s 执行完成，用时 %dms", result.TargetName, result.DurationMS))
	if !result.Success {
		msg := strings.TrimSpace(result.Error)
		if msg == "" {
			msg = strings.TrimSpace(result.Stderr)
		}
		if msg == "" {
			msg = "远程命令返回失败"
		}
		return fmt.Errorf("SSH 命令失败：%s", msg)
	}
	return nil
}
