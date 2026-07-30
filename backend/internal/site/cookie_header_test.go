package site

import "testing"

func TestMergeCookieHeaderPreservesBaseKeys(t *testing.T) {
	base := "c_secure_uid=1; c_secure_pass=old; sess=a"
	overlay := "sess=b; extra=1"
	got := MergeCookieHeader(base, overlay)
	for _, want := range []string{"c_secure_uid=1", "c_secure_pass=old", "sess=b", "extra=1"} {
		if !containsKV(got, want) {
			t.Fatalf("MergeCookieHeader=%q missing %q", got, want)
		}
	}
}

func TestMergeCookieHeaderEmptyOverlay(t *testing.T) {
	base := "a=1; b=2"
	if got := MergeCookieHeader(base, ""); got != base {
		t.Fatalf("empty overlay: got %q want %q", got, base)
	}
}

func TestCookieHeaderChangedIgnoresOrder(t *testing.T) {
	a := "b=2; a=1"
	b := "a=1; b=2"
	if CookieHeaderChanged(a, b) {
		t.Fatal("order-only change should not count")
	}
	if !CookieHeaderChanged(a, "a=1; b=3") {
		t.Fatal("value change should count")
	}
}

func containsKV(header, kv string) bool {
	m := ParseCookieHeader(header)
	k, v, ok := splitKV(kv)
	if !ok {
		return false
	}
	return m[k] == v
}

func splitKV(kv string) (string, string, bool) {
	for idx := 0; idx < len(kv); idx++ {
		if kv[idx] == '=' {
			if idx == 0 {
				return "", "", false
			}
			return kv[:idx], kv[idx+1:], true
		}
	}
	return "", "", false
}
