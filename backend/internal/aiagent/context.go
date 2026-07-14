package aiagent

import (
	"strings"

	"github.com/hxpt/ptpatronus/internal/llm"
)

// maxMessages 上下文保留的最近消息数硬上限（不含 system），作为 token 裁剪之上的二级保护。
const maxMessages = 30

// trimMessages 保留首条 system + 在 token 预算内尽可能多的最近消息，避免上下文撑爆模型 context window。
// 兼顾两层：先按 maxMessages 硬截（与原行为一致、防极端长单轮），再按 token 预算从最近向前收窄。
// 仍会去掉裁剪后以孤立 tool 消息开头的情形（provider 会拒绝无父 assistant(tool_calls) 的 tool 结果）。
func trimMessages(msgs []llm.Message, model string, maxTokens int) []llm.Message {
	head := []llm.Message{}
	if len(msgs) > 0 && msgs[0].Role == llm.RoleSystem {
		head = []llm.Message{msgs[0]}
		msgs = msgs[1:]
	}
	// 硬上限：只看最近 maxMessages 条
	if len(msgs) > maxMessages {
		msgs = msgs[len(msgs)-maxMessages:]
	}
	budget := contextBudget(model, maxTokens)
	used := 0
	for _, m := range head {
		used += estimateTokens(m)
	}
	// 从最近向前累加 token，超出预算即确定保留起点
	startIdx := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		t := estimateTokens(msgs[i])
		if used+t > budget {
			startIdx = i + 1
			break
		}
		used += t
	}
	if startIdx >= len(msgs) && len(msgs) > 0 {
		startIdx = len(msgs) - 1 // 至少保留最后一条（如单条即超预算的极端情形）
	}
	kept := msgs[startIdx:]
	// 去孤立 tool 前缀（无对应父 assistant）
	for len(kept) > 0 && kept[0].Role == llm.RoleTool {
		kept = kept[1:]
	}
	return append(head, kept...)
}

// estimateTokens 粗估单条消息 token 数（无 tokenizer 依赖）：
// content/reasoning 按 ~3 rune/token（中英混合的保守估计），tool_calls 的 JSON 也计入，加每条固定开销。
// 用于上下文裁剪的预算控制——高估偏保守（宁可多裁），不会因低估而撑爆。
func estimateTokens(m llm.Message) int {
	const overhead = 4
	n := overhead
	n += len([]rune(m.Content)) / 3
	n += len([]rune(m.Reasoning)) / 3
	for _, tc := range m.ToolCalls {
		n += len([]rune(tc.Name))/3 + len([]rune(tc.Arguments))/3 + overhead
	}
	return n
}

// contextBudget 返回输入上下文的 token 预算：模型窗口 − 输出(max_tokens) − 工具 schema 余量。
// 未知模型默认 32k 窗口（对主流 32k+ 模型安全）；已知大窗口模型给更大预算以支持更长会话。
func contextBudget(model string, maxTokens int) int {
	window := 32000
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "claude"):
		window = 200000
	case strings.Contains(m, "gpt-4o"), strings.Contains(m, "gpt-4.1"), strings.Contains(m, "gpt-5"),
		strings.Contains(m, "o1"), strings.Contains(m, "o3"), strings.Contains(m, "o4"):
		window = 128000
	case strings.Contains(m, "deepseek"):
		window = 64000
	case strings.Contains(m, "qwen"), strings.Contains(m, "kimi"), strings.Contains(m, "glm"), strings.Contains(m, "gemini"):
		window = 32000
	}
	reserve := maxTokens + 2048 // 输出 token + 工具 schema / system 提示余量
	if reserve > window/2 {
		reserve = window / 2 // 输出预留不超过一半窗口
	}
	budget := window - reserve
	if budget < 8000 {
		budget = 8000 // 下限：保证基本多轮
	}
	return budget
}
