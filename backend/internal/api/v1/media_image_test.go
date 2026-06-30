package v1

import "testing"

// TestMediaImageReferer：白名单匹配 + Referer 注入 + SSRF 拒绝。
func TestMediaImageReferer(t *testing.T) {
	cases := []struct {
		host     string
		wantOK   bool
		wantRef  string
	}{
		{"img9.doubanio.com", true, "https://movie.douban.com/"},
		{"qnmob3.doubanio.com", true, "https://movie.douban.com/"},
		{"DOUBANIO.COM", true, "https://movie.douban.com/"}, // 大小写不敏感
		{"image.tmdb.org", true, ""},
		{"evil.com", false, ""},
		{"doubanio.com.attacker.io", false, ""}, // 后缀伪装不命中
		{"", false, ""},
	}
	for _, c := range cases {
		ref, ok := mediaImageReferer(c.host)
		if ok != c.wantOK || ref != c.wantRef {
			t.Errorf("mediaImageReferer(%q) = (%q,%v), want (%q,%v)", c.host, ref, ok, c.wantRef, c.wantOK)
		}
	}
}
