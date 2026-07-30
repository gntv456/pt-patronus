package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/workflow"
)

// HTTPRequestNode 通用 HTTP 请求（图片收割机式 http.request）→ ecx.Content。
type HTTPRequestNode struct {
	hc *httpclient.Client
}

func NewHTTPRequestNode(hc *httpclient.Client) *HTTPRequestNode { return &HTTPRequestNode{hc: hc} }

func (n *HTTPRequestNode) Type() string     { return "http_request" }
func (n *HTTPRequestNode) Name() string     { return "HTTP 请求" }
func (n *HTTPRequestNode) Category() string { return "数据" }
func (n *HTTPRequestNode) Description() string {
	return "发起 GET/POST 请求，响应体写入文本载荷"
}
func (n *HTTPRequestNode) Icon() string { return "http" }
func (n *HTTPRequestNode) DefaultParams() map[string]any {
	return map[string]any{"method": "GET", "url": "", "headers": map[string]any{}, "body": "", "cookie": ""}
}
func (n *HTTPRequestNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "method", Label: "方法", Type: "select", Options: []workflow.ParamOption{
			{Label: "GET", Value: "GET"}, {Label: "POST", Value: "POST"},
		}},
		{Name: "url", Label: "URL", Type: "text", Required: true},
		{Name: "headers", Label: "请求头", Type: "kv"},
		{Name: "body", Label: "请求体", Type: "textarea", Help: "POST 用"},
		{Name: "cookie", Label: "Cookie", Type: "text"},
	}
}

func (n *HTTPRequestNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	method := strings.ToUpper(workflow.ParamString(params, "method"))
	if method == "" {
		method = "GET"
	}
	rawURL := strings.TrimSpace(workflow.ParamString(params, "url"))
	if rawURL == "" {
		return fmt.Errorf("URL 为空")
	}
	var bodyBytes []byte
	if b := workflow.ParamString(params, "body"); b != "" {
		bodyBytes = []byte(render(b, ecx))
	}
	headers := workflow.ParamKV(params, "headers")
	cookie := workflow.ParamString(params, "cookie")
	// ssrf=true：走 httpclient 的 SSRF 防护（拒私网/元数据/链路本地），堵住 VIP 用户借
	// workflow 抓内网或云元数据并把响应回显（凭据窃取/内网探测）。
	resp, err := n.hc.Do(ctx, method, rawURL, bodyBytes, headers, cookie, true)
	if err != nil {
		return fmt.Errorf("HTTP 请求失败: %w", err)
	}
	ecx.Content = string(resp.Body)
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("HTTP %s %s → %d 字节", method, rawURL, len(resp.Body)))
	return nil
}

// ExtractFieldsNode 用正则命名组从 ecx.Content 抽字段 → ecx.Variables。
type ExtractFieldsNode struct{}

func NewExtractFieldsNode() *ExtractFieldsNode { return &ExtractFieldsNode{} }

func (n *ExtractFieldsNode) Type() string     { return "extract_fields" }
func (n *ExtractFieldsNode) Name() string     { return "解析字段" }
func (n *ExtractFieldsNode) Category() string { return "数据" }
func (n *ExtractFieldsNode) Description() string {
	return "用正则命名组从文本载荷抽取字段到变量"
}
func (n *ExtractFieldsNode) Icon() string { return "extract" }
func (n *ExtractFieldsNode) DefaultParams() map[string]any {
	return map[string]any{"pattern": ""}
}
func (n *ExtractFieldsNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "pattern", Label: "正则", Type: "textarea", Required: true,
			Help: "Go 正则，用命名组 (?P<name>...)；各组写入同名变量"},
	}
}

func (n *ExtractFieldsNode) Execute(ctx context.Context, params map[string]any, ecx *workflow.Context) error {
	pattern := strings.TrimSpace(workflow.ParamString(params, "pattern"))
	if pattern == "" {
		return fmt.Errorf("正则为空")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("正则无效: %w", err)
	}
	// 限制匹配文本长度，防用户恶意正则对超大抓取内容（http_request 节点可塞 1MB+）做 ReDoS / CPU 耗尽。
	content := ecx.Content
	const maxMatchLen = 256 * 1024
	if len(content) > maxMatchLen {
		content = content[:maxMatchLen]
	}
	m := re.FindStringSubmatch(content)
	if m == nil {
		ecx.Messages = append(ecx.Messages, "解析字段：未匹配")
		return nil
	}
	if ecx.Variables == nil {
		ecx.Variables = map[string]any{}
	}
	for i, name := range re.SubexpNames() {
		if i == 0 || name == "" {
			continue
		}
		ecx.Variables[name] = m[i]
	}
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("解析字段：命中 %d 组", len(re.SubexpNames())-1))
	return nil
}

// ParseJSONNode 把 ecx.Content 当作 JSON 解析，按点分路径抽取到变量。
// fields 是「变量名: JSON路径」映射，如 title=$.title（$ 可省略）、name=items.0.name。
// fields 为空时把整个对象写入变量 json。
type ParseJSONNode struct{}

func NewParseJSONNode() *ParseJSONNode { return &ParseJSONNode{} }

func (n *ParseJSONNode) Type() string     { return "parse_json" }
func (n *ParseJSONNode) Name() string     { return "JSON 解析" }
func (n *ParseJSONNode) Category() string { return "数据" }
func (n *ParseJSONNode) Description() string {
	return "解析文本载荷为 JSON，按点分路径抽取到变量"
}
func (n *ParseJSONNode) Icon() string { return "json" }
func (n *ParseJSONNode) DefaultParams() map[string]any {
	return map[string]any{"fields": map[string]any{}}
}
func (n *ParseJSONNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "fields", Label: "抽取字段", Type: "kv",
			Help: "键=变量名，值=JSON 路径（如 title 或 items.0.name）；留空则整体存入 json 变量"},
	}
}

func (n *ParseJSONNode) Execute(_ context.Context, params map[string]any, ecx *workflow.Context) error {
	var data any
	if err := json.Unmarshal([]byte(ecx.Content), &data); err != nil {
		return fmt.Errorf("JSON 解析失败: %w", err)
	}
	if ecx.Variables == nil {
		ecx.Variables = map[string]any{}
	}
	fields := workflow.ParamKV(params, "fields")
	if len(fields) == 0 {
		ecx.Variables["json"] = data
		ecx.Messages = append(ecx.Messages, "JSON 解析：整体存入变量 json")
		return nil
	}
	hit := 0
	for varName, path := range fields {
		path = strings.TrimSpace(strings.TrimPrefix(path, "$."))
		if path == "" || strings.HasPrefix(path, "$") {
			path = strings.TrimPrefix(strings.TrimSpace(path), "$")
		}
		v, ok := walkJSON(data, path)
		if !ok {
			continue
		}
		ecx.Variables[varName] = flattenJSONValue(v)
		hit++
	}
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("JSON 解析：抽取 %d/%d 个字段", hit, len(fields)))
	return nil
}

// walkJSON 按点分路径（支持数组下标 items.0.name）在解析后的 JSON 中取值。
func walkJSON(data any, path string) (any, bool) {
	cur := data
	for _, seg := range strings.Split(path, ".") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		switch v := cur.(type) {
		case map[string]any:
			next, ok := v[seg]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			cur = v[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

// flattenJSONValue 把标量原样返回；复合值（对象/数组）序列化为 JSON 字符串，便于模板渲染。
func flattenJSONValue(v any) any {
	switch v.(type) {
	case string, float64, int, int64, bool, nil:
		return v
	}
	if b, err := json.Marshal(v); err == nil {
		return string(b)
	}
	return fmt.Sprintf("%v", v)
}

// RegexExtractNode 用正则在文本载荷中提取「全部」匹配（区别于「解析字段」只取首条命名组），
// 把首个捕获组的值列表写入变量（换行连接），并记录数量。
type RegexExtractNode struct{}

func NewRegexExtractNode() *RegexExtractNode { return &RegexExtractNode{} }

func (n *RegexExtractNode) Type() string     { return "regex_extract" }
func (n *RegexExtractNode) Name() string     { return "正则提取" }
func (n *RegexExtractNode) Category() string { return "数据" }
func (n *RegexExtractNode) Description() string {
	return "正则提取全部匹配为列表变量（首个捕获组）"
}
func (n *RegexExtractNode) Icon() string { return "extract" }
func (n *RegexExtractNode) DefaultParams() map[string]any {
	return map[string]any{"pattern": "", "output_var": "matches"}
}
func (n *RegexExtractNode) ParamSchema() []workflow.ParamField {
	return []workflow.ParamField{
		{Name: "pattern", Label: "正则", Type: "textarea", Required: true,
			Help: "用捕获组包裹要提取的部分，如 (https?://\\S+\\.torrent)"},
		{Name: "output_var", Label: "输出变量", Type: "text", Default: "matches"},
	}
}

func (n *RegexExtractNode) Execute(_ context.Context, params map[string]any, ecx *workflow.Context) error {
	pattern := strings.TrimSpace(workflow.ParamString(params, "pattern"))
	if pattern == "" {
		return fmt.Errorf("正则为空")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("正则无效: %w", err)
	}
	out := workflow.ParamString(params, "output_var")
	if out == "" {
		out = "matches"
	}
	// 限制匹配文本长度与匹配数上限，防用户恶意正则对超大抓取内容做 ReDoS / 内存膨胀。
	content := ecx.Content
	const maxMatchLen = 256 * 1024
	if len(content) > maxMatchLen {
		content = content[:maxMatchLen]
	}
	matches := re.FindAllStringSubmatch(content, 1000)
	values := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			values = append(values, m[1]) // 首个捕获组
		} else if len(m) == 1 {
			values = append(values, m[0]) // 无捕获组 → 整个匹配
		}
	}
	if ecx.Variables == nil {
		ecx.Variables = map[string]any{}
	}
	ecx.Variables[out] = strings.Join(values, "\n")
	ecx.Variables[out+"_count"] = len(values)
	ecx.Messages = append(ecx.Messages, fmt.Sprintf("正则提取：%d 个匹配", len(values)))
	return nil
}
