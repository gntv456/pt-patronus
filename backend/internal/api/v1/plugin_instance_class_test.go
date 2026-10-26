package v1

import (
	"os"
	"strings"
	"testing"
)

// 分身（插件实例）这几条出口必须共用 instanceErr 的分档。
// 文件里 marketOperErr 的注释就写着「同一个失败在两条路上必须显示同一句话」，
// 但 SavePluginInstanceConfig / ReorderPluginInstances 当时各自写了
// errcode.Respond(c, errcode.InvalidInput.WithErr(err))：
// 市场运营层没装配（该去开配置，重试无用）被说成「请求参数有误，请检查输入」，
// 用户只会反复点保存。这条护栏防止再有人复制那一句。
func TestInstanceRoutesShareInstanceErrClassification(t *testing.T) {
	src, err := os.ReadFile("plugin_market.go")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	calls := []string{"h.manager.SavePluginInstanceConfig(", "h.manager.ReorderPluginInstances(", "h.manager.CreateInstance(", "h.manager.DeletePluginInstance("}
	checked, bare := 0, []string{}
	for i, l := range lines {
		if !strings.Contains(l, "if err := ") {
			continue
		}
		if !strings.Contains(l, "h.manager.") {
			continue
		}
		if !strings.Contains(l, "Instance") {
			continue
		}
		matched := false
		for _, c := range calls {
			if strings.Contains(l, c) {
				matched = true
			}
		}
		if !matched {
			continue
		}
		checked++
		body := strings.Join(lines[i+1:min(i+5, len(lines))], "\n")
		if strings.Contains(body, "errcode.InvalidInput.WithErr(err)") {
			bare = append(bare, "line "+itoaGuard(i+1))
		}
	}
	if checked < 3 {
		t.Fatalf("只核到 %d 处分身写调用——判据没跑起来（清单变了要跟着改）", checked)
	}
	if len(bare) != 0 {
		t.Errorf("这些分身出口还在把 manager 失败压成「参数有误」：%s", strings.Join(bare, ", "))
	}
	// 正控：旧形状必须被认出来。
	old := "if err := h.manager.ReorderPluginInstances(id, in.IDs); err != nil {\n\t\terrcode.Respond(c, errcode.InvalidInput.WithErr(err))\n\t\treturn\n\t}"
	if !strings.Contains(old, "errcode.InvalidInput.WithErr(err)") {
		t.Error("判据本身失效（认不出手写旧形状）")
	}
}

func itoaGuard(n int) string {
	return string(rune('0'+n/100%10)) + string(rune('0'+n/10%10)) + string(rune('0'+n%10))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
