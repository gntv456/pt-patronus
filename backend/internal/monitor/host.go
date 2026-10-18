package monitor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/sysinfo"
)

// HostCollector 采集本机（PTPatronus 运行所在机器）指标，委托 sysinfo（local 与 agent 共用同一份采集逻辑）。
// 忽略 target.Host 与 creds（本机恒在线）。
type HostCollector struct{}

// NewHostCollector 构造。
func NewHostCollector() HostCollector { return HostCollector{} }

func (HostCollector) String() string { return "host(local/gopsutil)" }

// Probe 本机探活：host.Info 成功即在线（走 sysinfo.HostOnline，免 200ms CPU 采样）。
func (HostCollector) Probe(_ context.Context, _ *models.MonitorTarget, _ Credentials) (bool, error) {
	return sysinfo.HostOnline(), nil
}

// Collect 采集本机 CPU/内存/磁盘/网络/负载/温度/进程/运行时长（sysinfo.CollectSystemMetrics）。
// Metrics 是 sysinfo.Metrics 的类型别名，零转换。
func (HostCollector) Collect(_ context.Context, t *models.MonitorTarget, _ Credentials) (Metrics, error) {
	return sysinfo.CollectSystemMetrics(parseDiskPaths(t.DiskPaths)...), nil
}

// Power 对 PTPatronus 所在机器执行固定电源动作；仅支持重启/关机白名单。
func (HostCollector) Power(ctx context.Context, _ *models.MonitorTarget, _ Credentials, action string) error {
	action = strings.ToLower(strings.TrimSpace(action))
	name, args, err := localPowerCommand(action)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUnsupportedPowerAction, action)
	}
	return runLocalPower(ctx, name, args)
}

// runLocalPower 执行本机电源命令，并把 stderr/stdout 一并带回错误里。
// `shutdown.exe /r` 在权限不足时只回 exit status 5，真正的原因（拒绝访问）在 stderr；
// 从前用 Run() 丢掉它，界面上就只剩「exit status 5」这种没头没尾的失败。
func runLocalPower(ctx context.Context, name string, args []string) error {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err == nil {
		return nil
	}
	if msg := strings.TrimSpace(string(out)); msg != "" {
		return fmt.Errorf("%w: %s", err, msg)
	}
	return err
}

func localPowerCommand(action string) (string, []string, error) {
	switch runtime.GOOS {
	case "windows":
		switch action {
		case PowerActionReboot:
			return "shutdown.exe", []string{"/r", "/t", "0"}, nil
		case PowerActionShutdown:
			return "shutdown.exe", []string{"/s", "/t", "0"}, nil
		}
	default:
		if os.Geteuid() == 0 {
			switch action {
			case PowerActionReboot:
				return "reboot", nil, nil
			case PowerActionShutdown:
				return "shutdown", []string{"-h", "now"}, nil
			}
		}
		switch action {
		case PowerActionReboot:
			return "sudo", []string{"-n", "reboot"}, nil
		case PowerActionShutdown:
			return "sudo", []string{"-n", "shutdown", "-h", "now"}, nil
		}
	}
	return "", nil, ErrUnsupportedPowerAction
}

// parseDiskPaths 把 target.DiskPaths（换行或逗号分隔）拆成路径切片，trim + 去空。
// 供 host 采集器把宿主 bind mount 目录传给 sysinfo.CollectSystemMetrics（MoviePilot 式磁盘监控）。
func parseDiskPaths(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		for _, p := range strings.Split(line, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
