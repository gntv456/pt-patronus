package monitor

import (
	"context"
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
@@MEM@@
MemTotal:       16384 kB
MemFree:          256 kB
MemAvailable:    8192 kB
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
	r.Register("host", "local", NewHostCollector())
	c, ok := r.CollectorFor("host", "local")
	if !ok || c == nil {
		t.Fatal("host/local collector not found")
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

func TestSSHCollectorPowerRebootUsesFixedCommand(t *testing.T) {
	ex := &fakeSSHExec{}
	c := SSHCollector{dial: func(*models.MonitorTarget, Credentials, time.Duration) (sshExecutor, error) {
		return ex, nil
	}}
	err := c.Power(context.Background(), mkTarget("host", "127.0.0.1", "", "ssh"), Credentials{Username: "root"}, PowerActionReboot)
	if err != nil {
		t.Fatalf("power reboot: %v", err)
	}
	if ex.cmd != "sudo -n reboot" {
		t.Fatalf("command = %q", ex.cmd)
	}
}

func TestSSHCollectorPowerRejectsUnknownAction(t *testing.T) {
	c := SSHCollector{dial: func(*models.MonitorTarget, Credentials, time.Duration) (sshExecutor, error) {
		t.Fatal("dial should not be called for unsupported action")
		return nil, nil
	}}
	err := c.Power(context.Background(), mkTarget("host", "127.0.0.1", "", "ssh"), Credentials{}, "shutdown")
	if err == nil || !strings.Contains(err.Error(), ErrUnsupportedPowerAction.Error()) {
		t.Fatalf("err = %v, want unsupported power action", err)
	}
}
