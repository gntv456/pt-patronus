package agent

import (
	"runtime"
	"strings"
	"testing"
)

// resolveChecksum：下载地址里的 {arch} 由本机展开，各架构二进制哈希不同。
// 单值配置对异构机群必然「sha256 校验失败」，看起来像被篡改——必须能按架构取用，
// 取不到时说清「是哪台架构没配」，而不是让人去查供应链。
func TestResolveChecksum(t *testing.T) {
	const amd = "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111"
	const arm = "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222"

	got, err := resolveChecksum(amd, "amd64")
	if err != nil || got != amd {
		t.Fatalf("单值配置应原样使用（向后兼容），got %q err %v", got, err)
	}
	mixed := "amd64=" + amd + "; arm64=" + arm
	for _, tc := range []struct{ arch, want string }{{"amd64", amd}, {"arm64", arm}} {
		got, err := resolveChecksum(mixed, tc.arch)
		if err != nil || got != tc.want {
			t.Errorf("arch=%s got %q err %v", tc.arch, got, err)
		}
	}
	// 机群里出现没配过的架构（如 arm 软浮点）：要拒绝，并点名缺哪个架构。
	_, err = resolveChecksum(mixed, "arm")
	if err == nil {
		t.Fatal("未覆盖的架构必须拒绝，不能拿别的架构哈希去校验")
	}
	if !strings.Contains(err.Error(), "arm") || !strings.Contains(err.Error(), "amd64") {
		t.Errorf("原因要点名缺失架构与已配置架构，got %q", err)
	}
	if _, err := resolveChecksum("", runtime.GOARCH); err == nil {
		t.Error("空校验值不得通过（无校验自更新等于把执行权交给下载源）")
	}
	// 反例：不能把「配置串本身」当哈希返回
	if g, _ := resolveChecksum(mixed, "mips64"); g == mixed || g != "" {
		t.Errorf("缺失架构时不能返回原始配置串当哈希，got %q", g)
	}
}
