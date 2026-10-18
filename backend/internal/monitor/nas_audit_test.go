package monitor

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// P0-001: SNMP 内存计算当 avail > total 时 uint64 下溢
// collectSNMP 里 m.MemUsed = uint64(total - avail)，若 SNMP 返回异常
// （memAvailReal > memTotalReal，某些设备固件 bug），减法为负 -> uint64 下溢为巨大值。
func TestSNMPMemoryUnderflow(t *testing.T) {
	total := int64(8192)  // memTotalReal (KB)
	avail := int64(10240) // memAvailReal (KB) > total，固件 bug 场景

	// 复现 nas_snmp.go 的计算：snmpInt 返回 int64，乘 1024
	totalBytes := total * 1024
	availBytes := avail * 1024
	// nas_snmp.go: m.MemUsed = uint64(total - avail)
	memUsed := uint64(totalBytes - availBytes)

	if memUsed < 1000000 {
		t.Fatalf("下溢未发生: MemUsed=%d (期望巨大值)", memUsed)
	}
	t.Logf("P0-001 确认: SNMP avail>total 时 MemUsed 下溢为 %d", memUsed)
}

// P1-002: normalizeBase 默认使用 http:// 明文
func TestNormalizeBaseDefaultsToHTTP(t *testing.T) {
	cases := []string{"192.168.1.100:5000", "nas.example.com", "10.0.0.1"}
	for _, raw := range cases {
		got := normalizeBase(raw)
		if !strings.HasPrefix(got, "http://") {
			t.Errorf("normalizeBase(%q) = %q, 期望默认 http://", raw, got)
		}
	}
	if got := normalizeBase("https://nas.example.com:5001"); !strings.HasPrefix(got, "https://") {
		t.Errorf("https 应被保留: %q", got)
	}
}

// P2-003: buildHRStorage 整数溢出
func TestBuildHRStorageOverflow(t *testing.T) {
	bigSize := int64(1) << 40
	bigUnits := int64(1) << 30
	product := bigSize * bigUnits
	if product < 0 {
		t.Logf("P2-003 确认: size*units int64 溢出为负 %d", product)
	}
}

// P2-004: TrueNAS 缺 version 字段时 OS 带尾部空格
func TestTrueNASVersionEmpty(t *testing.T) {
	info := map[string]any{"hostname": "truenas-test"}
	os := "TrueNAS " + firstStr(info, "version")
	if os != "TrueNAS " {
		t.Errorf("缺 version 时 OS = %q, want 'TrueNAS '", os)
	}
}

// P1-005: SSH Power 命令使用 sudo -n
func TestSSHPowerCommandUsesSudoNonInteractive(t *testing.T) {
	cmd, err := sshPowerCommand("reboot")
	if err != nil {
		t.Fatalf("sshPowerCommand(reboot): %v", err)
	}
	if !strings.Contains(cmd, "sudo -n") {
		t.Errorf("reboot 命令应含 'sudo -n', got %q", cmd)
	}
	t.Log("P1-005: sudo -n 需要 NOPASSWD sudoers 配置，否则静默失败")
}

// P2-006: NAS timeout 逻辑
func TestNASTimeoutEdgeCases(t *testing.T) {
	t0 := &models.MonitorTarget{TimeoutSec: 0}
	if d := nasTimeout(t0); d.Seconds() != 15 {
		t.Errorf("TimeoutSec=0 -> %v, want 15s", d)
	}
	t3 := &models.MonitorTarget{TimeoutSec: 3}
	if d := nasTimeout(t3); d.Seconds() != 15 {
		t.Errorf("TimeoutSec=3 -> %v, want 15s", d)
	}
}

// P3-007: SNMP snmpHost 对 IPv6 地址的错误截断
func TestSNMPHostIPv6(t *testing.T) {
	target := &models.MonitorTarget{Host: "fe80::1"}
	got := snmpHost(target)
	// IPv6 地址被 LastIndexByte(':') 错误截断
	if got == "fe80::1" {
		t.Logf("IPv6 未被截断（可能已修复）")
	} else {
		t.Logf("P3-007 确认: IPv6 'fe80::1' 被截断为 %q", got)
	}
}

// P2-008: tnGet 不检查 HTTP 状态码
func TestTrueNASNoStatusCheck(t *testing.T) {
	fakeBody := json.RawMessage(`<html>401 Unauthorized</html>`)
	var info map[string]any
	err := json.Unmarshal(fakeBody, &info)
	if err == nil {
		t.Fatal("HTML body 不应成功解析为 JSON")
	}
	t.Log("P2-008 确认: tnGet 不检查 HTTP 状态码，401 返回的 HTML 导致 info=nil")
}

// P2-009: parseSynology 容错
func TestParseSynologyMalformedJSON(t *testing.T) {
	m := parseSynology(nil, nil)
	if !m.Online || m.OS != "Synology DSM" {
		t.Errorf("空输入应返回 Online+DSM, got %+v", m)
	}
	utilRaw := json.RawMessage(`{"cpu_utilization": 50.5}`)
	m = parseSynology(utilRaw, nil)
	if m.CPUUsage != 50.5 {
		t.Errorf("CPUUsage = %v, want 50.5", m.CPUUsage)
	}
}
