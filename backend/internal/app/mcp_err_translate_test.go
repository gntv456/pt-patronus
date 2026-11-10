// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

// #350 MCP/AI 工具的失败原因原样透传给 LLM，没有任何翻译（此前无任何扫描覆盖这条出口）。
//
// agent_tools*.go 里有 16 处写成 `return mcpJSON(x), err` —— err 从 chain/oper 一路
// 原样返回。mcp/server.go 再把它转成 isError=true 的内容块交给模型。
//
// 于是链层任何一句英文原文都会直接进 LLM 上下文：
//
//	transfer.go 的 `empty hash`（#349 刚修）
//	各 oper 的 fmt.Errorf("history %d has no download hash to redo")
//	「record not found」（gorm 原文；errtext 已能翻，但这里压根没调）
//
// 模型看到的是 "history 5 has no download hash to redo" 或 "record not found"，
// 既不知道该改什么参数，也不知道下一步做什么，只能反复瞎试同一把工具。
//
// 修法：加统一出口 mcpErr(err) —— 走 errtext.Text（已有的单一来源：识别得了的哨兵
// 翻成可操作中文，识别不了的自产/适配器原文原样透传），16 处全部改走它。
// 刻意不新造翻译表：errtext 已是全仓文案的单一来源，再写一份必然漂移。

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/errtext"
	"gorm.io/gorm"
)

// TestMcpErrTranslatesKnownSentinels 已识别的哨兵必须翻成可操作的中文。
func TestMcpErrTranslatesKnownSentinels(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
	}{
		{gorm.ErrRecordNotFound, "目标不存在"},
		{errors.New("该站点登录态已失效，请更新 Cookie"), "该站点登录态已失效"}, // 适配器原文原样透传
	} {
		got := mcpErr(c.err)
		if got == nil {
			t.Errorf("mcpErr(%v) = nil, want 含 %q 的错误", c.err, c.want)
			continue
		}
		if !strings.Contains(got.Error(), c.want) {
			t.Errorf("mcpErr(%v) = %q, want 含 %q", c.err, got.Error(), c.want)
		}
	}
}

// TestMcpErrKeepsAdapterText 适配器/下载器自己的原文是用户唯一的线索，不许被改写。
func TestMcpErrKeepsAdapterText(t *testing.T) {
	own := errors.New("qBittorrent: 端口被占用")
	if got := mcpErr(own); got == nil || got.Error() != own.Error() {
		t.Errorf("自产原文必须原样透传，got %v want %q", got, own.Error())
	}
}

// TestMcpErrHandlesNil nil 必须回 nil error，不许变成 "<nil>" 把成功报成失败。
func TestMcpErrHandlesNil(t *testing.T) {
	if got := mcpErr(nil); got != nil {
		t.Errorf("mcpErr(nil) = %v, want nil（成功路径不许被报成失败）", got)
	}
}

// TestMcpErrDelegatesToErrtext mcpErr 必须与 errtext.Text 同口径，不许自己另写翻译表
// （另写一份必然与 errtext 漂移——那是本仓反复出现的「文案没有单一来源」）。
func TestMcpErrDelegatesToErrtext(t *testing.T) {
	for _, sentinel := range []error{
		gorm.ErrRecordNotFound,
		errors.New("some adapter text"),
	} {
		want := errtext.Text(sentinel)
		got := mcpErr(sentinel)
		if got == nil || got.Error() != want {
			t.Errorf("mcpErr(%v) = %v, want %q（须与 errtext.Text 同口径）", sentinel, got, want)
		}
	}
}

// TestAgentToolsUseMcpErr 16 处直透传出口都要改走 mcpErr，不许再裸 return err。
func TestAgentToolsUseMcpErr(t *testing.T) {
	total, translated := 0, 0
	for _, f := range []string{"agent_tools.go", "agent_tools_ext.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("读不到 %s：%v", f, err)
		}
		for _, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			// 形状：`return mcpJSON(<x>), <错误表达式>` —— 无论那个表达式是裸 err
			// 还是 mcpErr(err)，都要数进来，否则改造后计数会归零、判据变成空转。
			if !strings.HasPrefix(trimmed, "return mcpJSON(") {
				continue
			}
			rest := strings.TrimPrefix(trimmed, "return mcpJSON(")
			if !strings.Contains(rest, "), ") || !strings.HasSuffix(rest, "err") && !strings.HasSuffix(rest, "mcpErr(err)") {
				continue
			}
			total++
			if strings.HasSuffix(trimmed, ", mcpErr(err)") {
				translated++
			}
		}
	}
	if total-translated > 0 {
		t.Errorf("还有 %d/%d 处 `return mcpJSON(x), err` 原样透传：英文原因会直接进 LLM 上下文",
			total-translated, total)
	}
	if total < 16 {
		t.Errorf("判据只数到 %d 处出口（want ≥16）：形状变了的话请同步判据", total)
	}
}
