// Package sysinfo 封装 gopsutil 本机指标采集（纯函数，不依赖 target/models）。
// 设计：被 internal/monitor 的 local HostCollector 和 standalone agent 二进制共用，
// 避免采集逻辑重复；agent 不 import internal/monitor（否则拉入 RTSP/gortsplib 重型依赖致体积爆炸）。
//
// Metrics/DiskMetric/TempMetric/DeviceInfo 在此为唯一定义源；internal/monitor 用类型别名复用
// （type Metrics = sysinfo.Metrics），故 agent 上报的 JSON 可在服务端直接 json.Unmarshal 成
// monitor.Metrics（= sysinfo.Metrics），字段与 json tag 完全对齐。
package sysinfo

import (
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/shirou/gopsutil/v4/sensors"
)

// Metrics 一次本机采集的指标快照。字段与 json tag 与（未来的）monitor.Metrics 别名完全一致。
type Metrics struct {
	Online       bool           `json:"online"`
	CPUUsage     float64        `json:"cpu_usage,omitempty"` // %（0-100）
	CPUCores     int            `json:"cpu_cores,omitempty"`
	MemTotal     uint64         `json:"mem_total,omitempty"` // bytes
	MemUsed      uint64         `json:"mem_used,omitempty"`
	MemUsage     float64        `json:"mem_usage,omitempty"` // %
	SwapTotal    uint64         `json:"swap_total,omitempty"`
	SwapUsed     uint64         `json:"swap_used,omitempty"`
	Disks        []DiskMetric   `json:"disks,omitempty"`        // 分区 / 存储池
	NetRxBytes   uint64         `json:"net_rx_bytes,omitempty"` // 累计接收字节（采集器填）
	NetTxBytes   uint64         `json:"net_tx_bytes,omitempty"` // 累计发送字节（采集器填）
	NetRxRate    uint64         `json:"net_rx_rate,omitempty"`  // 接收速率 bytes/s（服务端对比上次快照算）
	NetTxRate    uint64         `json:"net_tx_rate,omitempty"`  // 发送速率 bytes/s
	Load1        float64        `json:"load1,omitempty"`
	Load5        float64        `json:"load5,omitempty"`
	Load15       float64        `json:"load15,omitempty"`
	Temps        []TempMetric   `json:"temps,omitempty"`
	ProcessCount int            `json:"process_count,omitempty"`
	Uptime       uint64         `json:"uptime,omitempty"` // 秒
	OS           string         `json:"os,omitempty"`
	Hostname     string         `json:"hostname,omitempty"`
	Kernel       string         `json:"kernel_version,omitempty"`
	DeviceInfo   *DeviceInfo    `json:"device_info,omitempty"` // 摄像头等（agent 不填）
	Extra        map[string]any `json:"extra,omitempty"`
}

// DiskMetric 单个分区或存储池用量。
type DiskMetric struct {
	Device string  `json:"device"`
	Total  uint64  `json:"total"`
	Used   uint64  `json:"used"`
	Usage  float64 `json:"usage"`          // %
	Type   string  `json:"type,omitempty"` // partition | pool
}

// TempMetric 温度读数。
type TempMetric struct {
	Name string  `json:"name"`
	Temp float64 `json:"temp"` // °C
}

// DeviceInfo 设备标识信息（摄像头 ONVIF 等；agent 不填）。
type DeviceInfo struct {
	Manufacturer string `json:"manufacturer"`
	Model        string `json:"model"`
	Firmware     string `json:"firmware"`
	Serial       string `json:"serial"`
	Hardware     string `json:"hardware"`
}

// CollectSystemMetrics 采集本机 CPU/内存/磁盘/网络/负载/温度/进程/运行时长。
// 单点失败不中断其余指标；返回的 Metrics.Online 恒为 true。
// 速率(NetRxRate/NetTxRate)不在此算——由服务端对比上次快照算（agent 只报累计字节）。
// extraPaths：host 采集器可传宿主 bind mount 进容器的目录（MoviePilot 式），statvfs 穿透到宿主 FS；
// agent 上报走无参调用（extraPaths=nil），行为不变。
func CollectSystemMetrics(extraPaths ...string) Metrics {
	m := Metrics{Online: true}
	// CPU：短间隔采样阻塞 ~200ms（Percent 内部两次 Times 差值，避免手算）。
	if percents, err := cpu.Percent(200*time.Millisecond, false); err == nil && len(percents) > 0 {
		m.CPUUsage = percents[0]
	}
	if cores, err := cpu.Counts(true); err == nil {
		m.CPUCores = cores
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		m.MemTotal, m.MemUsed, m.MemUsage = vm.Total, vm.Used, vm.UsedPercent
	}
	if sm, err := mem.SwapMemory(); err == nil {
		m.SwapTotal, m.SwapUsed = sm.Total, sm.Used
	}
	// 磁盘：collectDisks 拿全部挂载点（pseudoFS 黑名单过滤）+ extraPaths（宿主 bind mount 目录），
	// st_dev 去重防同设备重复统计。
	m.Disks = collectDisks(extraPaths)
	// 网络：累计字节（速率由服务端对比上次快照算）。
	if io, err := gnet.IOCounters(false); err == nil && len(io) > 0 {
		m.NetRxBytes = io[0].BytesRecv
		m.NetTxBytes = io[0].BytesSent
	}
	if l, err := load.Avg(); err == nil {
		m.Load1, m.Load5, m.Load15 = l.Load1, l.Load5, l.Load15
	}
	if hi, err := host.Info(); err == nil {
		m.Uptime = hi.Uptime
		m.OS = hi.OS + " " + hi.Platform + " " + hi.PlatformVersion
		m.Hostname = hi.Hostname
		m.Kernel = hi.KernelVersion
	}
	if ps, err := process.Processes(); err == nil {
		m.ProcessCount = len(ps)
	}
	// 温度：Linux 读 hwmon；Windows 多为空，忽略错误。
	if temps, err := sensors.SensorsTemperatures(); err == nil {
		for _, t := range temps {
			if t.Temperature > 0 {
				m.Temps = append(m.Temps, TempMetric{Name: t.SensorKey, Temp: t.Temperature})
			}
		}
	}
	return m
}

// HostOnline 本机是否可达（host.Info 成功即在线）；供 Probe 复用，避免 CollectSystemMetrics 的 200ms 采样。
func HostOnline() bool {
	_, err := host.Info()
	return err == nil
}

// pseudoFS 列出不需要统计用量的伪/虚拟文件系统类型（overlay 不在此列——容器根）。
var pseudoFS = map[string]bool{
	"tmpfs": true, "devtmpfs": true, "devfs": true, "proc": true, "sysfs": true,
	"cgroup": true, "cgroup2": true, "mqueue": true, "shm": true, "autofs": true,
	"rpc_pipefs": true, "fuse.gvfsd-fuse": true, "fdescfs": true,
}

func isPseudoFS(fstype string) bool { return pseudoFS[fstype] }

// collectDisks 采集磁盘用量：先遍历所有挂载点（pseudoFS 黑名单过滤），再对 extraPaths 逐个 statvfs。
// 用 st_dev 去重——同一物理设备上的多个挂载点/bind/子目录只统计一次（防 /mnt/nas/movies 与
// /mnt/nas/tv 同池重复计入，对齐 MoviePilot space_usage 的 st_dev 去重）。devOf 在非 Linux 返 0 即跳过去重。
func collectDisks(extraPaths []string) []DiskMetric {
	var out []DiskMetric
	seen := map[uint64]bool{}
	if parts, err := disk.Partitions(true); err == nil {
		for _, p := range parts {
			if isPseudoFS(p.Fstype) {
				continue
			}
			u, err := disk.Usage(p.Mountpoint)
			if err != nil {
				continue
			}
			if dev := devOf(p.Mountpoint); dev != 0 {
				if seen[dev] {
					continue
				}
				seen[dev] = true
			}
			out = append(out, DiskMetric{Device: p.Mountpoint, Total: u.Total, Used: u.Used, Usage: u.UsedPercent, Type: "partition"})
		}
	}
	// 额外路径：宿主目录经 docker bind mount 进容器，statvfs 穿透到宿主 FS（MoviePilot 式）。
	for _, raw := range extraPaths {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		u, err := disk.Usage(p)
		if err != nil {
			continue
		}
		if dev := devOf(p); dev != 0 {
			if seen[dev] {
				continue
			}
			seen[dev] = true
		}
		out = append(out, DiskMetric{Device: p, Total: u.Total, Used: u.Used, Usage: u.UsedPercent, Type: "partition"})
	}
	return out
}
