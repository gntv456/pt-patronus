package sysinfo

import "testing"

// TestCollectSystemMetrics 烟雾测试：本机采集应产出非空的关键字段（仿 monitor_test.go 的 TestHostCollectSmoke）。
func TestCollectSystemMetrics(t *testing.T) {
	m := CollectSystemMetrics()
	if !m.Online {
		t.Error("Online should be true")
	}
	if m.MemTotal == 0 {
		t.Error("MemTotal should be > 0")
	}
	if m.CPUCores == 0 {
		t.Error("CPUCores should be > 0")
	}
	if m.Hostname == "" {
		t.Error("Hostname should not be empty")
	}
	if m.Uptime == 0 {
		t.Error("Uptime should be > 0")
	}
	if len(m.Disks) == 0 {
		t.Log("warn: no disks collected (unusual on most hosts)")
	}
}

func TestHostOnline(t *testing.T) {
	if !HostOnline() {
		t.Error("HostOnline should be true on the running machine")
	}
}
