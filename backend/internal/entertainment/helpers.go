package entertainment

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// 通用 JSON 响应小工具（九宫格/刮刮乐/财神刮刮乐等游戏复用）。

// firstGroup 取 body 中正则首个捕获组；无匹配返回空（魔力刮刮乐 buy 返 HTML 抠 scratch_id 用）。
func firstGroup(body []byte, pattern string) string {
	m := regexp.MustCompile(pattern).FindSubmatch(body)
	if len(m) >= 2 {
		return string(m[1])
	}
	return ""
}

// findGroups 取正则全部捕获组（含整体匹配）；无匹配返回 nil（魔力刮刮乐 stats 抠 剩余/上限 用）。
func findGroups(body []byte, pattern string) []string {
	m := regexp.MustCompile(pattern).FindSubmatch(body)
	if m == nil {
		return nil
	}
	out := make([]string, len(m))
	for i, g := range m {
		out[i] = string(g)
	}
	return out
}

// stripTags 去 HTML 标签（抠 buy 失败提示文本用）。
func stripTags(s string) string {
	return regexp.MustCompile(`<[^>]+>`).ReplaceAllString(s, "")
}

// pathTail 取 URL 最后一段（日志标识用）。
func pathTail(urlStr string) string {
	if i := strings.LastIndexByte(urlStr, '/'); i >= 0 {
		return urlStr[i+1:]
	}
	return urlStr
}

// logResp 在 Debug 级别记录站点原始响应（status + body 片段）——
// 诊断站点契约用：当站点返回 HTML 登录页/错误信封而非 JSON 时，据此定位缺什么（CSRF token / 参数 / 头）。
// 仅 debug 环境（log 级别=debug）可见。
func logResp(game, url string, resp *httpclient.Response) {
	if resp == nil {
		return
	}
	body := resp.Body
	if len(body) > 300 {
		body = body[:300]
	}
	slog.Debug("entertainment raw response", "game", game, "status", resp.StatusCode, "url", url, "body", string(body))
}

// joinPath 拼接 baseURL + 相对路径。
func joinPath(base, p string) string {
	b := strings.TrimRight(base, "/")
	p = strings.TrimLeft(p, "/")
	return b + "/" + p
}

// parseRaw 把响应体解析为 map。
func parseRaw(body []byte) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("parse game response: %w", err)
	}
	return m, nil
}

func boolField(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func strField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func numField(m map[string]any, key string) float64 {
	switch v := m[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		var f float64
		_, _ = fmt.Sscanf(v, "%f", &f)
		return f
	}
	return 0
}

// idField 取响应里的 id（scratch_id / id / scratchId 任一）。
func idField(m map[string]any) string {
	for _, k := range []string{"scratch_id", "id", "scratchId"} {
		switch v := m[k].(type) {
		case string:
			if v != "" {
				return v
			}
		case float64:
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}
