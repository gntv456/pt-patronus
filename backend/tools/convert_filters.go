// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package main

import (
	"fmt"
	"strings"
)

// translateFilters 翻译 filters 数组。识别的 name 映射，匿名函数记 TODO。
func translateFilters(v any, path string, todos *[]string) []map[string]any {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, f := range arr {
		fm, ok := f.(map[string]any)
		if !ok {
			// 匿名函数 filter
			*todos = append(*todos, fmt.Sprintf("%s.filters（匿名函数）", path))
			continue
		}
		name := str(fm["name"])
		if name == "" {
			// 整个 filter 是函数对象
			if _, ok := fm["__fn"]; ok {
				*todos = append(*todos, fmt.Sprintf("%s.filters（匿名函数）", path))
			}
			continue
		}
		mapped, ok := filterNameMap()[name]
		if !ok {
			*todos = append(*todos, fmt.Sprintf("%s.filters[%s]", path, name))
			continue
		}
		entry := map[string]any{"name": mapped}
		if a := filterArgs(name, fm["args"]); a != "" {
			entry["args"] = a
		}
		out = append(out, entry)
	}
	return out
}

func filterNameMap() map[string]string {
	return map[string]string{
		"querystring":    "queryparam",
		"parseNumber":    "parse_number",
		"parseSize":      "parse_size",
		"parseTime":      "parse_time",
		"parseTTL":       "parse_time",
		"parseDuration":  "parse_time",
		"parseFuzzyTime": "parse_time",
		"divide":         "divide",
		"toLower":        "lower", "toUpper": "upper",
		"replace": "replace", "trim": "trim",
		"regexp": "regexp", "regex": "regexp",
	}
}

// filterArgs 归一 filter args：querystring 的 args 是 ["id"]，本项目 queryparam 用 "id"。
func filterArgs(name string, v any) string {
	switch name {
	case "querystring":
		if s := strSliceClean(v); len(s) > 0 {
			return s[0]
		}
	}
	return str(v)
}

// translateLevelRequirements 翻译 levelRequirements 数组 → 本项目 level_requirements。
func translateLevelRequirements(doc map[string]any) []map[string]any {
	arr, ok := doc["levelRequirements"].([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		entry := map[string]any{}
		putNum(entry, "id", m["id"])
		putStr(entry, "name", str(m["name"]))
		putSlice(entry, "name_aka", strSlice(m["nameAka"]))
		putStr(entry, "interval", str(m["interval"]))
		putStr(entry, "downloaded", str(m["downloaded"]))
		putStr(entry, "uploaded", str(m["uploaded"]))
		putNum(entry, "ratio", m["ratio"])
		putNum(entry, "bonus", m["bonus"])
		putNum(entry, "seeding_bonus", m["seedingBonus"])
		putStr(entry, "seeding_time", str(m["seedingTime"]))
		putNum(entry, "uploads", m["uploads"])
		if k, _ := m["isKept"].(bool); k {
			entry["is_kept"] = true
		}
		putStr(entry, "group_type", str(m["groupType"]))
		putStr(entry, "privilege", str(m["privilege"]))
		out = append(out, entry)
	}
	return out
}

// translateCategory：把 PT-depiler category 中 key=="cat" 的 options 转为本项目 categories。
// 其余维度（medium/codec/...）是搜索过滤，本项目暂不用。
func translateCategory(doc map[string]any) []map[string]any {
	arr, ok := doc["category"].([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := m["__ref"]; ok {
			continue // 外部常量引用（CategoryIncldead 等），跳过
		}
		if str(m["key"]) != "cat" {
			continue
		}
		opts, _ := m["options"].([]any)
		for _, o := range opts {
			om, ok := o.(map[string]any)
			if !ok {
				continue
			}
			id := fmt.Sprintf("%v", om["value"])
			out = append(out, map[string]any{"id": id, "name": str(om["name"])})
		}
		if len(out) > 0 {
			break
		}
	}
	return out
}

// translateNoLogin：noLoginAssert → no_login（best-effort）。
func translateNoLogin(doc map[string]any) map[string]any {
	nl, _ := doc["noLoginAssert"].(map[string]any)
	if nl == nil {
		return nil
	}
	out := map[string]any{}
	if s := strSliceClean(nl["httpStatusCodes"]); len(s) > 0 {
		out["http_status"] = s
	}
	if s := strSliceClean(nl["urlPatterns"]); len(s) > 0 {
		out["url_pattern"] = s
	}
	if s := strSliceClean(nl["matchSelectors"]); len(s) > 0 {
		out["selector"] = strings.Join(s, " ")
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
