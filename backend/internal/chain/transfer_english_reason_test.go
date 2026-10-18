package chain

// #349 整理链层还有两句英文原文会直接进界面/AI 上下文（#52 的尾，此前被误判为死代码）。
//
// transfer.go:246  `if hash == "" { return nil, fmt.Errorf("empty hash") }`
//
// 上游判断「这是死代码，handler 已用 FieldRequired 拦掉空 hash」——**这个判断是错的**。
// ManualOrganize 有三条调用路径，只有一条经过 handler：
//
//	api/v1/transfer.go:150  Organize  → 先 if in.Hash == "" → FieldRequired（拦住了）
//	app/agent_tools.go:147  transfer_organize → argString(a,"hash","") 直接透传，
//	                       **没有这层拦截**，且 Handler 返回的 err 由 mcp/server.go
//	                       原样转成 isError=true 的内容块交给 LLM。
//
// 于是 AI 调 transfer_organize 漏了 hash（或传空串）时，模型看到的失败原因就是
// 光秃秃的英文 `empty hash` —— 既没有字段名也没有下一步，模型只能反复瞎试。
//
// 同组的 transfer.go:250 `fmt.Errorf("downloader %d: %w", dlID, err)` 同样是英文
// 句式；它 %w 保住了底层原因，errtext 能翻，但「downloader 2: 」这段英文仍会拼在
// 中文前面漏出去（text 包只对已识别的哨兵整体替换，对普通包装只透传）。
//
// 修法：两句都改成能照着做的中文，并说明为什么不是死代码。

import (
	"os"
	"strings"
	"testing"
)

// TestResolveAndProcessReasonIsChinese 链层自己产出的失败原因不许再是英文原文。
func TestResolveAndProcessReasonIsChinese(t *testing.T) {
	raw, err := os.ReadFile("transfer.go")
	if err != nil {
		t.Fatalf("读不到 transfer.go：%v", err)
	}
	code := stripCodeComments(string(raw))
	from := strings.Index(code, "func (c *TransferChain) resolveAndProcessResult")
	if from < 0 {
		t.Fatal("找不到 resolveAndProcessResult（判据没有落点）")
	}
	tail := code[from:]
	if n := strings.Index(tail[1:], "\nfunc "); n > 0 {
		tail = tail[:1+n]
	}

	// 英文原文不许再出现。
	for _, banned := range []string{`fmt.Errorf("empty hash")`, `fmt.Errorf("downloader %d`} {
		if strings.Contains(tail, banned) {
			t.Errorf("链层不许再产英文原文 %q：它会经 MCP 工具的 isError 内容块原样进 LLM 上下文",
				banned)
		}
	}
	// 两处都要有中文，且空 hash 那条要说清缺的是哪个字段。
	if !strings.Contains(tail, "种子 hash") || !strings.Contains(tail, "必填") {
		t.Errorf("空 hash 要说清「种子 hash 必填」，got %q", tailLinesWith(tail, "hash == \"\""))
	}
	// 下载器取不到那条也要有中文（%w 保留底层原因，但外层句式不能是英文）。
	if !strings.Contains(tail, "下载器") {
		t.Errorf("下载器取不到要有中文外层句式，got %q", tailLinesWith(tail, "dlMgr.Get"))
	}
}

// TestManualOrganizeEmptyHashIsNotDeadCode 正控：空 hash 这条出口确实可达
// （MCP 工具不经过 handler 的 FieldRequired），所以它不是死代码、必须给中文。
func TestManualOrganizeEmptyHashIsNotDeadCode(t *testing.T) {
	raw, err := os.ReadFile("transfer.go")
	if err != nil {
		t.Fatalf("读不到 transfer.go：%v", err)
	}
	// ManualOrganize 原样透传给 resolveAndProcessResult，不在链层拦空 hash。
	if !strings.Contains(stripCodeComments(string(raw)), "func (c *TransferChain) ManualOrganize") {
		t.Fatal("找不到 ManualOrganize（判据没有落点）")
	}
	// 说明白：MCP 工具那条路不经过 handler 的 FieldRequired，所以链层这一档必须自己说人话。
	src, err := os.ReadFile("transfer.go")
	if err != nil {
		t.Fatalf("读不到 transfer.go：%v", err)
	}
	if !strings.Contains(stripCodeComments(string(src)), "必填") {
		t.Error("空 hash 分支必须自带中文说明（它经 MCP 工具直接给 LLM，handler 那层拦不住）")
	}
}
