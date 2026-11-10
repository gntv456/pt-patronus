// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

// MCP / Agent 工具面的错误出口同样必须走 mcpErr：mcp.Server 把 Handler 返回的
// err 原样拼进 isError 内容块交给模型，链层/DB 的英文原文（"no such table"、
// "dial tcp…"）到这里对模型就是一句没有下文的谜，只能盲目重试。
//
// 同包已有 plugin_host_* 的同款护栏（host_tool_err_translate_test.go），本文件
// 把它扩到公开 MCP 工具（mcp_tools.go）与 Agent 工具（agent_tools.go）——
// 这两处此前有 13 个 `return "", err` 没被覆盖。

import (
	"os"
	"strings"
	"testing"
)

func TestMCPToolsUseMcpErr(t *testing.T) {
	files := []string{"mcp_tools.go", "agent_tools.go"}
	total, raw := 0, 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读不到 %s：%v", f, err)
		}
		for _, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			var outlet, translated bool
			switch {
			case trimmed == `return "", mcpErr(err)`:
				outlet, translated = true, true
			case trimmed == `return "", err`:
				outlet = true
			}
			if !outlet {
				continue
			}
			total++
			if !translated {
				raw++
			}
		}
	}
	if raw > 0 {
		t.Errorf("还有 %d/%d 处工具出口原样透传 err：英文原因会直接进模型上下文", raw, total)
	}
	// 防止判据空转：形状变了（例如改成 return nil, …）就必须同步这条护栏。
	if total < 13 {
		t.Errorf("判据只数到 %d 处出口（want ≥13）：形状变了的话请同步判据", total)
	}
}
