package v1

import (
	"encoding/json"
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

// shellQuote 用单引号包裹，转义内部单引号，防止命令注入。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
