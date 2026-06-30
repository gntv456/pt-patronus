package httpclient

import (
	"net"
	"testing"
)

func TestIsPublicIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"192.168.1.1", false},
		{"172.16.0.1", false},
		{"172.31.255.255", false},
		{"172.32.0.1", true}, // 刚好超出 172.16/12
		{"169.254.169.254", false},
		{"100.64.0.1", false}, // CGNAT
		{"100.127.255.255", false},
		{"100.128.0.1", true},
		{"0.0.0.0", false},
		{"::1", false},
		{"fc00::1", false},
		{"fe80::1", false},
		{"2606:4700:4700::1111", true},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("parse %s", c.ip)
		}
		if got := IsPublicIP(ip); got != c.want {
			t.Errorf("IsPublicIP(%s) = %v, want %v", c.ip, got, c.want)
		}
	}
}

func TestAssertPublicURLScheme(t *testing.T) {
	if err := AssertPublicURL("file:///etc/passwd"); err == nil {
		t.Error("file scheme should be blocked")
	}
	if err := AssertPublicURL("ftp://example.com"); err == nil {
		t.Error("ftp scheme should be blocked")
	}
}
