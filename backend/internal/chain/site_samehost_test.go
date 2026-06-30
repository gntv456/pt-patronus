package chain

import "testing"

// TestSameHost 验证同注册域判定：多段公共后缀（.co.uk）、不同子域、相对路径、
// 跨域、非 http(s) 协议、IP 回退。
func TestSameHost(t *testing.T) {
	cases := []struct {
		name        string
		base, cand  string
		want        bool
	}{
		{"same subdomain", "https://kp.m-team.cc", "https://kp.m-team.cc/details.php?id=1", true},
		{"sibling subdomain same registrable", "https://api.m-team.cc", "https://kp.m-team.cc/details.php?id=1", true},
		{"relative path allowed", "https://kp.m-team.cc", "messages.php?action=markread", true},
		{"absolute path allowed", "https://kp.m-team.cc/site/", "/messages.php?id=2", true},

		// 旧实现（取最后两段）会把这两者都判为 "co.uk" → 同域；publicsuffix 应区分。
		{"different registrable under co.uk", "https://foo.co.uk", "https://bar.co.uk/path", false},
		{"same registrable under co.uk", "https://a.foo.co.uk", "https://b.foo.co.uk/path", true},

		{"cross registrable", "https://m-team.cc", "https://example.com/details.php", false},
		{"protocol-relative cross host", "https://m-team.cc", "//evil.com/x", false},
		{"non-http scheme rejected", "https://m-team.cc", "javascript:alert(1)", false},
		{"empty base rejected", "", "https://m-team.cc/x", false},

		// 裸 IP / localhost：无公共后缀，回退精确主机比对。
		{"ip exact match", "http://127.0.0.1:8080", "http://127.0.0.1/details", true},
		{"ip mismatch", "http://127.0.0.1:8080", "http://10.0.0.1/details", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameHost(tc.base, tc.cand); got != tc.want {
				t.Fatalf("sameHost(%q, %q) = %v, want %v", tc.base, tc.cand, got, tc.want)
			}
		})
	}
}
