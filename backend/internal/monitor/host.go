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
func (HostCollector) Collect(_ context.Context, _ *models.MonitorTarget, _ Credentials) (Metrics, error) {
	return sysinfo.CollectSystemMetrics(), nil
}

// Power 对 PTPatronus 所在机器执行固定电源动作；仅支持重启/关机白名单。
func (HostCollector) Power(ctx context.Context, _ *models.MonitorTarget, _ Credentials, action string) error {
	action = strings.ToLower(strings.TrimSpace(action))
	name, args, err := localPowerCommand(action)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUnsupportedPowerAction, action)
	}
	return exec.CommandContext(ctx, name, args...).Run()
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
