// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package llmgateway

import "strings"

// defaultStreamErrMsg 是给「流被提前关闭 / 上游没给原因」用的兜底文案。
//
// 为什么要有它：agent 事件流可能带着空 Error 结束，也可能压根没有 done 帧
// （上游连接被掐、provider 回 503 且响应体为空）。这两种情况下从前网关
// 写的是 finish_reason=stop（Anthropic 侧是 stop_reason=end_turn）——
// 客户端读到的是「模型正常回答完了，只是没说话」，于是 Claude Code / OpenAI SDK
// 会把一次失败当成空回答继续往下走（#265）。
const defaultStreamErrMsg = "上游模型未返回错误原因（流被提前中断）"

// streamErrMsg 保证错误文案非空：空原因会让 error 帧没有可显示的内容，
// 等于把失败又渲染成一次「没有问题」。
func streamErrMsg(msg string) string {
	if strings.TrimSpace(msg) == "" {
		return defaultStreamErrMsg
	}
	return msg
}
