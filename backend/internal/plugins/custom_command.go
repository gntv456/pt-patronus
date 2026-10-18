package plugins

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// custom-command：定时在宿主机上执行一组 shell 命令并按结果推送通知。
// 与工作流 ssh_command（面向 agent 远程 SSH）互补：本插件面向宿主机本机命令编排。
// 对标 MoviePilot 自定义命令增强（thsrite）。
//
// 设计取舍：插件 Schedule 由 manifest 静态声明，无法为用户输入的 N 条命令各自注册 cron，
// 故采用「单一全局 cron 触发 + 一次运行全部已启用命令」模型（per-command 独立 cron 留工作流）。
// 每条命令可单独启用；执行超时、失败/成功推送策略、工作目录由配置决定。

// cmdItem 是配置 commands 文本域的一行：name|command[|timeout_sec]。
type cmdItem struct {
	Name    string
	Command string
	Timeout int // 秒；0=用全局 timeout
}

// cmdResult 是单条命令的执行结果。
type cmdResult struct {
	Name      string `json:"name"`
	Command   string `json:"command"`
	ExitCode  int    `json:"exit_code"`
	Output    string `json:"output"`
	Duration  int    `json:"duration_ms"`
	TimedOut  bool   `json:"timed_out,omitempty"`
	Error     string `json:"error,omitempty"`
	Succeeded bool   `json:"succeeded"`
}

func parseCmdItems(raw string) []cmdItem {
	out := make([]cmdItem, 0, 8)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		cmd := strings.TrimSpace(parts[1])
		if name == "" || cmd == "" {
			continue
		}
		item := cmdItem{Name: name, Command: cmd}
		if len(parts) == 3 {
			item.Timeout = atoiOr(parts[2], 0)
		}
		out = append(out, item)
	}
	return out
}

func atoiOr(s string, def int) int {
	s = strings.TrimSpace(s)
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return def
		}
		n = n*10 + int(r-'0')
	}
	if s == "" {
		return def
	}
	return n
}

type customCommandPlugin struct{}

// NewCustomCommandPlugin 构造。
func NewCustomCommandPlugin() Plugin { return &customCommandPlugin{} }

func (p *customCommandPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "ptp-custom-command",
		Name:        "自定义命令",
		Version:     "1.0.0",
		APIVersion:  "1",
		AppVersion:  ">=0.1.0",
		Author:      "PTPatronus Community",
		Description: "【宿主机定时命令】按全局定时在宿主机执行一组 shell 命令（备份/清理/健康检查等），依据退出码推送成功/失败通知。与工作流 ssh_command（agent 远程）互补，本插件面向宿主机本机。",
		License:     "MIT",
		Keywords:    []string{"command", "cron", "shell", "exec", "schedule"},
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"log:write", "notice:write", "kv:read", "kv:write"},
		Actions: []ActionSpec{
			{Name: "run", Label: "立即运行全部", Description: "运行全部已配置命令并返回每条结果。"},
			{Name: "test", Label: "运行单条", Description: "按入参 name 运行单条命令（用于联调）。", InputSchema: []ConfigField{{Key: "name", Label: "命令名", Type: "text"}}},
		},
		ConfigSchema: []ConfigField{
			{Key: "commands", Label: "命令列表", Type: "textarea", Group: "命令", Required: true, Help: "每行一条：name|command[|timeout秒]。# 开头注释。如：清理临时|rm -rf /tmp/ptpatronus/*|60"},
			{Key: "working_dir", Label: "工作目录", Type: "text", Group: "命令", Help: "命令执行的工作目录，留空为进程当前目录。"},
			{Key: "timeout", Label: "默认超时(秒)", Type: "number", Group: "命令", Default: 300},
			{Key: "push_on_success", Label: "成功也推送", Type: "switch", Group: "通知", Default: false, Help: "开启后命令成功（exit 0）也推送通知，默认仅失败推送。"},
			{Key: "push_on_fail", Label: "失败推送", Type: "switch", Group: "通知", Default: true},
			{Key: "cron", Label: "定时表达式", Type: "text", Group: "调度", Default: "0 */6 * * *", Help: "默认每 6 小时运行全部命令。留空则不自动，可手动触发。"},
		},
		Schedule: []ScheduleSpec{
			{ID: "run", Label: "定时运行命令", Action: "run", Cron: "0 */6 * * *", CronConfigKey: "cron"},
		},
		Contributes: &ContributionSpec{
			Commands: []CommandContribution{{ID: "ptp-custom-command.run", Label: "自定义命令：立即运行全部", Action: "run"}},
			Menus:    []MenuContribution{{Location: "plugins/actions", Command: "ptp-custom-command.run"}},
		},
		ResourceAccess: &ResourceAccessSpec{
			ExecBinaries: []string{"用户配置的任意 shell 命令（rm/curl/tar/系统工具等）"},
		},
	}
}

func (p *customCommandPlugin) DefaultConfig() map[string]any {
	return map[string]any{
		"commands": "", "working_dir": "", "timeout": 300,
		"push_on_success": false, "push_on_fail": true, "cron": "0 */6 * * *",
	}
}

func (p *customCommandPlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (p *customCommandPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	switch action {
	case "run", "run_now":
		results := p.runAll(ctx, rt)
		return map[string]any{"ok": true, "results": results}, nil
	case "test":
		name := strings.TrimSpace(fmt.Sprint(input["name"]))
		items := parseCmdItems(pluginString(rt.Config()["commands"]))
		for _, it := range items {
			if it.Name == name {
				r := p.runOne(ctx, rt, it)
				return map[string]any{"ok": true, "result": r}, nil
			}
		}
		return nil, fmt.Errorf("命令 %q 不存在", name)
	default:
		return nil, unknownActionErr(action)
	}
}

// runAll 逐条执行并按配置推送汇总通知。
func (p *customCommandPlugin) runAll(ctx context.Context, rt Runtime) []cmdResult {
	cfg := rt.Config()
	items := parseCmdItems(pluginString(cfg["commands"]))
	results := make([]cmdResult, 0, len(items))
	for _, it := range items {
		results = append(results, p.runOne(ctx, rt, it))
	}
	p.notifyResults(ctx, rt, results)
	return results
}

func hostShellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd.exe", "/D", "/S", "/C", command)
	}
	return exec.CommandContext(ctx, "sh", "-c", command)
}

// runOne 执行单条命令，使用宿主平台 shell，带超时；超时则 kill 进程。
func (p *customCommandPlugin) runOne(ctx context.Context, rt Runtime, it cmdItem) cmdResult {
	cfg := rt.Config()
	timeout := it.Timeout
	if timeout <= 0 {
		timeout = atoiOr(fmt.Sprint(cfg["timeout"]), 300)
	}
	if timeout <= 0 {
		timeout = 300
	}
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	start := time.Now()
	cmd := hostShellCommand(cctx, it.Command)
	if wd := strings.TrimSpace(pluginString(cfg["working_dir"])); wd != "" {
		cmd.Dir = wd
	}
	out, err := cmd.CombinedOutput()
	r := cmdResult{
		Name: it.Name, Command: it.Command,
		Output: truncateOut(string(out)), Duration: int(time.Since(start) / time.Millisecond),
	}
	if cctx.Err() == context.DeadlineExceeded {
		r.TimedOut = true
		r.Error = fmt.Sprintf("超时（%ds）", timeout)
		r.Succeeded = false
	} else if err != nil {
		r.Error = err.Error()
		if ee, ok := err.(*exec.ExitError); ok {
			r.ExitCode = ee.ExitCode()
		}
		r.Succeeded = false
	} else {
		r.Succeeded = true
	}
	rt.Log(condLogLevel(r.Succeeded), "custom_command",
		fmt.Sprintf("%s exit=%d", it.Name, r.ExitCode), map[string]any{"name": it.Name, "succeeded": r.Succeeded})
	return r
}

// notifyResults 按成功/失败策略汇总推送一条通知（非每条一推，避免刷屏）。
func (p *customCommandPlugin) notifyResults(ctx context.Context, rt Runtime, results []cmdResult) {
	if len(results) == 0 {
		return
	}
	cfg := rt.Config()
	pushSuccess, _ := cfg["push_on_success"].(bool)
	pushFail, _ := cfg["push_on_fail"].(bool)
	failed := make([]cmdResult, 0)
	succeeded := 0
	for _, r := range results {
		if r.Succeeded {
			succeeded++
		} else {
			failed = append(failed, r)
		}
	}
	level := "success"
	title := fmt.Sprintf("自定义命令完成：%d 成功 %d 失败", succeeded, len(failed))
	if len(failed) > 0 {
		level = "danger"
		title = fmt.Sprintf("自定义命令有 %d 条失败", len(failed))
	} else if !pushSuccess {
		return // 全成功且未开「成功也推送」：静默
	}
	if len(failed) == 0 && !pushSuccess {
		return
	}
	if len(failed) > 0 && !pushFail {
		return
	}
	var sb strings.Builder
	for _, r := range failed {
		sb.WriteString(fmt.Sprintf("• %s：exit %d %s\n", r.Name, r.ExitCode, r.Error))
	}
	if sb.Len() == 0 {
		sb.WriteString(fmt.Sprintf("全部 %d 条命令执行成功。", succeeded))
	}
	_ = rt.Notice(ctx, NoticeInput{Title: title, Body: strings.TrimSpace(sb.String()), Level: level})
}

func truncateOut(s string) string {
	const max = 2000
	if len(s) > max {
		return s[len(s)-max:] + "\n...(输出已截断)"
	}
	return strings.TrimSpace(s)
}

func condLogLevel(ok bool) string {
	if ok {
		return logInfo
	}
	return logError
}
