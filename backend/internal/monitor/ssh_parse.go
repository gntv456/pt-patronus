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
			// STAT2 改为完整 /proc/stat 时，用 cpuN 行数作核心数（HOST.cores 优先覆盖）。
			if c := countCPUCoresFromStat(s2); c > 0 {
				m.CPUCores = c
			}
		}
	}
	if raw, ok := secs["MEM"]; ok {
		mi := parseMemInfoFull(raw)
		m.MemTotal = mi.Total
		m.MemUsed = mi.Total - mi.Available
		if mi.Total > 0 {
			m.MemUsage = float64(mi.Total-mi.Available) / float64(mi.Total) * 100
		}
		m.SwapTotal = mi.SwapTotal
		if mi.SwapTotal >= mi.SwapFree {
			m.SwapUsed = mi.SwapTotal - mi.SwapFree
		}
	}
	if raw, ok := secs["LOAD"]; ok {
		m.Load1, m.Load5, m.Load15, m.ProcessCount = parseLoadAvgFull(raw)
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
	if raw, ok := secs["HOST"]; ok {
		applyHostSection(&m, raw)
	}
	if raw, ok := secs["TEMP"]; ok {
		m.Temps = parseTemps(raw)
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

// cpuUsageFromStat 用两次 /proc/stat 汇总行（首行 "cpu "）差值算 CPU 使用率。
// STAT2 可能是完整 /proc/stat（含 cpu0/cpu1…），必须只取首行，否则会把各核字段累进 total。
func cpuUsageFromStat(s1, s2 string) float64 {
	i1, t1 := cpuIdleTotal(firstLine(s1))
	i2, t2 := cpuIdleTotal(firstLine(s2))
	dt := t2 - t1
	if dt == 0 {
		return 0
	}
	return float64(dt-(i2-i1)) / float64(dt) * 100
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
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

// parseMemInfo 取 MemTotal / MemAvailable（kB → bytes）。保留给旧调用/测试。
func parseMemInfo(raw string) (total, avail uint64) {
	mi := parseMemInfoFull(raw)
	return mi.Total, mi.Available
}

type memInfo struct {
	Total, Available, SwapTotal, SwapFree uint64
}

// parseMemInfoFull 解析 MemTotal/Available 与 SwapTotal/Free（kB → bytes）。
func parseMemInfoFull(raw string) memInfo {
	var mi memInfo
	for _, line := range strings.Split(raw, "\n") {
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			mi.Total = parseKB(line)
		case strings.HasPrefix(line, "MemAvailable:"):
			mi.Available = parseKB(line)
		case strings.HasPrefix(line, "SwapTotal:"):
			mi.SwapTotal = parseKB(line)
		case strings.HasPrefix(line, "SwapFree:"):
			mi.SwapFree = parseKB(line)
		}
	}
	return mi
}

// countCPUCoresFromStat 统计 /proc/stat 里 cpu0..cpuN 行数。
func countCPUCoresFromStat(raw string) int {
	n := 0
	for _, line := range strings.Split(raw, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		// 只要 cpu0/cpu1…，跳过汇总行 "cpu "
		if strings.HasPrefix(f[0], "cpu") && f[0] != "cpu" {
			n++
		}
	}
	return n
}

// applyHostSection 解析 hostname= / kernel= / os= / cores= 键值行。
func applyHostSection(m *Metrics, raw string) {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.ToLower(key))
		val = strings.TrimSpace(val)
		switch key {
		case "hostname":
			m.Hostname = val
		case "kernel":
			m.Kernel = val
		case "os":
			m.OS = val
		case "cores":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				m.CPUCores = n
			}
		}
	}
}

// parseTemps 解析 TEMP 段：每行 "chip label millidegC"，值通常是毫摄氏度。
func parseTemps(raw string) []TempMetric {
	var out []TempMetric
	seen := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		chip, label := f[0], f[1]
		milli, err := strconv.ParseFloat(f[len(f)-1], 64)
		if err != nil {
			continue
		}
		// hwmon 为毫摄氏度；少数驱动已是摄氏度（值 < 200 且无小数意义时仍按毫度：200mC=0.2C 不合理，阈值 1000）
		temp := milli
		if milli > 200 { // 绝大多数是 millidegree
			temp = milli / 1000
		}
		if temp <= 0 || temp > 150 {
			continue
		}
		name := chip + "_" + label
		name = strings.ToLower(strings.ReplaceAll(name, " ", "_"))
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, TempMetric{Name: name, Temp: temp})
	}
	return out
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
	l1, l5, l15, _ = parseLoadAvgFull(raw)
	return
}

// parseLoadAvgFull 解析负载与进程数。第 4 字段形如 "1/234"（运行中/总进程）。
func parseLoadAvgFull(raw string) (l1, l5, l15 float64, procs int) {
	f := strings.Fields(raw)
	if len(f) >= 3 {
		l1, _ = strconv.ParseFloat(f[0], 64)
		l5, _ = strconv.ParseFloat(f[1], 64)
		l15, _ = strconv.ParseFloat(f[2], 64)
	}
	if len(f) >= 4 {
		if i := strings.IndexByte(f[3], '/'); i >= 0 {
			procs, _ = strconv.Atoi(f[3][i+1:])
		}
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

// virtualDFSources：df 里会出现但没有独立物理容量的源（容器层/伪 FS）。
// 它们通常把底层根盘容量原样报出，计入会把同一块盘乘 N 倍（派糖 8 个 docker overlay → 0.5TB 变 4TB）。
var virtualDFSources = map[string]bool{
	"overlay": true, "overlay2": true, "aufs": true, "tmpfs": true, "devtmpfs": true,
	"squashfs": true, "shm": true, "devfs": true, "proc": true, "sysfs": true,
	"cgroup": true, "cgroup2": true, "nsfs": true, "fuse.lxcfs": true,
}

// skipDiskMount 过滤容器运行时/虚拟挂载点——这些路径 stat/df 读到的是宿主同盘容量，不能当独立盘累加。
func skipDiskMount(mount string) bool {
	m := strings.ToLower(mount)
	// Windows 盘符不走这里；Linux/容器路径：
	prefixes := []string{
		"/var/lib/docker/",
		"/var/lib/containerd/",
		"/var/lib/containers/",
		"/run/docker/",
		"/run/containerd/",
		"/run/containers/",
		"/var/lib/lxc/",
		"/snap/",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(m, p) {
			return true
		}
	}
	// docker rootfs 旧路径偶发不带尾斜杠变体
	if strings.Contains(m, "/docker/rootfs/") || strings.Contains(m, "/overlay2/") {
		return true
	}
	return false
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
		src, mount := f[0], f[5]
		// 跳过容器层 / 伪文件系统：无独立容量，df 报的是底层盘，计入会把同盘乘 N 倍。
		if virtualDFSources[src] {
			continue
		}
		// 按 Filesystem(source)去重：btrfs/zfs 同一文件系统的子卷/快照在 df 里各自一行，
		// 且每行都报告整池容量，全累加会把同一物理池重复计入 N 倍（飞牛 btrfs 实测 22 倍成 1.1PB）。
		if strings.HasPrefix(src, "/dev/") {
			if seen[src] {
				continue
			}
			seen[src] = true
			// 同设备若先出现在 docker 挂载点、后出现在正常挂载点，仍只计一次（seen 已标记）。
			// 展示名优先非容器路径：容器路径先到时仍计入容量，避免「盘只 bind 进 docker」被丢。
		} else if skipDiskMount(mount) {
			// 非块设备（NFS 等）挂在容器路径下也跳过，防重复。
			continue
		}
		total, _ := strconv.ParseUint(f[1], 10, 64)
		used, _ := strconv.ParseUint(f[2], 10, 64)
		usePct, _ := strconv.ParseFloat(strings.TrimSuffix(f[4], "%"), 64)
		disks = append(disks, DiskMetric{Device: mount, Total: total, Used: used, Usage: usePct, Type: "partition"})
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
