package chain

import (
	"os"
	"strings"
	"testing"
)

// #325 「逐条循环里单项失败被裸 continue 吃掉」两例：
// ①工作台媒体概览——一台服务器的库列表读不出（外层支只有 Debug，默认日志级别等于没留痕），
//   单个库读不出（内层支裸 continue）⇒ 界面显示「0 个库 / 少一部影片」，与「真的是空的」同形。
// ②注册检查——UpdateStatus 写失败裸 continue ⇒ 这一站的开/关没落账、也不进本轮结果清单，
//   界面上像「没有这个站」而不是「这次没更新」。
// 判据取自同文件兄弟分支的留痕方式（A/B），不是我以为「应该有日志」。

func TestWorkbenchLibraryReadFailureIsLogged(t *testing.T) {
	body := guardFuncBody(t, "dashboard_workbench.go", "func (c *MediaServerChain) MediaLibraryOverview")

	i := strings.Index(body, "libs, err := cli.Libraries(ctx)")
	if i < 0 {
		t.Fatalf("找不到 Libraries 读取那一支（下面的判据就成了空的）")
	}
	outer := body[i:]
	outer = outer[:min(320, len(outer))]
	if !strings.Contains(outer, `slog.Warn("workbench: media server libraries failed`) {
		t.Errorf("整台媒体的库读不出必须 Warn（Debug 级默认看不见，界面只会显示 0 个库）：%q", firstCodeLines(outer, 3))
	}

	j := strings.Index(body, "items, err := cli.LibraryItems(ctx, lib.ID)")
	if j < 0 {
		t.Fatalf("找不到 LibraryItems 读取那一支")
	}
	inner := body[j:]
	inner = inner[:min(320, len(inner))]
	if !strings.Contains(inner, `slog.Warn("workbench: library items failed`) {
		t.Errorf("单个库读不出必须 Warn（否则总数被静默少算一个库，与空库同形）：%q", firstCodeLines(inner, 3))
	}
	if strings.Contains(inner, "if err != nil {\n\t\t\t\tcontinue") {
		t.Errorf("这一支不许退回裸 continue：%q", firstCodeLines(inner, 4))
	}
}

func TestRegistrationStatusWriteFailureIsLogged(t *testing.T) {
	body := guardFuncBody(t, "registration.go", "func (c *RegistrationChain) CheckAllAndUpdate")

	i := strings.Index(body, "prev, err := c.regOp.UpdateStatus(w.ID, open)")
	if i < 0 {
		t.Fatalf("找不到 UpdateStatus 那一支")
	}
	branch := body[i:]
	branch = branch[:min(300, len(branch))]
	if !strings.Contains(branch, `slog.Warn("registration: update status failed`) {
		t.Errorf("状态写失败必须留痕（否则这一站的开/关没落账，界面上像「没这个站」）：%q", firstCodeLines(branch, 3))
	}
	if strings.Contains(branch, "if err != nil {\n\t\t\tcontinue") {
		t.Errorf("这一支不许退回裸 continue：%q", firstCodeLines(branch, 4))
	}
}

// guardFuncBody 取出某个函数的体（剥掉整行注释，切到下一个顶层 func 前），
// 避免同文件别处的 slog 把这一支洗白。
func guardFuncBody(t *testing.T, file, marker string) string {
	t.Helper()
	src, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("读不到 %s：%v", file, err)
	}
	code := stripStatComments(string(src))
	from := strings.Index(code, marker)
	if from < 0 {
		t.Fatalf("%s 里找不到 %q", file, marker)
	}
	rest := code[from+1:]
	to := strings.Index(rest, "\nfunc ")
	body := code[from:]
	if to > 0 {
		body = code[from : from+1+to]
	}
	return body
}
