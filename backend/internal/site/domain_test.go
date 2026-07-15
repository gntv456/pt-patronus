package site

import "testing"

// TestRegisteredDomain 验证注册域（eTLD+1）提取——站点唯一性 key 的基础。
// 搬自 chain/cookiecloud_test.go（函数已下沉为本包导出函数）。
func TestRegisteredDomain(t *testing.T) {
	cases := map[string]string{
		"https://www.demo.pt/index.php": "demo.pt",
		"www.demo.pt":                   "demo.pt",
		".demo.pt":                      "demo.pt",
		"https://sub.co.uk/x":           "sub.co.uk", // co.uk 是公共后缀 → eTLD+1 = sub.co.uk（非 co.uk）
		"https://hxpt.org":              "hxpt.org",
		"https://www.hxpt.org/":         "hxpt.org", // 子域 / trailing-slash 不影响注册域（去重依据）
		"https://kp.m-team.cc":          "m-team.cc",
		"https://m-team.cc":             "m-team.cc", // 同注册域不同子域 → 同一站点
	}
	for in, want := range cases {
		if got := RegisteredDomain(in); got != want {
			t.Errorf("RegisteredDomain(%q) = %q, want %q", in, got, want)
		}
	}
}
