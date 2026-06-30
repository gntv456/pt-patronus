package update

import "strings"

// compareVersions 逐段整数比较版本号(移植自旧版 PTPatronus UpdateChecker.compareVersions)。
// 返回 -1/0/1 表示 a<b/a==b/a>b。非数字段当 0，缺位补 0，前导零归一。
//
// 用逐段比较而非 semver 库：版本号 "0.70.01" 含前导零，不符合严格 semver；
// 旧版/pt_mate/MoviePilot 一致采用此朴素算法，已验证可靠。
//
// 示例：("0.70.01","0.70.02")→-1  ("0.70.01","0.70.1")→0  ("1.2","1.2.3")→-1  ("2.0","1.9.9")→1
func compareVersions(a, b string) int {
	sa := splitVer(a)
	sb := splitVer(b)
	n := len(sa)
	if len(sb) > n {
		n = len(sb)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(sa) {
			x = sa[i]
		}
		if i < len(sb) {
			y = sb[i]
		}
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
	}
	return 0
}

// splitVer 把 "v0.70.1-beta+build" 拆成 [0,70,1,0,0](去 v 前缀，按 ./-/_/+ 切，
// 非数字段如 "beta" 记 0)。切片长度即版本段数，缺位由 compareVersions 补 0。
func splitVer(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.FieldsFunc(v, func(r rune) bool {
		return r == '.' || r == '-' || r == '_' || r == '+'
	})
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		out = append(out, leadingInt(p))
	}
	return out
}

// leadingInt 取字符串前导数字部分："01"→1，"70a"→70，"beta"→0。
// 前导零自然归一(01→1)，故 "0.70.01" 与 "0.70.1" 判等。
func leadingInt(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
