package plugins

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

type httpProbePlugin struct{}

func NewHTTPProbePlugin() Plugin { return httpProbePlugin{} }

func (httpProbePlugin) Manifest() Manifest {
	return Manifest{
		ID:          "http-probe",
		Name:        "HTTP 站点探测",
		Version:     "1.0.0",
		Author:      "PTPatronus",
		Description: "探测站点或 API 的状态码、响应时间和内容片段，适合临时检查服务是否可用。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"network:http", "action:execute"},
		Actions: []ActionSpec{
			{
				Name:        "probe",
				Label:       "探测 URL",
				Description: "发起一次 HTTP/HTTPS 请求，返回状态码、耗时、内容类型和响应片段。",
				InputSchema: []ConfigField{
					{Key: "url", Label: "URL", Type: "text", Required: true, Default: "https://example.com"},
					{Key: "method", Label: "方法", Type: "select", Default: http.MethodGet, Options: []ConfigOption{
						{Label: "GET", Value: http.MethodGet},
						{Label: "HEAD", Value: http.MethodHead},
						{Label: "POST", Value: http.MethodPost},
					}},
					{Key: "expected_status", Label: "期望状态码", Type: "number", Default: 200},
					{Key: "headers", Label: "请求头", Type: "textarea", Help: "每行一个 Header: Value，可留空。"},
					{Key: "body", Label: "请求体", Type: "textarea", Help: "POST 时使用，可留空。"},
				},
			},
		},
		ConfigSchema: []ConfigField{
			{Key: "timeout_seconds", Label: "超时时间(秒)", Type: "number", Default: 10},
			{Key: "follow_redirects", Label: "跟随重定向", Type: "switch", Default: true},
			{Key: "allow_private_network", Label: "允许内网地址", Type: "switch", Default: false, Help: "默认阻止本机、内网和链路本地地址，避免误探测敏感服务。"},
			{Key: "max_body_bytes", Label: "响应片段字节", Type: "number", Default: 2048},
			{Key: "default_headers", Label: "默认请求头", Type: "textarea", Default: "User-Agent: PTPatronus-PluginProbe/1.0"},
		},
	}
}

func (httpProbePlugin) DefaultConfig() map[string]any {
	return map[string]any{
		"timeout_seconds":       10,
		"follow_redirects":      true,
		"allow_private_network": false,
		"max_body_bytes":        2048,
		"default_headers":       "User-Agent: PTPatronus-PluginProbe/1.0",
	}
}

func (httpProbePlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (httpProbePlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	if action != "probe" {
		return nil, fmt.Errorf("unknown action %s", action)
	}
	cfg := rt.Config()
	rawURL := strings.TrimSpace(pluginString(input["url"]))
	if rawURL == "" {
		return nil, errors.New("URL 不能为空")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("解析 URL 失败: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("仅支持 HTTP/HTTPS URL")
	}
	if parsed.Hostname() == "" {
		return nil, errors.New("URL 缺少主机名")
	}
	if !asBool(cfg["allow_private_network"]) {
		// 预检提供早期清晰错误（主机名非法/字面内网 IP）；实际连接由 pluginSSRFDialContext
		// 在拨号时原子「解析→校验→直连」，消除预检与连接间的 DNS 重绑定窗口（TOCTOU）。
		if err := pluginValidatePublicHost(ctx, parsed.Hostname()); err != nil {
			return nil, err
		}
	}
	method := strings.ToUpper(strings.TrimSpace(pluginString(input["method"])))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodPost {
		return nil, errors.New("仅支持 GET、HEAD、POST")
	}
	var body io.Reader
	if method == http.MethodPost {
		body = strings.NewReader(pluginString(input["body"]))
	}
	timeout := time.Duration(pluginInt(cfg["timeout_seconds"], 10)) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	maxBody := pluginInt(cfg["max_body_bytes"], 2048)
	if maxBody <= 0 || maxBody > 65536 {
		maxBody = 2048
	}
	client := &http.Client{Timeout: timeout}
	if !asBool(cfg["follow_redirects"]) {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	// 公网模式下用拨号时校验的 transport，把 SSRF 校验绑定到实际连接（消除 DNS 重绑定）；
	// allow_private_network 模式放行内网，沿用默认 transport。
	if !asBool(cfg["allow_private_network"]) {
		client.Transport = &http.Transport{DialContext: pluginSSRFDialContext}
	}
	req, err := http.NewRequestWithContext(ctx, method, parsed.String(), body)
	if err != nil {
		return nil, err
	}
	for k, v := range pluginParseHeaders(pluginString(cfg["default_headers"])) {
		req.Header.Set(k, v)
	}
	for k, v := range pluginParseHeaders(pluginString(input["headers"])) {
		req.Header.Set(k, v)
	}
	start := time.Now()
	res, err := client.Do(req)
	latency := time.Since(start)
	if err != nil {
		return map[string]any{
			"ok":         false,
			"error":      err.Error(),
			"latency_ms": latency.Milliseconds(),
		}, nil
	}
	defer res.Body.Close()
	snippetRaw, _ := io.ReadAll(io.LimitReader(res.Body, int64(maxBody)+1))
	truncated := len(snippetRaw) > maxBody
	if truncated {
		snippetRaw = snippetRaw[:maxBody]
	}
	expected := pluginInt(input["expected_status"], 200)
	healthy := expected <= 0 || res.StatusCode == expected
	out := map[string]any{
		"ok":              true,
		"healthy":         healthy,
		"status_code":     res.StatusCode,
		"expected_status": expected,
		"latency_ms":      latency.Milliseconds(),
		"final_url":       res.Request.URL.String(),
		"content_type":    res.Header.Get("Content-Type"),
		"body_snippet":    string(snippetRaw),
		"body_truncated":  truncated,
	}
	rt.Log(logInfo, "probe", "http probe completed", map[string]any{
		"url":         rawURL,
		"status_code": res.StatusCode,
		"latency_ms":  latency.Milliseconds(),
		"healthy":     healthy,
	})
	return out, nil
}

type jsonToolboxPlugin struct{}

func NewJSONToolboxPlugin() Plugin { return jsonToolboxPlugin{} }

func (jsonToolboxPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "json-toolbox",
		Name:        "JSON 工具箱",
		Version:     "1.0.0",
		Author:      "PTPatronus",
		Description: "格式化、压缩和按路径提取 JSON，适合调试站点接口、Webhook 和插件数据。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"action:execute"},
		Actions: []ActionSpec{
			{
				Name:        "format",
				Label:       "格式化 JSON",
				Description: "校验并格式化 JSON 文本。",
				InputSchema: []ConfigField{
					{Key: "json", Label: "JSON", Type: "textarea", Required: true, Default: `{"hello":"world"}`},
					{Key: "indent", Label: "缩进", Type: "text", Default: "  "},
				},
			},
			{
				Name:        "minify",
				Label:       "压缩 JSON",
				Description: "去掉 JSON 中多余空白，便于复制到配置或请求体。",
				InputSchema: []ConfigField{
					{Key: "json", Label: "JSON", Type: "textarea", Required: true, Default: "{\n  \"hello\": \"world\"\n}"},
				},
			},
			{
				Name:        "extract",
				Label:       "提取字段",
				Description: "用点路径提取字段，支持 items.0.title 和 items[0].title。",
				InputSchema: []ConfigField{
					{Key: "json", Label: "JSON", Type: "textarea", Required: true, Default: `{"site":{"name":"PTPatronus"},"items":[{"title":"demo"}]}`},
					{Key: "path", Label: "路径", Type: "text", Required: true, Default: "site.name"},
				},
			},
		},
	}
}

func (jsonToolboxPlugin) DefaultConfig() map[string]any { return map[string]any{} }

func (jsonToolboxPlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (jsonToolboxPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	_ = ctx
	switch action {
	case "format":
		var v any
		if err := pluginDecodeJSON(pluginString(input["json"]), &v); err != nil {
			return nil, err
		}
		indent := pluginString(input["indent"])
		if indent == "" {
			indent = "  "
		}
		raw, err := json.MarshalIndent(v, "", indent)
		if err != nil {
			return nil, err
		}
		rt.Log(logInfo, action, "json formatted", nil)
		return map[string]any{"json": string(raw), "bytes": len(raw)}, nil
	case "minify":
		var buf bytes.Buffer
		if err := json.Compact(&buf, []byte(pluginString(input["json"]))); err != nil {
			return nil, err
		}
		rt.Log(logInfo, action, "json minified", nil)
		return map[string]any{"json": buf.String(), "bytes": buf.Len()}, nil
	case "extract":
		var v any
		if err := pluginDecodeJSON(pluginString(input["json"]), &v); err != nil {
			return nil, err
		}
		path := strings.TrimSpace(pluginString(input["path"]))
		value, found := pluginValueAtPath(v, path)
		out := map[string]any{"found": found, "path": path, "value": value}
		if found {
			if raw, err := json.MarshalIndent(value, "", "  "); err == nil {
				out["json"] = string(raw)
			}
		}
		rt.Log(logInfo, action, "json path extracted", map[string]any{"path": path, "found": found})
		return out, nil
	default:
		return nil, fmt.Errorf("unknown action %s", action)
	}
}

type textToolboxPlugin struct{}

func NewTextToolboxPlugin() Plugin { return textToolboxPlugin{} }

func (textToolboxPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "text-toolbox",
		Name:        "文本工具箱",
		Version:     "1.0.0",
		Author:      "PTPatronus",
		Description: "Base64、URL 编码、哈希和正则提取，处理 Cookie、Token、日志片段时很方便。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"action:execute"},
		Actions: []ActionSpec{
			{
				Name:        "base64",
				Label:       "Base64 编解码",
				Description: "对文本进行 Base64 编码或解码。",
				InputSchema: []ConfigField{
					{Key: "mode", Label: "模式", Type: "select", Default: "encode", Options: []ConfigOption{
						{Label: "编码", Value: "encode"},
						{Label: "解码", Value: "decode"},
					}},
					{Key: "text", Label: "文本", Type: "textarea", Required: true, Default: "PTPatronus"},
				},
			},
			{
				Name:        "url_codec",
				Label:       "URL 编解码",
				Description: "对文本进行 URL QueryEscape 编码或解码。",
				InputSchema: []ConfigField{
					{Key: "mode", Label: "模式", Type: "select", Default: "encode", Options: []ConfigOption{
						{Label: "编码", Value: "encode"},
						{Label: "解码", Value: "decode"},
					}},
					{Key: "text", Label: "文本", Type: "textarea", Required: true, Default: "keyword=测试"},
				},
			},
			{
				Name:        "hash",
				Label:       "计算哈希",
				Description: "计算 MD5、SHA1、SHA256 或 SHA512。",
				InputSchema: []ConfigField{
					{Key: "algorithm", Label: "算法", Type: "select", Default: "sha256", Options: []ConfigOption{
						{Label: "MD5", Value: "md5"},
						{Label: "SHA1", Value: "sha1"},
						{Label: "SHA256", Value: "sha256"},
						{Label: "SHA512", Value: "sha512"},
					}},
					{Key: "text", Label: "文本", Type: "textarea", Required: true, Default: "PTPatronus"},
				},
			},
			{
				Name:        "regex_extract",
				Label:       "正则提取",
				Description: "用 Go 正则表达式提取匹配和分组。",
				InputSchema: []ConfigField{
					{Key: "text", Label: "文本", Type: "textarea", Required: true, Default: "status=200 latency=123ms"},
					{Key: "pattern", Label: "正则", Type: "text", Required: true, Default: `status=(\d+)`},
					{Key: "all", Label: "提取全部", Type: "switch", Default: true},
				},
			},
		},
	}
}

func (textToolboxPlugin) DefaultConfig() map[string]any { return map[string]any{} }

func (textToolboxPlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (textToolboxPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	_ = ctx
	switch action {
	case "base64":
		text := pluginString(input["text"])
		switch pluginString(input["mode"]) {
		case "decode":
			raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
			if err != nil {
				return nil, err
			}
			rt.Log(logInfo, action, "base64 decoded", nil)
			return map[string]any{"text": string(raw), "bytes": len(raw)}, nil
		default:
			out := base64.StdEncoding.EncodeToString([]byte(text))
			rt.Log(logInfo, action, "base64 encoded", nil)
			return map[string]any{"text": out, "bytes": len(out)}, nil
		}
	case "url_codec":
		text := pluginString(input["text"])
		switch pluginString(input["mode"]) {
		case "decode":
			out, err := url.QueryUnescape(text)
			if err != nil {
				return nil, err
			}
			rt.Log(logInfo, action, "url decoded", nil)
			return map[string]any{"text": out}, nil
		default:
			out := url.QueryEscape(text)
			rt.Log(logInfo, action, "url encoded", nil)
			return map[string]any{"text": out}, nil
		}
	case "hash":
		text := []byte(pluginString(input["text"]))
		algorithm := strings.ToLower(strings.TrimSpace(pluginString(input["algorithm"])))
		var sum []byte
		switch algorithm {
		case "md5":
			v := md5.Sum(text)
			sum = v[:]
		case "sha1":
			v := sha1.Sum(text)
			sum = v[:]
		case "sha512":
			v := sha512.Sum512(text)
			sum = v[:]
		default:
			algorithm = "sha256"
			v := sha256.Sum256(text)
			sum = v[:]
		}
		rt.Log(logInfo, action, "hash calculated", map[string]any{"algorithm": algorithm})
		return map[string]any{"algorithm": algorithm, "hex": hex.EncodeToString(sum)}, nil
	case "regex_extract":
		re, err := regexp.Compile(pluginString(input["pattern"]))
		if err != nil {
			return nil, err
		}
		text := pluginString(input["text"])
		var matches [][]string
		if asBool(input["all"]) {
			matches = re.FindAllStringSubmatch(text, -1)
		} else if m := re.FindStringSubmatch(text); m != nil {
			matches = [][]string{m}
		}
		rt.Log(logInfo, action, "regex extracted", map[string]any{"count": len(matches)})
		return map[string]any{"count": len(matches), "matches": matches}, nil
	default:
		return nil, fmt.Errorf("unknown action %s", action)
	}
}

type webhookTransformerPlugin struct{}

func NewWebhookTransformerPlugin() Plugin { return webhookTransformerPlugin{} }

func (webhookTransformerPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "webhook-transformer",
		Name:        "Webhook 字段转换器",
		Version:     "1.0.0",
		Author:      "PTPatronus",
		Description: "把不同来源的 Webhook 字段整理成统一事件，也可以同步生成站内通知。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"event:read", "event:write", "notice:write"},
		Events: []EventSpec{
			{Type: "webhook", Description: "进站 Webhook 事件"},
		},
		Actions: []ActionSpec{
			{
				Name:        "preview_transform",
				Label:       "预览转换",
				Description: "用当前映射规则预览 Webhook 转换结果，不会发布事件。",
				InputSchema: []ConfigField{
					{Key: "payload", Label: "Webhook JSON", Type: "textarea", Required: true, Default: `{"title":"构建完成","message":"main 发布成功","level":"success"}`},
				},
			},
		},
		ConfigSchema: []ConfigField{
			{Key: "mappings", Label: "字段映射", Type: "textarea", Default: "title=title\nmessage=message\nlevel=level\nsource=source", Help: "每行 dest=source.path，支持数组路径如 items[0].title。"},
			{Key: "output_event", Label: "输出事件", Type: "text", Default: "plugin.webhook.normalized"},
			{Key: "publish_normalized", Label: "发布标准事件", Type: "switch", Default: true},
			{Key: "send_notice", Label: "同步发通知", Type: "switch", Default: false},
			{Key: "notice_title_path", Label: "通知标题字段", Type: "text", Default: "title"},
			{Key: "notice_body_path", Label: "通知正文字段", Type: "text", Default: "message"},
			{Key: "include_original", Label: "附带原始数据", Type: "switch", Default: false},
		},
	}
}

func (webhookTransformerPlugin) DefaultConfig() map[string]any {
	return map[string]any{
		"mappings":           "title=title\nmessage=message\nlevel=level\nsource=source",
		"output_event":       "plugin.webhook.normalized",
		"publish_normalized": true,
		"send_notice":        false,
		"notice_title_path":  "title",
		"notice_body_path":   "message",
		"include_original":   false,
	}
}

func (webhookTransformerPlugin) HandleEvent(ctx context.Context, rt Runtime, e Event) error {
	cfg := rt.Config()
	out := pluginApplyMappings(e.Data, pluginString(cfg["mappings"]))
	if asBool(cfg["include_original"]) {
		out["_original"] = e.Data
	}
	if asBool(cfg["publish_normalized"]) {
		eventType := strings.TrimSpace(pluginString(cfg["output_event"]))
		if eventType != "" {
			rt.Publish(ctx, eventType, out)
		}
	}
	if asBool(cfg["send_notice"]) {
		title := pluginPathString(out, pluginString(cfg["notice_title_path"]), "Webhook")
		body := pluginPathString(out, pluginString(cfg["notice_body_path"]), pluginPrettyJSON(out, 1600))
		level := pluginPathString(out, "level", "info")
		if err := rt.Notice(ctx, NoticeInput{Title: title, Body: body, Level: level}); err != nil {
			return err
		}
	}
	rt.Log(logInfo, e.Type, "webhook transformed", out)
	return nil
}

func (p webhookTransformerPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	_ = ctx
	if action != "preview_transform" {
		return nil, fmt.Errorf("unknown action %s", action)
	}
	var payload map[string]any
	if err := pluginDecodeJSON(pluginString(input["payload"]), &payload); err != nil {
		return nil, err
	}
	out := pluginApplyMappings(payload, pluginString(rt.Config()["mappings"]))
	rt.Log(logInfo, action, "webhook transform previewed", nil)
	return map[string]any{"output": out}, nil
}

type keywordAlertPlugin struct{}

func NewKeywordAlertPlugin() Plugin { return keywordAlertPlugin{} }

func (keywordAlertPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "keyword-alert",
		Name:        "关键词告警",
		Version:     "1.0.0",
		Author:      "PTPatronus",
		Description: "监听 Webhook 或插件事件，命中失败、异常等关键词时自动生成告警通知。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"event:read", "notice:write"},
		Events: []EventSpec{
			{Type: "webhook", Description: "进站 Webhook 事件"},
			{Type: "plugin.test", Description: "插件系统测试事件"},
			{Type: "plugin.webhook.normalized", Description: "标准化 Webhook 事件"},
		},
		Actions: []ActionSpec{
			{
				Name:        "test_match",
				Label:       "测试匹配",
				Description: "用当前关键词和字段配置测试一段 JSON 是否会触发告警。",
				InputSchema: []ConfigField{
					{Key: "payload", Label: "事件 JSON", Type: "textarea", Required: true, Default: `{"title":"部署失败","message":"service restart failed"}`},
				},
			},
		},
		ConfigSchema: []ConfigField{
			{Key: "event_types", Label: "监听事件", Type: "textarea", Default: "webhook\nplugin.webhook.normalized", Help: "逗号或换行分隔，留空表示监听已声明事件。"},
			{Key: "keywords", Label: "关键词", Type: "textarea", Default: "error\nfailed\nfail\n失败\n异常\n告警"},
			{Key: "fields", Label: "检查字段", Type: "textarea", Default: "title\nmessage\nbody\ncontent\ntext", Help: "按点路径读取；留空时检查整段事件 JSON。"},
			{Key: "case_sensitive", Label: "区分大小写", Type: "switch", Default: false},
			{Key: "level", Label: "通知级别", Type: "select", Default: "warning", Options: []ConfigOption{
				{Label: "信息", Value: "info"},
				{Label: "成功", Value: "success"},
				{Label: "警告", Value: "warning"},
				{Label: "危险", Value: "danger"},
			}},
			{Key: "title_prefix", Label: "标题前缀", Type: "text", Default: "[关键词告警]"},
			{Key: "include_payload", Label: "通知附带原文", Type: "switch", Default: false},
		},
	}
}

func (keywordAlertPlugin) DefaultConfig() map[string]any {
	return map[string]any{
		"event_types":     "webhook\nplugin.webhook.normalized",
		"keywords":        "error\nfailed\nfail\n失败\n异常\n告警",
		"fields":          "title\nmessage\nbody\ncontent\ntext",
		"case_sensitive":  false,
		"level":           "warning",
		"title_prefix":    "[关键词告警]",
		"include_payload": false,
	}
}

func (keywordAlertPlugin) HandleEvent(ctx context.Context, rt Runtime, e Event) error {
	cfg := rt.Config()
	if !eventAllowed(pluginString(cfg["event_types"]), e.Type) {
		return nil
	}
	result := pluginKeywordMatch(e.Data, cfg)
	if !result.Matched {
		return nil
	}
	title := strings.TrimSpace(pluginString(cfg["title_prefix"])) + " " + e.Type
	body := fmt.Sprintf("命中关键词：%s\n匹配字段：%s", strings.Join(result.Keywords, ", "), strings.Join(result.Fields, ", "))
	if asBool(cfg["include_payload"]) {
		body += "\n" + pluginPrettyJSON(e.Data, 2000)
	}
	if err := rt.Notice(ctx, NoticeInput{Title: strings.TrimSpace(title), Body: body, Level: pluginString(cfg["level"])}); err != nil {
		return err
	}
	rt.Log(logWarning, e.Type, "keyword alert matched", map[string]any{
		"keywords": result.Keywords,
		"fields":   result.Fields,
	})
	return nil
}

func (keywordAlertPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	_ = ctx
	if action != "test_match" {
		return nil, fmt.Errorf("unknown action %s", action)
	}
	var payload map[string]any
	if err := pluginDecodeJSON(pluginString(input["payload"]), &payload); err != nil {
		return nil, err
	}
	result := pluginKeywordMatch(payload, rt.Config())
	rt.Log(logInfo, action, "keyword match tested", map[string]any{"matched": result.Matched})
	return map[string]any{
		"matched":  result.Matched,
		"keywords": result.Keywords,
		"fields":   result.Fields,
	}, nil
}

type cronHelperPlugin struct{}

func NewCronHelperPlugin() Plugin { return cronHelperPlugin{} }

func (cronHelperPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "cron-helper",
		Name:        "Cron 助手",
		Version:     "1.0.0",
		Author:      "PTPatronus",
		Description: "校验 Cron 表达式并计算未来执行时间，配置定时任务前先预览结果。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"action:execute"},
		Actions: []ActionSpec{
			{
				Name:        "next_runs",
				Label:       "计算执行时间",
				Description: "按标准 5 段 Cron 表达式计算接下来几次执行时间。",
				InputSchema: []ConfigField{
					{Key: "expression", Label: "Cron 表达式", Type: "text", Required: true, Default: "0 3 * * *"},
					{Key: "count", Label: "次数", Type: "number", Default: 5},
					{Key: "timezone", Label: "时区", Type: "text", Default: "Asia/Shanghai"},
				},
			},
		},
	}
}

func (cronHelperPlugin) DefaultConfig() map[string]any { return map[string]any{} }

func (cronHelperPlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (cronHelperPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	_ = ctx
	if action != "next_runs" {
		return nil, fmt.Errorf("unknown action %s", action)
	}
	expr := strings.TrimSpace(pluginString(input["expression"]))
	if expr == "" {
		return nil, errors.New("Cron 表达式不能为空")
	}
	schedule, err := cron.ParseStandard(expr)
	if err != nil {
		return nil, err
	}
	locName := strings.TrimSpace(pluginString(input["timezone"]))
	if locName == "" {
		locName = "Asia/Shanghai"
	}
	loc, err := time.LoadLocation(locName)
	if err != nil {
		return nil, err
	}
	count := pluginInt(input["count"], 5)
	if count <= 0 {
		count = 5
	}
	if count > 20 {
		count = 20
	}
	next := make([]string, 0, count)
	t := time.Now().In(loc)
	for i := 0; i < count; i++ {
		t = schedule.Next(t).In(loc)
		next = append(next, t.Format("2006-01-02 15:04:05 MST"))
	}
	rt.Log(logInfo, action, "cron next runs calculated", map[string]any{"expression": expr, "count": count})
	return map[string]any{"expression": expr, "timezone": locName, "next_runs": next}, nil
}

type pluginKeywordResult struct {
	Matched  bool
	Keywords []string
	Fields   []string
}

func pluginKeywordMatch(data map[string]any, cfg map[string]any) pluginKeywordResult {
	keywords := pluginSplitList(pluginString(cfg["keywords"]))
	fields := pluginSplitList(pluginString(cfg["fields"]))
	caseSensitive := asBool(cfg["case_sensitive"])
	hits := map[string]bool{}
	fieldHits := map[string]bool{}
	search := func(field, text string) {
		needleText := text
		if !caseSensitive {
			needleText = strings.ToLower(needleText)
		}
		for _, kw := range keywords {
			cmp := kw
			if !caseSensitive {
				cmp = strings.ToLower(cmp)
			}
			if cmp != "" && strings.Contains(needleText, cmp) {
				hits[kw] = true
				fieldHits[field] = true
			}
		}
	}
	if len(fields) == 0 {
		search("$payload", pluginPrettyJSON(data, 100000))
	} else {
		for _, field := range fields {
			if v, ok := pluginValueAtPath(data, field); ok {
				search(field, pluginAnyString(v))
			}
		}
	}
	return pluginKeywordResult{
		Matched:  len(hits) > 0,
		Keywords: pluginSortedKeys(hits),
		Fields:   pluginSortedKeys(fieldHits),
	}
}

func pluginApplyMappings(data map[string]any, raw string) map[string]any {
	out := map[string]any{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		dest := strings.TrimSpace(k)
		src := strings.TrimSpace(v)
		if dest == "" || src == "" {
			continue
		}
		if val, found := pluginValueAtPath(data, src); found {
			out[dest] = val
		}
	}
	return out
}

func pluginValueAtPath(root any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" || path == "$" {
		return root, true
	}
	cur := root
	for _, part := range pluginPathParts(path) {
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[part]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

func pluginPathString(root any, path, fallback string) string {
	if v, ok := pluginValueAtPath(root, path); ok {
		s := strings.TrimSpace(pluginAnyString(v))
		if s != "" {
			return s
		}
	}
	return fallback
}

func pluginPathParts(path string) []string {
	var parts []string
	var b strings.Builder
	inBracket := false
	for _, r := range path {
		switch r {
		case '.':
			if inBracket {
				b.WriteRune(r)
				continue
			}
			if b.Len() > 0 {
				parts = append(parts, b.String())
				b.Reset()
			}
		case '[':
			if b.Len() > 0 {
				parts = append(parts, b.String())
				b.Reset()
			}
			inBracket = true
		case ']':
			if b.Len() > 0 {
				parts = append(parts, b.String())
				b.Reset()
			}
			inBracket = false
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		parts = append(parts, b.String())
	}
	return parts
}

func pluginValidatePublicHost(ctx context.Context, host string) error {
	host = strings.TrimSpace(strings.TrimSuffix(host, "."))
	if host == "" {
		return errors.New("主机名不能为空")
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") || strings.HasSuffix(lower, ".local") {
		return fmt.Errorf("已阻止内网/本机地址 %s；如需探测请在插件配置中开启允许内网地址", host)
	}
	if ip := net.ParseIP(host); ip != nil {
		if pluginIsPrivateIP(ip) {
			return fmt.Errorf("已阻止内网/本机地址 %s；如需探测请在插件配置中开启允许内网地址", host)
		}
		return nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil {
		return fmt.Errorf("解析主机失败: %w", err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("主机 %s 没有可用 IP", host)
	}
	for _, addr := range addrs {
		if pluginIsPrivateIP(addr.IP) {
			return fmt.Errorf("已阻止解析到内网/本机地址 %s；如需探测请在插件配置中开启允许内网地址", addr.IP.String())
		}
	}
	return nil
}

func pluginIsPrivateIP(ip net.IP) bool {
	return ip.IsPrivate() ||
		ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}

// pluginZeroDialer 复用同一零配置拨号器（无 KeepAlive 干扰，按需建连）。
var pluginZeroDialer = &net.Dialer{}

// pluginSSRFDialContext 在拨号时原子地「解析主机→逐 IP 校验公网→直连已校验 IP」，
// 消除 pluginValidatePublicHost 预检与实际连接之间的 DNS 重绑定窗口（TOCTOU）：
// 预检与请求各做一次 DNS 解析，攻击者可在两次解析间令记录从公网 IP 翻转至内网 IP。
// 此处把校验下沉到拨号点，解析得到的 IP 直接用于建连，不再二次解析。
// 每次拨号（含重定向新主机）都独立校验，首跳与后续跳一视同仁。
// 参考 httpclient.ssrfDialContext。
func pluginSSRFDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	// 字面 IP：直接校验，无需解析（无重绑定窗口）。
	if ip := net.ParseIP(host); ip != nil {
		if pluginIsPrivateIP(ip) {
			return nil, fmt.Errorf("已阻止内网/本机地址 %s", host)
		}
		return pluginZeroDialer.DialContext(ctx, network, addr)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, a := range addrs {
		if pluginIsPrivateIP(a.IP) {
			lastErr = fmt.Errorf("已阻止解析到内网/本机地址 %s", a.IP.String())
			continue
		}
		conn, derr := pluginZeroDialer.DialContext(ctx, network, net.JoinHostPort(a.IP.String(), port))
		if derr == nil {
			return conn, nil
		}
		lastErr = derr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("主机 %s 没有可用的公网 IP", host)
	}
	return nil, lastErr
}

func pluginDecodeJSON(raw string, out any) error {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("JSON 解析失败: %w", err)
	}
	return nil
}

func pluginParseHeaders(raw string) map[string]string {
	headers := map[string]string{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		headers[key] = strings.TrimSpace(v)
	}
	return headers
}

func pluginInt(v any, fallback int) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case json.Number:
		if n, err := x.Int64(); err == nil {
			return int(n)
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(x)); err == nil {
			return n
		}
	}
	return fallback
}

func pluginString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}

func pluginAnyString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		raw, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprint(x)
		}
		return string(raw)
	}
}

func pluginSplitList(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ';'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func pluginSortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func pluginPrettyJSON(v any, limit int) string {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprint(v)
	}
	s := string(raw)
	if limit > 0 && len(s) > limit {
		return s[:limit] + "..."
	}
	return s
}
