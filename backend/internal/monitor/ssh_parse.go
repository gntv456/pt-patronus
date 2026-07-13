package monitor

import (
	"regexp"
	"strconv"
	"strings"
)

// parseRemoteProc 解析 sshScript 输出为 Metrics。各 section 用 @@NAME@@ 标记分隔。
func parseRemoteProc(out string) Metrics {
	secs := splitSections(out)
	m := Metrics{Online: true}
	if s1, ok := secs["STAT1"]; ok {
		if s2, ok2 := secs["STAT2"]; ok2 {
			m.CPUUsage = cpuUsageFromStat(s1, s2)
		}
	}
	if raw, ok := secs["MEM"]; ok {
		total, avail := parseMemInfo(raw)
		m.MemTotal = total
		m.MemUsed = total - avail
		if total > 0 {
			m.MemUsage = float64(total-avail) / float64(total) * 100
		}
	}
	if raw, ok := secs["LOAD"]; ok {
		m.Load1, m.Load5, m.Load15 = parseLoadAvg(raw)
	}
	if raw, ok := secs["UPT"]; ok {
		m.Uptime = parseUptime(raw)
	}
	if raw, ok := secs["DISK"]; ok {
		m.Disks = parseDF(raw)
	}
	if raw, ok := secs["NET"]; ok {
		m.NetRxBytes, m.NetTxBytes = parseNetDev(raw)
	}
	return m
}

var sectionRe = regexp.MustCompile(`@@(\w+)@@`)

// splitSections 按 @@NAME@@ 标记切片，返回 name→内容。
func splitSections(out string) map[string]string {
	secs := map[string]string{}
	idx := sectionRe.FindAllStringSubmatchIndex(out, -1)
	for i, m := range idx {
		name := out[m[2]:m[3]]
		start := m[1]
		end := len(out)
		if i+1 < len(idx) {
			end = idx[i+1][0]
		}
		secs[name] = strings.TrimSpace(out[start:end])
	}
	return secs
}

// cpuUsageFromStat 用两次 /proc/stat 首行差值算 CPU 使用率。
// 首行格式：cpu  user nice system idle iowait irq softirq steal ...
func cpuUsageFromStat(s1, s2 string) float64 {
	i1, t1 := cpuIdleTotal(s1)
	i2, t2 := cpuIdleTotal(s2)
	dt := t2 - t1
	if dt == 0 {
		return 0
	}
	return float64(dt-(i2-i1)) / float64(dt) * 100
}

func cpuIdleTotal(line string) (idle, total uint64) {
	f := strings.Fields(line)
	for i := 1; i < len(f); i++ {
		v, _ := strconv.ParseUint(f[i], 10, 64)
		total += v
		if i == 4 || i == 5 { // idle + iowait 视为空闲
			idle += v
		}
	}
	return
}

// parseMemInfo 取 MemTotal / MemAvailable（kB → bytes）。
func parseMemInfo(raw string) (total, avail uint64) {
	for _, line := range strings.Split(raw, "\n") {
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total = parseKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			avail = parseKB(line)
		}
	}
	return
}

func parseKB(line string) uint64 {
	f := strings.Fields(line)
	if len(f) < 2 {
		return 0
	}
	v, _ := strconv.ParseUint(f[1], 10, 64)
	return v * 1024
}

func parseLoadAvg(raw string) (l1, l5, l15 float64) {
	f := strings.Fields(raw)
	if len(f) >= 3 {
		l1, _ = strconv.ParseFloat(f[0], 64)
		l5, _ = strconv.ParseFloat(f[1], 64)
		l15, _ = strconv.ParseFloat(f[2], 64)
	}
	return
}

func parseUptime(raw string) uint64 {
	f := strings.Fields(raw)
	if len(f) >= 1 {
		v, _ := strconv.ParseFloat(f[0], 64)
		return uint64(v)
	}
	return 0
}

// parseDF 解析 df -B1 输出：Filesystem 1B-blocks Used Avail Use% Mountpoint。
func parseDF(raw string) []DiskMetric {
	var disks []DiskMetric
	seen := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		f := strings.Fields(line)
		if len(f) < 6 || f[0] == "Filesystem" || !strings.HasSuffix(f[4], "%") {
			continue
		}
		// 按 Filesystem(source)去重：btrfs/zfs 同一文件系统的子卷/快照在 df 里各自一行，
		// 且每行都报告整池容量，全累加会把同一物理池重复计入 N 倍（飞牛 btrfs 实测 22 倍成 1.1PB）。
		// 只对真实块设备（/dev/…）去重——overlay/aufs 等各层 source 名相同却确是不同层，不去重。
		if src := f[0]; strings.HasPrefix(src, "/dev/") {
			if seen[src] {
				continue
			}
			seen[src] = true
		}
		total, _ := strconv.ParseUint(f[1], 10, 64)
		used, _ := strconv.ParseUint(f[2], 10, 64)
		usePct, _ := strconv.ParseFloat(strings.TrimSuffix(f[4], "%"), 64)
		disks = append(disks, DiskMetric{Device: f[5], Total: total, Used: used, Usage: usePct, Type: "partition"})
	}
	return disks
}

// parseNetDev 解析 /proc/net/dev，累加非 lo 接口的 rx/tx 字节。
func parseNetDev(raw string) (rx, tx uint64) {
	for _, line := range strings.Split(raw, "\n") {
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		iface := strings.TrimSpace(line[:idx])
		if iface == "lo" {
			continue
		}
		f := strings.Fields(line[idx+1:])
		if len(f) >= 9 {
			r, _ := strconv.ParseUint(f[0], 10, 64) // 第 1 列 rx bytes
			t, _ := strconv.ParseUint(f[8], 10, 64) // 第 9 列 tx bytes
			rx += r
			tx += t
		}
	}
	return
}
