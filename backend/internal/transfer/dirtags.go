// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package transfer —— 目录标签（DirTag）：一处定义目录，跨模块引用。
//
// 订阅保存路径 / STRM 生成目录 / 云端整理目标 等场景引用
// 「标签名」而非重复填路径字符串。标签 → 目录的映射存 SystemConfig（单一真源），
// 各模块运行时解析（改标签映射全局生效，无需逐处改配置）。
package transfer

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// DirTag 目录标签（name 全局唯一；cloud 型指网盘内路径，local 型指本机路径）。
type DirTag struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`               // local | cloud
	DriveID uint   `json:"drive_id,omitempty"` // cloud 型：目标网盘
	Path    string `json:"path"`               // 目录路径（local=本机；cloud=网盘内）
	Note    string `json:"note,omitempty"`
}

const dirTagsKey = "transfer.dir_tags"

// LoadDirTags 读标签表。
func LoadDirTags(kv KVStore) []DirTag {
	if kv == nil {
		return nil
	}
	raw, err := kv.Get(dirTagsKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var ts []DirTag
	if json.Unmarshal([]byte(raw), &ts) != nil {
		return nil
	}
	return ts
}

// SaveDirTags 写标签表（name 查重）。
func SaveDirTags(kv KVStore, ts []DirTag) error {
	seen := map[string]bool{}
	for _, t := range ts {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			return fmt.Errorf("标签名不能为空")
		}
		if seen[name] {
			return fmt.Errorf("标签名重复: %s", name)
		}
		seen[name] = true
		if t.Kind != "local" && t.Kind != "cloud" {
			return fmt.Errorf("标签 %s 类型非法（local|cloud）", name)
		}
		if strings.TrimSpace(t.Path) == "" {
			return fmt.Errorf("标签 %s 路径为空", name)
		}
	}
	b, err := json.Marshal(ts)
	if err != nil {
		return err
	}
	return kv.Set(dirTagsKey, string(b))
}

// ResolveDirTag 按名解析标签；不存在返回错误（显式失败优于静默落到错误路径）。
func ResolveDirTag(kv KVStore, name string) (*DirTag, error) {
	for _, t := range LoadDirTags(kv) {
		if t.Name == name {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("目录标签不存在: %s", name)
}

// SortDirTags 名称序（UI 稳定展示）。
func SortDirTags(ts []DirTag) {
	sort.Slice(ts, func(i, j int) bool { return ts[i].Name < ts[j].Name })
}

// NormalizeDirPath local 型路径规整（跨平台分隔符）。
func NormalizeDirPath(p string) string {
	return filepath.Clean(strings.ReplaceAll(strings.TrimSpace(p), "\\", "/"))
}
