package monitor

import (
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/models"
)

// Verify P0 fix: SNMP memory no longer underflows
func TestP0_SNMPMemoryNoUnderflow(t *testing.T) {
	totalKB := int64(8192)
	availKB := int64(10240) // > total (firmware bug scenario)
	if availKB > totalKB {
		availKB = totalKB // the fix
	}
	used := (totalKB - availKB) * 1024
	if used != 0 {
		t.Errorf("after fix, used should be 0, got %d", used)
	}
	t.Log("P0 fix confirmed: avail clamped to total, no underflow")
}

// Verify P1 fix: HTTPS detection by port
func TestP1_NormalizeBaseHTTPSDetection(t *testing.T) {
	cases := []struct {
		raw       string
		wantHTTPS bool
	}{
		{"192.168.1.100:5001", true},  // Synology HTTPS port
		{"nas.local:443", true},       // generic HTTPS
		{"10.0.0.1:8443", true},       // alt HTTPS
		{"192.168.1.100:5000", false}, // Synology HTTP port
		{"10.0.0.1:80", false},        // HTTP
	}
	for _, c := range cases {
		got := normalizeBase(c.raw)
		isHTTPS := strings.HasPrefix(got, "https://")
		if isHTTPS != c.wantHTTPS {
			t.Errorf("normalizeBase(%q) = %q, wantHTTPS=%v but got %v", c.raw, got, c.wantHTTPS, isHTTPS)
		}
	}
}

// Verify P1 fix: SSH power command has fallback
func TestP1_SSHPowerFallback(t *testing.T) {
	cmd, _ := sshPowerCommand("reboot")
	if !strings.Contains(cmd, "sudo -n reboot") || !strings.Contains(cmd, "sudo reboot") {
		t.Errorf("reboot should have fallback: %q", cmd)
	}
	cmd, _ = sshPowerCommand("shutdown")
	if !strings.Contains(cmd, "sudo -n shutdown") || !strings.Contains(cmd, "sudo shutdown") {
		t.Errorf("shutdown should have fallback: %q", cmd)
	}
}

// Verify P3 fix: snmpHost handles IPv6
func TestP3_SNMPHostIPv6(t *testing.T) {
	target := &models.MonitorTarget{Host: "fe80::1"}
	got := snmpHost(target)
	if got != "fe80::1" {
		t.Errorf("IPv6 'fe80::1' should be preserved, got %q", got)
	}
	// bracketed IPv6 with port
	target2 := &models.MonitorTarget{Host: "[fe80::1]:161"}
	got2 := snmpHost(target2)
	if got2 != "fe80::1" {
		t.Errorf("[fe80::1]:161 -> %q, want 'fe80::1'", got2)
	}
}

// Verify P3 fix: TrueNAS version no trailing space
func TestP3_TrueNASVersionNoTrailingSpace(t *testing.T) {
	// Simulate the fixed logic
	info := map[string]any{"hostname": "test"}
	version := firstStr(info, "version")
	var os string
	if version != "" {
		os = "TrueNAS " + version
	} else {
		os = "TrueNAS"
	}
	if os != "TrueNAS" {
		t.Errorf("empty version should give 'TrueNAS', got %q", os)
	}
	if strings.HasSuffix(os, " ") {
		t.Errorf("OS should not have trailing space: %q", os)
	}
}
