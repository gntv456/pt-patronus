package monitor

import (
	"context"

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
