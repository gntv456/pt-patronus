package app

// #351 插件 Host API 的 host-only 工具同样把失败原因原样透传给插件作者。
//
// plugin_host_tools.go 的 (1) 路径（adopt registerAgentTools 的 35 个工具）
// 已随 #350 一并修复 —— 它们共用同一批闭包。那 35 个不是这次的盲区。
//
// 盲区在 (2)：plugin_host_{cloud,downloader,media,workflow}.go 里 27 处
// `return nil, err`（外加 okCount 里的透传），err 从 chain/oper 原样返回，
// 经 plugins/host_tools.go 的 t.Handler 回到插件的 CallHostTool。
//
// 读者是**插件作者**，不是 LLM——比 LLM 更需要可读的错误：作者要靠这句话
// 决定改参数还是改代码。链层任何英文原文（"record not found"、
// "history 5 has no download hash to redo"…）到这里就是一句没有下文的谜。
//
// 修法：同包内直接复用 #350 的 mcpErr（同一个包、同一份 errtext 文案），
// 不另写翻译表——两处翻译必然漂移。

import (
	"os"
	"strings"
	"testing"
)

// TestHostOnlyToolsUseMcpErr host-only 工具的错误出口都要走 mcpErr。
func TestHostOnlyToolsUseMcpErr(t *testing.T) {
	files := []string{
		"plugin_host_cloud.go", "plugin_host_downloader.go",
		"plugin_host_media.go", "plugin_host_workflow.go",
	}
	total, raw := 0, 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读不到 %s：%v", f, err)
		}
		for _, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			// 两种透传形状：`return nil, <错误表达式>` 与 `return okCount(…, <错误表达式>)`。
			// 表达式既可能是裸 err（旧形状），也可能是 mcpErr(err)（已翻译）——两种都要数进来，
			// 否则改造后计数归零、判据变成空转（它会「绿」但什么都没检）。
			var isOutlet, translated bool
			switch {
			case strings.HasPrefix(trimmed, "return nil, mcpErr(err)"):
				isOutlet, translated = true, true
			case strings.HasPrefix(trimmed, "return nil, ") && strings.HasSuffix(trimmed, ", err"):
				isOutlet = true
			case strings.HasPrefix(trimmed, "return okCount(") && strings.HasSuffix(trimmed, ", mcpErr(err))"):
				isOutlet, translated = true, true
			case strings.HasPrefix(trimmed, "return okCount(") && strings.HasSuffix(trimmed, ", err)"):
				isOutlet = true
			}
			if !isOutlet {
				continue
			}
			total++
			if !translated {
				raw++
			}
		}
	}
	if raw > 0 {
		t.Errorf("还有 %d/%d 处 host-only 工具原样透传 err：英文原因会直接回到插件作者",
			raw, total)
	}
	if total < 27 {
		t.Errorf("判据只数到 %d 处出口（want ≥27）：形状变了的话请同步判据", total)
	}
}

// TestOkCountTranslatesError 反向对照：okCount 这个「N 个成功」的包装器也必须翻译，
// 否则上面那些调用点改了、它自己又把原因漏出去。
func TestOkCountTranslatesError(t *testing.T) {
	src, err := os.ReadFile("plugin_host_downloader.go")
	if err != nil {
		t.Fatalf("读不到 plugin_host_downloader.go：%v", err)
	}
	code := string(src)
	at := strings.Index(code, "func okCount(")
	if at < 0 {
		t.Fatal("找不到 okCount（判据没有落点）")
	}
	tail := code[at:]
	if n := strings.Index(tail[1:], "\nfunc "); n > 0 {
		tail = tail[:1+n]
	}
	if strings.Contains(tail, "return nil, err") {
		t.Error("okCount 不许原样透传 err（旧形状：上面调用点改了、它自己又把英文漏出去）")
	}
	if !strings.Contains(tail, "mcpErr(err)") {
		t.Error("okCount 的失败路径要走 mcpErr")
	}
}

// TestHostToolsHaveNoEnglishRawErrors host-only 工具自己产的入参校验也不许是英文原文。
// 这三条不经过任何翻译（是 errors.New/fmt.Errorf 直接产出的），会原样回到插件作者：
//
//	plugin_host_cloud.go   "name/site_id/downloader_id are required"
//	plugin_host_workflow.go "name and url are required"
//	plugin_host_tools.go   "invalid tool input: %w"
//
// 插件作者据此判断「该改参数还是改代码」，英文原文只告诉他「出错了」。
func TestHostToolsHaveNoEnglishRawErrors(t *testing.T) {
	for _, f := range []string{"plugin_host_cloud.go", "plugin_host_workflow.go", "plugin_host_tools.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读不到 %s：%v", f, err)
		}
		for _, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			// 出口可能是 `return nil, fmt.Errorf("…")` 或 `return fmt.Errorf("…")`，
			// 两种都要匹配到——只认行首那个会把它们全漏掉（判据空转）。
			idx := strings.Index(trimmed, "fmt.Errorf(\"")
			if idx < 0 {
				idx = strings.Index(trimmed, "errors.New(\"")
			}
			if idx < 0 {
				continue
			}
			// 只看纯 ASCII 的英文句子（中文文案含汉字，不会命中）。
			rest := trimmed[idx:]
			for _, prefix := range []string{"fmt.Errorf(\"", "errors.New(\""} {
				rest = strings.TrimPrefix(rest, prefix)
			}
			if isAllASCIISentence(rest) {
				t.Errorf("%s 里有英文原文 %q：它会原样回到插件作者，该说人话", f, trimmed)
			}
		}
	}
}

// isAllASCIISentence 判断一句话里是否含汉字（不含即视为纯英文原文）。
func isAllASCIISentence(s string) bool {
	for _, r := range s {
		if r > 0x2E80 { // CJK 区起点之上即视为中文
			return false
		}
	}
	return true
}
