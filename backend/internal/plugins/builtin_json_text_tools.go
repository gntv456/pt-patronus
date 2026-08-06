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
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

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
