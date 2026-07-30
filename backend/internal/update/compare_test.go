package update

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.70.01", "0.70.02", -1},
		{"0.70.02", "0.70.01", 1},
		{"0.70.01", "0.70.1", 0}, // 前导零归一(01==1)
		{"0.70.01", "0.70.01", 0},
		{"1.2", "1.2.3", -1}, // 缺位补 0
		{"1.2.3", "1.2", 1},
		{"2.0", "1.9.9", 1},
		{"v0.70.01", "0.70.02", -1},    // v 前缀
		{"0.70.01-beta", "0.70.01", 0}, // 非数字段(beta)当 0
		{"1.10", "1.9", 1},             // 逐段数字比较(10>9)，非字典序("1.9">"1.10")
		{"0.0.1", "0.0.1+build42", 0},  // build 元数据当 0
	}
	for _, c := range cases {
		got := compareVersions(c.a, c.b)
		if got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
