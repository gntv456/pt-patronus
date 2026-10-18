package app

import (
	"os"
	"strings"
	"testing"
)

// #326 接线护栏：启动期体检必须真的被调用，且调用点覆盖「内置 + 外部目录」两条加载路径。
// 只测 site.LintFilterPatterns 本身不够——它没被装配就等于没有。
func TestStartupLintsSiteFilterPatterns(t *testing.T) {
	src, err := os.ReadFile("app_setup.go")
	if err != nil {
		t.Fatalf("读不到 app_setup.go：%v", err)
	}
	var code []string
	for _, l := range strings.Split(string(src), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "//") {
			continue
		}
		code = append(code, l)
	}
	c := strings.Join(code, "\n")

	i := strings.Index(c, "loadSiteDefinitions(registry, defsDir, defsDirExplicit)")
	if i < 0 {
		t.Fatalf("找不到 loadSiteDefinitions 调用点（判据成了空的）")
	}
	window := c[i : i+min(200, len(c)-i)]
	if !strings.Contains(window, "warnBrokenSiteFilterPatterns(registry)") {
		t.Errorf("体检必须紧跟在定义加载之后（覆盖内置与外部两条路径）：实际 %q", strings.TrimSpace(window))
	}

	fn := strings.Index(c, "func warnBrokenSiteFilterPatterns")
	if fn < 0 {
		t.Fatalf("找不到 warnBrokenSiteFilterPatterns")
	}
	body := c[fn:]
	if j := strings.Index(body[1:], "\nfunc "); j > 0 {
		body = body[:1+j]
	}
	if !strings.Contains(body, "registry.RecordFilterPatternDiagnostics()") {
		t.Errorf("体检函数必须真跑注册表里的定义并记账：%q", body)
	}
	// 级别要连串一起钉：只 Contains("LintFilterPatterns") 会放行「扫了但不出声」的实现。
	if !strings.Contains(body, `slog.Warn("site filter regexp does not compile`) {
		t.Errorf("必须 Warn 出声（Debug 级默认看不见）：%q", body)
	}
	for _, k := range []string{`"site"`, `"path"`, `"pattern"`, `"err"`} {
		if !strings.Contains(body, k) {
			t.Errorf("留痕必须带定位信息 %s，否则日志里只知道有条规则坏了：%q", k, body)
		}
	}
}
