package monitor

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

func mkTarget(typ, host, vendor, protocol string) *models.MonitorTarget {
	return &models.MonitorTarget{Type: typ, Host: host, Vendor: vendor, Protocol: protocol, TimeoutSec: 10, IntervalSec: 60}
}

func ctxBG() context.Context { return context.Background() }

// 真实 /proc 文本样例（SSH 采集脚本输出），断言 CPU/内存/负载/磁盘/网络解析。
const sshSample = `@@STAT1@@
cpu  100 0 0 900 0
@@STAT2@@
cpu  110 0 0 990 0
cpu0 50 0 0 450 0
cpu1 60 0 0 540 0
@@MEM@@
MemTotal:       16384 kB
MemFree:          256 kB
MemAvailable:    8192 kB
SwapTotal:        2048 kB
SwapFree:         1024 kB
@@LOAD@@
0.52 0.41 0.33 1/100 1234
@@UPT@@
3600.50 1800.25
@@DISK@@
Filesystem     1B-blocks      Used Available Use% Mounted on
/dev/sda1      1000000000 500000000 400000000  56% /
/dev/sda2      2000000000 800000000 1100000000  41% /home
@@NET@@
Inter-|   Receive                                                |  Transmit
 face |bytes packets errs drop fifo frame compressed multicast|bytes    packets
  eth0: 123456 789 0 0 0 0 0 0 654321 210 0 0 0 0 0 0
    lo: 999 1 0 0 0 0 0 0 999 1 0 0 0 0 0 0
@@HOST@@
hostname=paitang
kernel=6.8.0-51-generic
os=Ubuntu 24.04.1 LTS
cores=4
@@TEMP@@
coretemp Package_id_0 45000
coretemp Core_0 42000
thermal x86_pkg_temp 44000
@@END@@
`

func TestParseRemoteProc(t *testing.T) {
	m := parseRemoteProc(sshSample)
	// CPU：busy=10, total=100 → 10%
	if m.CPUUsage < 9.5 || m.CPUUsage > 10.5 {
		t.Errorf("cpu usage = %.2f, want ~10", m.CPUUsage)
	}
	// 内存：16384kB total, 8192kB avail → 50%
	if m.MemTotal != 16384*1024 {
		t.Errorf("mem total = %d, want %d", m.MemTotal, 16384*1024)
	}
	if m.MemUsage < 49.5 || m.MemUsage > 50.5 {
		t.Errorf("mem usage = %.2f, want ~50", m.MemUsage)
	}
	// 负载
	if m.Load1 != 0.52 || m.Load5 != 0.41 || m.Load15 != 0.33 {
		t.Errorf("load = %v/%v/%v", m.Load1, m.Load5, m.Load15)
	}
	if m.Uptime != 3600 {
		t.Errorf("uptime = %d, want 3600", m.Uptime)
	}
	// 磁盘两行
	if len(m.Disks) != 2 {
		t.Fatalf("disks = %d, want 2", len(m.Disks))
	}
	if m.Disks[0].Device != "/" || m.Disks[0].Usage != 56 {
		t.Errorf("disk0 = %+v", m.Disks[0])
	}
	// 网络：eth0 rx=123456 tx=654321（lo 不计）
	if m.NetRxBytes != 123456 || m.NetTxBytes != 654321 {
		t.Errorf("net = rx %d tx %d", m.NetRxBytes, m.NetTxBytes)
	}
	// 扩展字段：核心/Swap/进程/主机/温度
	if m.CPUCores != 4 { // HOST.cores 优先于 STAT cpuN 行数(2)
		t.Errorf("cpu cores = %d, want 4", m.CPUCores)
	}
	if m.SwapTotal != 2048*1024 || m.SwapUsed != 1024*1024 {
		t.Errorf("swap = %d/%d, want 1024k used of 2048k", m.SwapUsed, m.SwapTotal)
	}
	if m.ProcessCount != 100 {
		t.Errorf("process_count = %d, want 100", m.ProcessCount)
	}
	if m.Hostname != "paitang" || m.Kernel != "6.8.0-51-generic" {
		t.Errorf("host = %q kernel = %q", m.Hostname, m.Kernel)
	}
	if m.OS != "Ubuntu 24.04.1 LTS" {
		t.Errorf("os = %q", m.OS)
	}
	if len(m.Temps) < 2 {
		t.Fatalf("temps = %d, want >=2", len(m.Temps))
	}
	if m.Temps[0].Temp < 40 || m.Temps[0].Temp > 50 {
		t.Errorf("temp0 = %+v, want ~45C", m.Temps[0])
	}
}

func TestCountCPUCoresFromStat(t *testing.T) {
	raw := "cpu  1 2 3\ncpu0 1\ncpu1 1\ncpu2 1\nintr 0"
	if n := countCPUCoresFromStat(raw); n != 3 {
		t.Errorf("cores = %d, want 3", n)
	}
}

func TestParseTemps_SkipInvalid(t *testing.T) {
	raw := "coretemp Core_0 45000\nbogus\nhwmon bad 999999"
	ts := parseTemps(raw)
	if len(ts) != 1 || ts[0].Temp != 45 {
		t.Fatalf("temps = %+v", ts)
	}
}

// TestParseDF_BtrfsSubvolumeDedup 验证 btrfs 同一文件系统的多个 subvolume/snapshot 在 df 里各自成行、
// 且每行报告整池容量时，按 Filesystem(source)去重——同 /dev/ 设备只算一次。
// 回归飞牛 btrfs 把 50.92TB 池的 ~22 个子卷重复累加成 1.1PB 的缺陷。
func TestParseDF_BtrfsSubvolumeDedup(t *testing.T) {
	raw := `Filesystem     1B-blocks          Used Available Use% Mounted on
/dev/sda3      50920000000000   39870000000000 10000000000000  79% /mnt/pool
/dev/sda3      50920000000000   39870000000000 10000000000000  79% /mnt/pool/movies
/dev/sda3      50920000000000   39870000000000 10000000000000  79% /mnt/pool/tv
/dev/sda1      886000000000     579000000000    300000000000   66% /`
	disks := parseDF(raw)
	// 3 行 /dev/sda3 + 1 行 /dev/sda1 → 去重后 2 条
	if len(disks) != 2 {
		t.Fatalf("disks = %d (%+v), want 2（/dev/sda3 三子卷应去重为 1）", len(disks), disks)
	}
	// /dev/sda3 只计入一次：total 应为 单池50.92TB + 系统886GB，而非把 50.92TB 累加 3 次
	var sum uint64
	for _, d := range disks {
		sum += d.Total
	}
	if want := uint64(50920000000000 + 886000000000); sum != want {
		t.Errorf("total = %d, want %d（/dev/sda3 不应重复累加）", sum, want)
	}
}

// TestParseDF_OverlaySkipped docker overlay 把底层盘容量原样报出，必须整层跳过，否则同盘乘 N 倍。
// 回归：派糖 1 块 ~500GB 根盘 + 8 个 overlay → 卡片显示 4.4TB。
func TestParseDF_OverlaySkipped(t *testing.T) {
	raw := `Filesystem     1B-blocks      Used Available Use% Mounted on
/dev/sda1      539792977920 331035738112 180000000000  64% /
/dev/sda2         989052928    117829632    800000000  13% /boot
overlay        539792977920 331035738112 180000000000  64% /var/lib/docker/rootfs/overlayfs/aaa
overlay        539792977920 331035738112 180000000000  64% /var/lib/docker/rootfs/overlayfs/bbb
overlay        539792977920 331035738112 180000000000  64% /var/lib/docker/rootfs/overlayfs/ccc
/dev/sda1      539792977920 331035738112 180000000000  64% /var/lib/docker/volumes/x/_data`
	disks := parseDF(raw)
	if len(disks) != 2 {
		t.Fatalf("disks = %d (%+v), want 2（/ + /boot；overlay 跳过；同 /dev/sda1 去重）", len(disks), disks)
	}
	var sum uint64
	for _, d := range disks {
		sum += d.Total
	}
	if want := uint64(539792977920 + 989052928); sum != want {
		t.Errorf("total = %d, want %d", sum, want)
	}
}

// TestParseDF_SameDeviceDockerBindDedup 同一 /dev 先挂 /data 再 bind 进 docker，只计一次。
func TestParseDF_SameDeviceDockerBindDedup(t *testing.T) {
	raw := `Filesystem     1B-blocks      Used Available Use% Mounted on
/dev/sdb1      1000000000000 500000000000 500000000000  50% /data
/dev/sdb1      1000000000000 500000000000 500000000000  50% /var/lib/docker/volumes/media/_data`
	disks := parseDF(raw)
	if len(disks) != 1 {
		t.Fatalf("disks = %d, want 1", len(disks))
	}
	if disks[0].Device != "/data" {
		t.Errorf("device = %q, want /data", disks[0].Device)
	}
}

func TestCPUUsageFromStat(t *testing.T) {
	// dt==0 应返回 0（防除零）
	if v := cpuUsageFromStat("cpu 1 2 3 4 5", "cpu 1 2 3 4 5"); v != 0 {
		t.Errorf("zero-delta cpu = %.2f, want 0", v)
	}
}

func TestExtractTag(t *testing.T) {
	xml := `<s:Envelope><tds:GetDeviceInformationResponse><tds:Manufacturer>Hikvision</tds:Manufacturer><tds:Model>DS-2CD</tds:Model></tds:GetDeviceInformationResponse></s:Envelope>`
	if v := extractTag(xml, "Manufacturer"); v != "Hikvision" {
		t.Errorf("manufacturer = %q", v)
	}
	if v := extractTag(xml, "Model"); v != "DS-2CD" {
		t.Errorf("model = %q", v)
	}
}

func TestParseThresholds(t *testing.T) {
	def := ParseThresholds("")
	if def.CPUUsage != 90 || def.DiskUsage != 90 {
		t.Errorf("default thresholds = %+v", def)
	}
	if def.Offline == nil || !*def.Offline {
		t.Error("default offline should be true")
	}
	custom := ParseThresholds(`{"cpu_usage":75,"mem_usage":80}`)
	if custom.CPUUsage != 75 || custom.MemUsage != 80 {
		t.Errorf("custom = %+v", custom)
	}
	if custom.DiskUsage != 90 { // 未设应回退默认
		t.Errorf("disk fallback = %.0f, want 90", custom.DiskUsage)
	}
}

func TestRegistryAndResolve(t *testing.T) {
	r := NewRegistry()
	localCollector := NewHostCollector()
	r.Register("host", "local", localCollector)
	r.Register("nas", "local", localCollector)
	c, ok := r.CollectorFor("host", "local")
	if !ok || c == nil {
		t.Fatal("host/local collector not found")
	}
	c, ok = r.CollectorFor("nas", "local")
	if !ok || c == nil {
		t.Fatal("nas/local collector not found")
	}
	if _, ok := r.CollectorFor("host", "xxx"); ok {
		t.Error("unknown protocol should miss")
	}
	// ResolveProtocol 推断
	local := mkTarget("host", "", "", "")
	if ResolveProtocol(local) != "local" {
		t.Errorf("local host resolve = %s", ResolveProtocol(local))
	}
	remote := mkTarget("host", "1.2.3.4", "", "")
	if ResolveProtocol(remote) != "ssh" {
		t.Errorf("remote host resolve = %s", ResolveProtocol(remote))
	}
	nas := mkTarget("nas", "nas", "", "")
	if ResolveProtocol(nas) != "api" {
		t.Errorf("nas resolve = %s", ResolveProtocol(nas))
	}
	localNAS := mkTarget("nas", "", "", "")
	if ResolveProtocol(localNAS) != "local" {
		t.Errorf("local nas resolve = %s", ResolveProtocol(localNAS))
	}
}

func TestHostCollectSmoke(t *testing.T) {
	// 本机真实采集烟雾测试：不 panic、返回 online、内存>0。
	m, err := NewHostCollector().Collect(ctxBG(), mkTarget("host", "", "", ""), Credentials{})
	if err != nil {
		t.Fatalf("host collect: %v", err)
	}
	if !m.Online {
		t.Error("local host should be online")
	}
	if m.MemTotal == 0 {
		t.Error("mem total should be > 0")
	}
}

type fakeSSHExec struct {
	cmd string
}

func (f *fakeSSHExec) Run(_ context.Context, cmd string) (string, error) {
	f.cmd = cmd
	return "", nil
}

func (f *fakeSSHExec) Close() error { return nil }

type fakeSSHOutputExec struct {
	cmd    string
	stdout string
	stderr string
	err    error
}

func (f *fakeSSHOutputExec) Run(_ context.Context, cmd string) (string, error) {
	f.cmd = cmd
	return f.stdout, f.err
}

func (f *fakeSSHOutputExec) RunOutput(_ context.Context, cmd string) (string, string, error) {
	f.cmd = cmd
	return f.stdout, f.stderr, f.err
}

func (f *fakeSSHOutputExec) Close() error { return nil }

func TestSSHCollectorPowerRebootUsesFixedCommand(t *testing.T) {
	ex := &fakeSSHExec{}
	c := SSHCollector{dial: func(*models.MonitorTarget, Credentials, time.Duration) (sshExecutor, error) {
		return ex, nil
	}}
	err := c.Power(context.Background(), mkTarget("host", "127.0.0.1", "", "ssh"), Credentials{Username: "root"}, PowerActionReboot)
	if err != nil {
		t.Fatalf("power reboot: %v", err)
	}
	if !strings.Contains(ex.cmd, "sudo -n reboot") || !strings.Contains(ex.cmd, "sudo reboot") {
		t.Fatalf("command = %q", ex.cmd)
	}
}

func TestSSHCollectorPowerShutdownUsesFixedCommand(t *testing.T) {
	ex := &fakeSSHExec{}
	c := SSHCollector{dial: func(*models.MonitorTarget, Credentials, time.Duration) (sshExecutor, error) {
		return ex, nil
	}}
	err := c.Power(context.Background(), mkTarget("host", "127.0.0.1", "", "ssh"), Credentials{Username: "root"}, PowerActionShutdown)
	if err != nil {
		t.Fatalf("power shutdown: %v", err)
	}
	if !strings.Contains(ex.cmd, "sudo -n shutdown -h now") || !strings.Contains(ex.cmd, "sudo shutdown -h now") {
		t.Fatalf("command = %q", ex.cmd)
	}
}

func TestSSHCollectorPowerRejectsUnknownAction(t *testing.T) {
	c := SSHCollector{dial: func(*models.MonitorTarget, Credentials, time.Duration) (sshExecutor, error) {
		t.Fatal("dial should not be called for unsupported action")
		return nil, nil
	}}
	err := c.Power(context.Background(), mkTarget("host", "127.0.0.1", "", "ssh"), Credentials{}, "hibernate")
	if err == nil || !strings.Contains(err.Error(), ErrUnsupportedPowerAction.Error()) {
		t.Fatalf("err = %v, want unsupported power action", err)
	}
}

func TestSSHCollectorRunCommandCapturesOutput(t *testing.T) {
	ex := &fakeSSHOutputExec{stdout: "ok\n", stderr: "warn\n", err: errors.New("exit status 1")}
	c := SSHCollector{dial: func(*models.MonitorTarget, Credentials, time.Duration) (sshExecutor, error) {
		return ex, nil
	}}
	out, err := c.RunCommand(context.Background(), mkTarget("host", "127.0.0.1", "", "ssh"), Credentials{Username: "root"}, "whoami")
	if err == nil || err.Error() != "exit status 1" {
		t.Fatalf("err = %v, want exit status 1", err)
	}
	if ex.cmd != "whoami" {
		t.Fatalf("command = %q", ex.cmd)
	}
	if out.Stdout != "ok\n" || out.Stderr != "warn\n" {
		t.Fatalf("output = %#v", out)
	}
}

func TestLocalPowerCommandUsesFixedCommand(t *testing.T) {
	name, args, err := localPowerCommand(PowerActionShutdown)
	if err != nil {
		t.Fatalf("local shutdown command: %v", err)
	}
	if runtime.GOOS == "windows" {
		if name != "shutdown.exe" || strings.Join(args, " ") != "/s /t 0" {
			t.Fatalf("local windows shutdown = %s %v", name, args)
		}
		return
	}
	if os.Geteuid() == 0 {
		if name != "shutdown" || strings.Join(args, " ") != "-h now" {
			t.Fatalf("local root shutdown = %s %v", name, args)
		}
		return
	}
	if name != "sudo" || strings.Join(args, " ") != "-n shutdown -h now" {
		t.Fatalf("local shutdown = %s %v", name, args)
	}
}
