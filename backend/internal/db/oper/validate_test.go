package oper

import "testing"

func TestValidateURLScheme(t *testing.T) {
	good := []string{
		"http://1.2.3.4:8080",
		"https://example.com/dav",
		"HTTPS://A.B.C", // scheme 大小写不敏感
	}
	bad := []string{
		"file:///etc/passwd",
		"gopher://x",
		"ftp://x",
		"javascript:alert(1)",
		"http://",       // 无 host
		"//example.com", // 无 scheme
		"",
	}
	for _, u := range good {
		if err := validateURLScheme(u); err != nil {
			t.Errorf("expected %q to pass, got %v", u, err)
		}
	}
	for _, u := range bad {
		if err := validateURLScheme(u); err == nil {
			t.Errorf("expected %q to be rejected", u)
		}
	}
}
