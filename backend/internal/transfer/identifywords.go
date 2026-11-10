// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package transfer — 识别词（自定义词表）：整理识别前对种子名/文件名做「替换/前缀剥离」
// 规整，让无法识别的文件名变得可识别。对标 MSaber 识别词（本地自定义部分）。
//
// 存储：SystemConfig key "transfer.identify_words"（JSON 数组，有序生效）。
// 生效点：searchKeyword（识别关键字清洗）与 ParseMeta 前的原始名规整——
// 本文件在 Engine 外挂：Engine 每次识别经 IdentifyReplacer 现取（热改即时生效）。
package transfer

import (
	"encoding/json"
	"strings"
)

// IdentifyWord 单条识别词。Kind:
//
//	replace：把 From（子串/正则禁用，纯子串）替换为 To（To 空=删除 From）
//	trim    ：删除 From 前缀（对开头命中的 From 整段剥离）
type IdentifyWord struct {
	Kind string `json:"kind"` // replace | trim
	From string `json:"from"`
	To   string `json:"to"`
	Note string `json:"note,omitempty"`
}

const identifyWordsKey = "transfer.identify_words"

// replacerCache 词表编译缓存（词表变更换版本失效）。
var ()

// LoadIdentifyWords 读词表（解析失败回空表，绝不阻断整理）。
func LoadIdentifyWords(kv KVStore) []IdentifyWord {
	if kv == nil {
		return nil
	}
	raw, err := kv.Get(identifyWordsKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return nil
	}
	var ws []IdentifyWord
	if json.Unmarshal([]byte(raw), &ws) != nil {
		return nil
	}
	return ws
}

// SaveIdentifyWords 写词表。
func SaveIdentifyWords(kv KVStore, ws []IdentifyWord) error {
	b, err := json.Marshal(ws)
	if err != nil {
		return err
	}
	return kv.Set(identifyWordsKey, string(b))
}

// ApplyIdentifyWords 按词表规整名称（replace 全部子串替换 + trim 前缀剥离；有序）。
func ApplyIdentifyWords(ws []IdentifyWord, name string) string {
	if name == "" {
		return name
	}
	for _, w := range ws {
		switch w.Kind {
		case "trim":
			// 前缀剥离（含其后紧邻的分隔符，防残留 " - " 破坏识别）
			if strings.HasPrefix(name, w.From) {
				name = strings.TrimLeft(name[len(w.From):], " -_.")
			}
		default: // replace（含 To 空=删除）
			name = strings.ReplaceAll(name, w.From, w.To)
		}
	}
	return name
}

// identifyReplacer 编译缓存版（整理高频路径用）：版本不一致时重编。
