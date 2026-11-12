// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package filter

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ---------- 全局绑定 ----------

// GetBindings 读取某全局绑定（组名列表）。
func (s *Store) GetBindings(key string) ([]string, error) {
	v, err := s.op.Get(key)
	if err != nil {
		return nil, err
	}
	if v == "" {
		return nil, nil
	}
	var names []string
	if err := json.Unmarshal([]byte(v), &names); err != nil {
		return nil, fmt.Errorf("解析绑定 %s 失败：%w", key, err)
	}
	return names, nil
}

// SetBindings 写某全局绑定。
func (s *Store) SetBindings(key string, names []string) error {
	b, err := json.Marshal(names)
	if err != nil {
		return err
	}
	return s.op.Set(key, string(b))
}

// ---------- 规则集与组解析 ----------

// RuleSet 返回运行期规则集 = 深拷贝内置 + 自定义覆盖。
func (s *Store) RuleSet() (map[string]Rule, error) {
	out := make(map[string]Rule, len(BuiltinRuleSet))
	for _, id := range builtinOrder {
		out[id] = cloneRule(BuiltinRuleSet[id])
	}
	customs, err := s.GetCustomRules()
	if err != nil {
		return nil, err
	}
	for _, c := range customs {
		out[c.ID] = c // 自定义覆盖内置
	}
	return out, nil
}

// KnownIDs 返回内置 ∪ 自定义的 id 集合（供 ValidateRuleString）。
func (s *Store) KnownIDs() (map[string]bool, error) {
	rs, err := s.RuleSet()
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(rs))
	for id := range rs {
		m[id] = true
	}
	return m, nil
}

// ResolveGroups 按 media_type/category 筛选适用组；names 非空时再按名过滤。
// 按 media_type 取规则组：
//   - 组 media_type 空 → 适用（全局）
//   - 组 media_type 命中媒体类型且 category 空 → 适用
//   - 组 category 命中媒体 category → 适用（最具体）
func (s *Store) ResolveGroups(media *MediaInfo, names []string) ([]RuleGroup, error) {
	all, err := s.GetGroups()
	if err != nil {
		return nil, err
	}
	var nameSet map[string]bool
	if len(names) > 0 {
		nameSet = make(map[string]bool, len(names))
		for _, n := range names {
			nameSet[strings.TrimSpace(n)] = true
		}
	}
	out := make([]RuleGroup, 0, len(all))
	for _, g := range all {
		if nameSet != nil && !nameSet[g.Name] {
			continue
		}
		if media == nil || (media.Type == "" && media.Category == "") {
			// 无媒体信息：仅全局组（media_type 空）适用。
			if g.MediaType == "" {
				out = append(out, g)
			}
			continue
		}
		mt := mediaTypeCN(media.Type)
		switch {
		case g.MediaType == "":
			out = append(out, g)
		case g.MediaType == mt && g.Category == "":
			out = append(out, g)
		case media.Category != "" && g.Category == media.Category:
			out = append(out, g)
		}
	}
	return out, nil
}
