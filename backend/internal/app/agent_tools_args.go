package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/site"
)

// 本文件是 agent/plugin 工具面共用的入参解析与 schema 片段：
// LLM/插件给的 JSON 数字统一解成 float64，id 常被写成字符串，缺省值一律在这里收口。

// ===== 参数解析小工具（LLM 传 float64/any，需安全转换） =====

func argString(a map[string]any, key, def string) string {
	v, ok := a[key]
	if !ok || v == nil {
		return def
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// runTrigger 取工作流触发来源：显式传空串/纯空白也要回落到默认值。
// trigger 既写进运行历史，也作为 {{trigger}} 变量参与节点分支，空白值会让历史与条件同时失真。
func runTrigger(a map[string]any, def string) string {
	s := strings.TrimSpace(argString(a, "trigger", def))
	if s == "" {
		return def
	}
	return s
}

func argInt(a map[string]any, key string) int {
	v, ok := a[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	}
	return 0
}
func argIntDefault(a map[string]any, key string, def int) int {
	v, ok := a[key]
	if !ok || v == nil {
		return def
	}
	if n, ok := v.(float64); ok && n > 0 {
		return int(n)
	}
	return def
}
func argBool(a map[string]any, key string) bool {
	v, ok := a[key]
	if !ok {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}
func argStrSlice(a map[string]any, key string) []string {
	v, ok := a[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// argIDSlice 从参数里安全取 uint 数组：JSON 数字解析成 float64，插件面/测试里可能是 int/int64，
// 模型也常把 id 写成字符串（"12"）。只认 float64 的写法会把「传了站」静默筛成「一个站都没传」。
func argIDSlice(a map[string]any, key string) []uint {
	v, ok := a[key]
	if !ok || v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]uint, 0, len(arr))
	for _, e := range arr {
		switch n := e.(type) {
		case float64:
			if n > 0 {
				out = append(out, uint(n))
			}
		case int:
			if n > 0 {
				out = append(out, uint(n))
			}
		case int64:
			if n > 0 {
				out = append(out, uint(n))
			}
		case string:
			if m, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil && m > 0 {
				out = append(out, uint(m))
			}
		}
	}
	return out
}

// hashesProp 种子哈希数组属性：minItems=1 —— required 只保证键存在，模型传 hashes:[] 也能过，
// 而空列表在下载器端等于「对全部种子操作」。chain 侧已硬拒，这里让模型早一步拿到明确错误。
func hashesProp() map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1}
}

func hashesSchema() map[string]any {
	return objSchema(map[string]any{
		"downloader_id": intProp(""),
		"hashes":        hashesProp(),
	}, []string{"downloader_id", "hashes"})
}

// mailboxType 把字符串映射到 site.MailboxType（未知回 inbox）。
func mailboxType(s string) site.MailboxType {
	switch strings.ToLower(s) {
	case "sent":
		return site.MailboxSent
	case "system":
		return site.MailboxSystem
	default:
		return site.MailboxInbox
	}
}
