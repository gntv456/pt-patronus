package sysinfo

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

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

// TestCollectDisks_ExtraPath 验证 extraPath（宿主 bind mount 目录）被采集进 Disks。
func TestCollectDisks_ExtraPath(t *testing.T) {
	tmp := t.TempDir()
	disks := collectDisks([]string{tmp})
	found := false
	for _, d := range disks {
		if d.Device == tmp && d.Total > 0 {
			found = true
		}
	}
	if !found {
		t.Errorf("extraPath %s 未被采集: %+v", tmp, disks)
	}
}

// TestCollectDisks_Dedup 同文件系统多个子目录应去重（dedupKey 优先 fsid——btrfs 子卷同池共享 fsid，
// 退化 st_dev），仅 Linux 验证。
func TestCollectDisks_Dedup(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("st_dev 去重仅 Linux 验证（devOf 在其他平台返 0 不去重）")
	}
	base := t.TempDir()
	sub1 := filepath.Join(base, "a")
	sub2 := filepath.Join(base, "b")
	if err := os.MkdirAll(sub1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sub2, 0o755); err != nil {
		t.Fatal(err)
	}
	disks := collectDisks([]string{sub1, sub2})
	count := 0
	for _, d := range disks {
		if d.Device == sub1 || d.Device == sub2 {
			count++
		}
	}
	if count > 1 {
		t.Errorf("同文件系统两子目录未去重，count=%d（dedupKey 应识别为同一 fsid）", count)
	}
}
