// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
	"fmt"
	"strings"
)

// splitDockerSections 把 "...\n---IMG---\n...\n---VOL---\n..." 拆成段。
func splitDockerSections(stdout string) map[string]string {
	sections := map[string]string{}
	key := ""
	var buf strings.Builder
	for _, line := range strings.Split(stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "---") && strings.HasSuffix(trimmed, "---") && len(trimmed) > 6 {
			sections[key] = buf.String()
			key = strings.TrimSuffix(strings.TrimPrefix(trimmed, "---"), "---")
			buf.Reset()
			continue
		}
		if trimmed == "" && buf.Len() == 0 {
			continue
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	sections[key] = buf.String()
	return sections
}

// parseJSONLines 把每行一个 JSON 对象的输出解析成切片；跳过空行和非法行。
func parseJSONLines[T any](raw string) []T {
	var out []T
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var item T
		if err := json.Unmarshal([]byte(line), &item); err == nil {
			out = append(out, item)
		}
	}
	return out
}

// fmtBytes 格式化字节数为人类可读字符串。
func fmtBytes(b int64) string {
	if b <= 0 {
		return "0B"
	}
	const unit = 1024
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	i := 0
	f := float64(b)
	for f >= unit && i < len(units)-1 {
		f /= unit
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%dB", b)
	}
	return fmt.Sprintf("%.1f%s", f, units[i])
}

// min 返回两个整数中的较小值。

// stripDockerLogHeader 剥离 Docker multiplexed 日志流的 8 字节头部。
func stripDockerLogHeader(raw []byte) string {
	var sb strings.Builder
	i := 0
	for i+8 <= len(raw) {
		length := int(raw[i+4])<<24 | int(raw[i+5])<<16 | int(raw[i+6])<<8 | int(raw[i+7])
		end := i + 8 + length
		if end > len(raw) {
			end = len(raw)
		}
		sb.Write(raw[i+8 : end])
		i = end
	}
	if sb.Len() == 0 {
		return string(raw)
	}
	return sb.String()
}

// strconvAtoi 轻量 atoi（避免在 handler 里 import strconv）。
func strconvAtoi(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

// shellQuote 用单引号包裹，转义内部单引号，防止命令注入。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
