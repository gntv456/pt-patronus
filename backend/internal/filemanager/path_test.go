package filemanager

import "testing"

func TestResolvePath(t *testing.T) {
	cases := []struct {
		base, p, want string
	}{
		{"", "/data", "/data"},
		{"", "data/x", "/data/x"},
		{"", "../etc/passwd", "/etc/passwd"}, // 无 base：清理但仍以 / 为根
		{"/media", "movies/x.mkv", "/media/movies/x.mkv"},
		{"/media", "/media/a/b", "/media/a/b"},   // 已在 base 下，原样保留
		{"/media", "../../../etc", "/media/etc"}, // 越界尝试被约束回 base 下
		{"/media", "", "/media"},                 // 空路径 → base 本身
		{"media", "a/b", "/media/a/b"},           // base 无前导斜杠也归一
		// P0 回归：Windows 反斜杠穿越。POSIX path.Clean 不认 \，修复前 \..\..\ 原样保留
		// → 后端/agent 端解析为分隔符后逃出 base；修复（清理前 \→/）后应钳回 base 下。
		{"/media", `..\..\..\Windows\System32\hosts`, "/media/Windows/System32/hosts"},
		{"/media", `..\../etc`, "/media/etc"},
	}
	for _, c := range cases {
		got := resolvePath(c.base, c.p)
		if got != c.want {
			t.Errorf("resolvePath(%q, %q) = %q, want %q", c.base, c.p, got, c.want)
		}
	}
}
