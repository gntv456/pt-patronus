package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// doubanChartPlugin — 豆瓣榜单订阅（对标 MSaber 订阅管理「豆瓣榜单订阅」）。
// 定时拉取豆瓣榜单（经宿主 media.trending 工具，category=topMovie/topTv/weekly），
// 把榜单条目批量建为订阅（已存在的按 title+type 去重跳过）。
type doubanChartPlugin struct{}

func NewDoubanChartPlugin() Plugin { return doubanChartPlugin{} }

func (doubanChartPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "douban-chart-subscribe",
		Name:        "豆瓣榜单订阅",
		Version:     "1.0.0",
		Author:      "PTPatronus",
		Description: "定时同步豆瓣榜单（Top250 电影/剧集、一周口碑榜）为订阅，榜单变化自动跟进。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"media:read", "subscribe:write"},
		Actions: []ActionSpec{
			{
				Name:        "sync",
				Label:       "立即同步",
				Description: "立即拉取榜单并同步订阅（不等定时）。",
			},
		},
		Schedule: []ScheduleSpec{
			{ID: "daily", Label: "每日同步榜单", Action: "sync", Cron: "0 4 * * *"},
		},
		ConfigSchema: []ConfigField{
			{Key: "charts", Label: "榜单", Type: "select", Default: "topMovie",
				Options: []ConfigOption{
					{Label: "电影 Top250", Value: "topMovie"},
					{Label: "剧集 Top250", Value: "topTv"},
					{Label: "一周口碑榜", Value: "weekly"},
				}},
			{Key: "limit", Label: "取前 N 条", Type: "number", Default: 50, Help: "每次最多新建订阅数"},
			{Key: "resolution", Label: "偏好分辨率", Type: "text", Default: "", Help: "如 1080p，留空不限"},
		},
	}
}

func (doubanChartPlugin) DefaultConfig() map[string]any {
	return map[string]any{"charts": "topMovie", "limit": 50}
}

func (doubanChartPlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

// runCron 定时入口：拉榜单 → 建订阅。
// 经 Runtime 的 HostCaller 能力直调宿主工具（media_trending → subscribe_create），
// 含权限校验与审计；宿主/测试桩未实现 HostCaller 时报「宿主工具不可用」。
func (doubanChartPlugin) runCron(ctx context.Context, rt Runtime) error {
	cfg := rt.Config()
	chart := pluginString(cfg["charts"])
	if chart == "" {
		chart = "topMovie"
	}
	limit := pluginInt(cfg["limit"], 50)
	hc, ok := rt.(HostCaller)
	if !ok {
		return fmt.Errorf("宿主工具不可用（Runtime 未实现 HostCaller）")
	}
	raw, err := hc.CallHostTool(ctx, "recommend_feeds", map[string]any{"source": "douban", "category": chart})
	if err != nil {
		rt.Log(logError, "sync", "trending fetch failed", map[string]any{"err": err.Error()})
		return err
	}
	// adopt 的 agent tool 返回 json.RawMessage（数组）；先 JSON 解码再遍历。
	var list []any
	switch v := raw.(type) {
	case []any:
		list = v
	case json.RawMessage:
		_ = json.Unmarshal(v, &list)
	case map[string]any:
		if items, ok := v["items"].([]any); ok {
			list = items
		}
	}
	created, skipped := 0, 0
	for i, it := range list {
		if i >= limit {
			break
		}
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		title := pluginString(m["title"])
		typ := pluginString(m["type"])
		sourceID := pluginString(m["source_id"])
		if title == "" || sourceID == "" {
			continue
		}
		if typ != "movie" && typ != "tv" {
			typ = "movie"
		}
		_, err := hc.CallHostTool(ctx, "subscribe_create", map[string]any{
			"title": title, "type": typ, "source": "douban", "source_id": sourceID,
			"year": pluginInt(m["year"], 0), "resolution": pluginString(cfg["resolution"]),
		})
		if err != nil {
			// 已存在（重复订阅）计为 skipped；其它错误记日志继续。
			if strings.Contains(err.Error(), "已存在") || strings.Contains(err.Error(), "exist") {
				skipped++
			} else {
				rt.Log(logWarning, "sync", "subscribe_create failed", map[string]any{"title": title, "err": err.Error()})
			}
			continue
		}
		created++
	}
	rt.Log(logInfo, "sync", "douban chart synced", map[string]any{"chart": chart, "created": created, "skipped": skipped})
	return nil
}

func (doubanChartPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	switch action {
	case "sync":
		if err := (doubanChartPlugin{}).runCron(ctx, rt); err != nil {
			return nil, err
		}
		cfg := rt.Config()
		return map[string]any{"ok": true, "chart": pluginString(cfg["charts"])}, nil
	}
	return nil, unknownActionErr(action)
}
