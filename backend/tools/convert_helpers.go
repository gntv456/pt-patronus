// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package main

import (
	"fmt"
	"os"
	"strings"
)

// strSliceClean 从 any 取字符串切片，过滤掉 map（含 __ 标记）元素。
func strSliceClean(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// normSelector 归一单个选择器：PT-depiler 的 "> td" 是相对当前上下文的直接子代（Sizzle 语法），
// goquery 不支持裸 ">" 开头；本项目 Extract 的 ctx 已是行（tr），去 "> " 前缀转为后代选择器等价。
func normSelector(s string) string {
	s = strings.TrimSpace(s)
	for strings.HasPrefix(s, "> ") || strings.HasPrefix(s, ">") {
		s = strings.TrimSpace(strings.TrimPrefix(s, ">"))
	}
	return s
}

// cleanSelectors 从 any 取字符串数组并归一每个选择器。
func cleanSelectors(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, e := range arr {
		if s, ok := e.(string); ok {
			if n := normSelector(s); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

func str(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func strSlice(v any) []string {
	if s, ok := v.(string); ok {
		return []string{s}
	}
	return strSliceClean(v)
}

func putStr(m map[string]any, k string, v string) {
	if v != "" {
		m[k] = v
	}
}

func putNum(m map[string]any, k string, v any) {
	if v == nil {
		return
	}
	f, ok := v.(float64)
	if !ok {
		return // 非数值（表达式/对象/区间数组）：跳过，避免破坏 LevelRequirement 数值字段的 yaml 类型
	}
	if f == 0 {
		return
	}
	// 整数值存 int，避免 yaml 科学计数法（如 1e+06）
	if f == float64(int64(f)) {
		m[k] = int64(f)
	} else {
		m[k] = f
	}
}

func putSlice(m map[string]any, k string, v []string) {
	if len(v) > 0 {
		m[k] = v
	}
}

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
