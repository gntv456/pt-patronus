package site

import "strings"

// 版本比较（站点定义 min_app_version 门控用）。版本形如 "1.03.27"——按 '.'
// 分段逐段数字比较，段数不齐短侧补 0；非数字段按 0 处理（宽松，不 panic）。
// 语义与发版版本号一致：1.03.9 < 1.03.10（逐段数值比较，非字符串序）。
func CompareVersions(a, b string) int {
	pa, pb := splitVersion(a), splitVersion(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(pa) {
			av = pa[i]
		}
		if i < len(pb) {
			bv = pb[i]
		}
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	return 0
}

// splitVersion "1.03.27" → [1,3,27]；非数字段按 0（容忍 "1.03.27-beta" 尾巴段取前导数字）。
func splitVersion(v string) []int {
	out := make([]int, 0, 4)
	cur := 0
	has := false
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c >= '0' && c <= '9' {
			cur = cur*10 + int(c-'0')
			has = true
			continue
		}
		if c == '.' || c == '-' || c == '_' {
			out = append(out, cur)
			cur, has = 0, false
			continue
		}
		// 其它字符（字母等）终止当前数字段
		if has {
			out = append(out, cur)
			cur, has = 0, false
		}
	}
	out = append(out, cur)
	return out
}

// VersionSatisfiesMin 报告 appVersion 是否满足 min 要求（min 为空 = 无要求恒满足）。
// 满足语义：appVersion >= min。
func VersionSatisfiesMin(appVersion, min string) bool {
	min = strings.TrimSpace(min)
	if min == "" {
		return true
	}
	appVersion = strings.TrimSpace(appVersion)
	if appVersion == "" {
		// 无版本上下文（内置定义随版本发布/测试注入）：不门控。
		return true
	}
	return CompareVersions(appVersion, min) >= 0
}
