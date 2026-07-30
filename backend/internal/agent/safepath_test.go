package agent

import (
	"strings"
	"testing"
)

// safePath 是 agent 文件操作的路径钳制闸门（与 filemanager.resolvePath 同构但独立实现，
// 因 agent 不能 import filemanager）。该测试是 agent 包的首个测试，锁定 P0 反斜杠穿越修复。
func TestSafePath(t *testing.T) {
	cases := []struct {
		base, p, want string
	}{
		{"", "/data/x", "/data/x"},
		{"", "data/x", "/data/x"},
		{"/media", "movies/a.mkv", "/media/movies/a.mkv"},
		{"/media", "../../../etc/passwd", "/media/etc/passwd"}, // POSIX ../ 越界被钳回 base 下
		{"/media", "/etc/passwd", "/media/etc/passwd"},         // 绝对路径钳回 base
		{"/media", "", "/media"},                               // 空路径 → base 本身
		{"media", "a/b", "/media/a/b"},                         // base 无前导斜杠归一
	}
	for _, c := range cases {
		got := safePath(c.base, c.p)
		if got != c.want {
			t.Errorf("safePath(%q, %q) = %q, want %q", c.base, c.p, got, c.want)
		}
	}
}

// 反斜杠穿越是 P0 回归重点：Windows agent 端经 filepath.FromSlash 把 \ 还原为分隔符，
// 若 safePath 未先规范 \→/，POSIX path.Clean 会原样保留 \..\..\，进而在 OS 层逃出 base。
// 此测试确保任意反斜杠组合都被钳制在 base 之下、无 .. 残留。
func TestSafePathBackslashTraversal(t *testing.T) {
	const base = "/media"
	attacks := []string{
		`..\..\..\Windows\System32\drivers\etc\hosts`,
		`..\../etc/passwd`,
		`\..\..\..\secret`,
		`..\..\..`,
	}
	for _, a := range attacks {
		got := safePath(base, a)
		if got != base && !strings.HasPrefix(got, base+"/") {
			t.Errorf("safePath(%q, %q) = %q，逃出 base %q", base, a, got, base)
		}
		if strings.Contains(got, "..") {
			t.Errorf("safePath(%q, %q) = %q，含 .. 残留（未折叠干净）", base, a, got)
		}
	}
}
