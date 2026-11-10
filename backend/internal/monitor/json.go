// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import "encoding/json"

// encodeJSON 序列化为 JSON 字符串，错误时返回 ""（指标序列化不应失败）。
func encodeJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

// decodeJSON 反序列化 JSON 字符串。
func decodeJSON(raw string, v any) error {
	return json.Unmarshal([]byte(raw), v)
}
