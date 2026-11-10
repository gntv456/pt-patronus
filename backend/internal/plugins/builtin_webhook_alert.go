// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package plugins

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

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
		return nil, unknownActionErr(action)
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
		return nil, unknownActionErr(action)
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
