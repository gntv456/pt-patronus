package entertainment

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// reTag 去 HTML 标签的固定正则（stripTags 用），包级预编译避免每次调用重编译。
var reTag = regexp.MustCompile(`<[^>]+>`)

// reCache 缓存动态拼接的正则（按 pattern 原样编译，不额外加 (?i)——调用方自行指定前缀）。
// 站点 HTML 解析正则多含 className/id/name 等动态片段，勋章合成/抽卡解析热路径上反复编译纯浪费。
var reCache sync.Map // pattern(string) → *regexp.Regexp

// compileRegex 编译 pattern 并缓存（原样，不加前缀）；pattern 假定合法（站点数据已验）。
func compileRegex(pattern string) *regexp.Regexp {
	if v, ok := reCache.Load(pattern); ok {
		return v.(*regexp.Regexp)
	}
	re := regexp.MustCompile(pattern)
	reCache.Store(pattern, re)
	return re
}

// 通用 JSON 响应小工具（九宫格/刮刮乐/财神刮刮乐等游戏复用）。

// firstGroup 取 body 中正则首个捕获组；无匹配返回空（魔力刮刮乐 buy 返 HTML 抠 scratch_id 用）。
func firstGroup(body []byte, pattern string) string {
	m := compileRegex(pattern).FindSubmatch(body)
	if len(m) >= 2 {
		return string(m[1])
	}
	return ""
}

// findGroups 取正则全部捕获组（含整体匹配）；无匹配返回 nil（魔力刮刮乐 stats 抠 剩余/上限 用）。
func findGroups(body []byte, pattern string) []string {
	m := compileRegex(pattern).FindSubmatch(body)
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
	return reTag.ReplaceAllString(s, "")
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

// formatFloat 把 float64 格式化为去掉多余 0 的字符串（strconv.FormatFloat 'f' -1 64 的简写）。
func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// numToStr 把任意数值/字符串类型的值转为干净字符串（longptNum 的逆操作；round_number/id 透传站点用）。
func numToStr(v any) string {
	switch n := v.(type) {
	case nil:
		return ""
	case string:
		return n
	case float64:
		return formatFloat(n)
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case json.Number:
		return n.String()
	}
	return fmt.Sprintf("%v", v)
}
