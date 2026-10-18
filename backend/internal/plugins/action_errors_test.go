package plugins

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// 哨兵必须真的挂在链上：API 层只认 errors.Is，接不住就等于没分桶。
func TestUnknownActionErrCarriesSentinel(t *testing.T) {
	err := unknownActionErr("ping")
	if !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("errors.Is 判不到哨兵: %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "ping") || !strings.Contains(msg, "插件面板") {
		t.Errorf("界面要拿到动作名和下一步: %q", msg)
	}
	if strings.HasPrefix(msg, "unknown action") {
		t.Errorf("英文原文不该顶在人话前面: %q", msg)
	}
}

// 正控：本包所有「动作不认识」的出口都必须经 unknownActionErr。
// 此前这些位置各写各的（英文 19 处 + 中文 2 处），API 层无法区分「动作名写错」
// 与「插件跑挂」，两者一起被兜底成「请求参数有误」。
func TestUnknownActionSitesAllRouted(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var left, routed int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(e.Name()) // 本包目录内的文件名字面量，非外部输入
		if err != nil {
			t.Fatal(err)
		}
		src := stripGoComments(string(b))
		left += strings.Count(src, `"unknown action`) + strings.Count(src, `"未知动作`)
		routed += strings.Count(src, "unknownActionErr(")
	}
	if left != 0 {
		t.Errorf("还有 %d 处自己拼「unknown action」的文案：没带哨兵就会被兜底成「参数有误」", left)
	}
	if routed < 20 {
		t.Errorf("经 unknownActionErr 的出口只有 %d 处，怀疑漏改（本轮记录 21 处）", routed)
	}
}

// stripGoComments 剥掉 // 注释，免得我自己写的说明文字命中判据。
func stripGoComments(src string) string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
