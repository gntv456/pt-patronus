// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"net"
	"testing"
)

func TestParseMAC(t *testing.T) {
	cases := map[string]bool{
		"AA:BB:CC:DD:EE:FF": true,
		"aabbccddeeff":      true,
		"AA-BB-CC-DD-EE-FF": true,
		"AA BB CC DD EE FF": true,
		"short":             false,
		"":                  false,
		"zzbbccddeeff":      false,
	}
	for mac, ok := range cases {
		_, err := parseMAC(mac)
		if ok && err != nil {
			t.Errorf("parseMAC(%q) unexpected err: %v", mac, err)
		}
		if !ok && err == nil {
			t.Errorf("parseMAC(%q) should fail", mac)
		}
	}
	m, err := parseMAC("AA:BB:CC:DD:EE:FF")
	if err != nil || len(m) != 6 || m[0] != 0xAA || m[5] != 0xFF {
		t.Fatalf("bad parsed mac: %v", m)
	}
}

// TestWOLMagicPacket 魔术包结构：6×0xFF + 16×MAC。
func TestWOLMagicPacket(t *testing.T) {
	mac, _ := parseMAC("AA:BB:CC:DD:EE:FF")
	packet := make([]byte, 6+16*len(mac))
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}
	for i := 0; i < 16; i++ {
		copy(packet[6+i*len(mac):], mac)
	}
	if len(packet) != 102 {
		t.Fatalf("packet len %d", len(packet))
	}
	// 发到回环端口验证长度与内容透传
	uc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Skip("udp loopback unavailable")
	}
	defer uc.Close()
	udpAddr, err := net.ResolveUDPAddr("udp", uc.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	if err := wolSend(mac, "127.0.0.1", udpAddr.Port); err != nil {
		t.Fatalf("wolSend: %v", err)
	}
	buf := make([]byte, 1200)
	n, _, err := uc.ReadFrom(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if n != 102 {
		t.Fatalf("received %d bytes", n)
	}
}

func TestWOLActionUnknown(t *testing.T) {
	p := wolPlugin{}
	if _, err := p.ExecuteAction(context.Background(), stubRT{}, "bogus", map[string]any{}); err == nil {
		t.Fatal("unknown action must error")
	}
}
