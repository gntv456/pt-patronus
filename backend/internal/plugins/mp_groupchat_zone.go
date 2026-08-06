package plugins

import (
	"context"
	"fmt"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/plugins/groupchat"
)

type groupChatZonePlugin struct {
	hc *httpclient.Client
}

func NewGroupChatZonePlugin() Plugin { return &groupChatZonePlugin{} }

// client lazy 复用一个 httpclient.Client(SSRF/retry/GBK/cookie jar 全套)。
func (p *groupChatZonePlugin) client() (*httpclient.Client, error) {
	if p.hc != nil {
		return p.hc, nil
	}
	c, err := httpclient.New(httpclient.Options{FollowRedirect: true, Timeout: 20 * time.Second})
	if err != nil {
		return nil, err
	}
	p.hc = c
	return c, nil
}

func (p *groupChatZonePlugin) Manifest() Manifest {
	return Manifest{
		ID:          "group-chat-zone",
		Name:        "群聊区",
		Version:     "3.0.0",
		Author:      "KoWming,madrays / PTPatronus",
		Description: "14 站 PT 喊话自动化(移植自 MoviePilot groupchatzone)：定时喊话/奖励反馈/失败重试/青蛙每日福利/LongPT抽奖/13City勋章/织梦邮件时间+24h独立调度。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"network:http", "action:execute", "notice:write"},
		Actions: []ActionSpec{
			{Name: "run_now", Label: "立即执行", Description: "立即触发一次主喊话任务。织梦在首次执行时一并初始化独立调度。"},
			{
				Name:        "send_message",
				Label:       "发送喊话(自定义)",
				Description: "未识别站点的通用喊话兜底：NexusPHP GET / 表单 POST / JSON POST。",
				InputSchema: []ConfigField{
					{Key: "base_url", Label: "站点地址", Type: "text", Required: true},
					{Key: "cookie", Label: "Cookie", Type: "textarea", Required: true},
					{Key: "message", Label: "消息", Type: "textarea", Required: true},
					{Key: "mode", Label: "发送方式", Type: "select", Default: "nexus_get", Options: []ConfigOption{
						{Label: "NexusPHP GET", Value: "nexus_get"},
						{Label: "表单 POST", Value: "form_post"},
						{Label: "JSON POST", Value: "json_post"},
					}},
					{Key: "endpoint", Label: "发送路径", Type: "text", Default: "/shoutbox.php"},
					{Key: "message_field", Label: "消息字段", Type: "text", Default: "shbox_text"},
					{Key: "headers", Label: "附加请求头", Type: "textarea"},
					{Key: "fetch_feedback", Label: "抓取反馈", Type: "switch", Default: true},
				},
			},
			{
				Name:        "fetch_messages",
				Label:       "读取群聊(自定义)",
				Description: "读取群聊区页面片段，检查 Cookie/路径。",
				InputSchema: []ConfigField{
					{Key: "base_url", Label: "站点地址", Type: "text", Required: true},
					{Key: "cookie", Label: "Cookie", Type: "textarea", Required: true},
					{Key: "endpoint", Label: "群聊路径", Type: "text", Default: "/shoutbox.php"},
					{Key: "headers", Label: "附加请求头", Type: "textarea"},
				},
			},
		},
		ConfigSchema: []ConfigField{
			// 基础
			{Key: "notify", Label: "任务完成通知", Type: "switch", Group: "基础", Default: true},
			{Key: "use_proxy", Label: "使用代理", Type: "switch", Group: "基础", Default: false},
			// 执行
			{Key: "cron", Label: "主任务cron", Type: "text", Group: "执行", Default: "0 9 * * *", Help: "主喊话任务定时。留空=不定时。织梦站由独立任务(邮件时间+24h)自动调度。"},
			{Key: "interval_cnt", Label: "消息间隔(秒)", Type: "select", Group: "执行", Default: 2, Options: []ConfigOption{{Label: "1秒", Value: 1}, {Label: "2秒", Value: 2}}},
			{Key: "feedback_timeout", Label: "反馈等待(秒)", Type: "select", Group: "执行", Default: 5, Options: []ConfigOption{{Label: "1秒", Value: 1}, {Label: "2秒", Value: 2}, {Label: "3秒", Value: 3}, {Label: "4秒", Value: 4}, {Label: "5秒", Value: 5}}},
			{Key: "retry_count", Label: "失败重试次数", Type: "select", Group: "执行", Default: 2, Options: []ConfigOption{{Label: "0(不重试)", Value: 0}, {Label: "1", Value: 1}, {Label: "2", Value: 2}, {Label: "3", Value: 3}}},
			{Key: "retry_interval", Label: "重试间隔(分钟)", Type: "select", Group: "执行", Default: 10, Options: []ConfigOption{{Label: "5", Value: 5}, {Label: "10", Value: 10}, {Label: "15", Value: 15}, {Label: "30", Value: 30}, {Label: "60", Value: 60}, {Label: "120", Value: 120}}},
			{Key: "zm_interval", Label: "织梦消息间隔(秒)", Type: "select", Group: "执行", Default: 60, Options: []ConfigOption{{Label: "60", Value: 60}, {Label: "75", Value: 75}, {Label: "90", Value: 90}, {Label: "105", Value: 105}, {Label: "120", Value: 120}}},
			{Key: "retry_notify", Label: "重试时通知", Type: "switch", Group: "执行", Default: false},
			// 站点
			{Key: "get_feedback", Label: "抓取喊话反馈", Type: "switch", Group: "站点", Default: true, Help: "发送后抓 shoutbox 解析奖励(魔力/上传/VIP等)。"},
			{Key: "qingwa_daily_bonus", Label: "青蛙每日福利", Type: "switch", Group: "站点", Default: false},
			{Key: "longpt_daily_lottery", Label: "LongPT每日抽奖", Type: "switch", Group: "站点", Default: false},
			{Key: "thirteencity_auto_buy_blessing", Label: "13City诸神赐福", Type: "switch", Group: "站点", Default: false},
			{Key: "chat_sites", Label: "喊话站点", Type: "multiselect", Group: "站点", OptionsSource: "sites", Help: "勾选要喊话的站点（从已配置站点里选，不必填ID）。"},
			{Key: "sites_messages", Label: "站点消息", Type: "textarea", Group: "站点", Help: "每行：站名|消息1|消息2|...|Ns（Ns=该站自定义间隔秒，可选）。站名按子串匹配。"},
			// 网络(自定义兜底用)
			{Key: "timeout_seconds", Label: "超时(秒,自定义用)", Type: "number", Group: "网络", Default: 20},
			{Key: "allow_private_network", Label: "允许内网站点(自定义用)", Type: "switch", Group: "网络", Default: false},
		},
		Schedule: []ScheduleSpec{
			{ID: "main", Label: "主喊话任务", Action: "run_main", Cron: "0 9 * * *", CronConfigKey: "cron"},
			{ID: "zm", Label: "织梦独立任务", Action: "run_zm", Cron: "", CronConfigKey: "zm_cron"},
		},
	}
}

func (p *groupChatZonePlugin) DefaultConfig() map[string]any {
	return map[string]any{
		"notify": true, "use_proxy": false,
		"cron": "0 9 * * *", "interval_cnt": 2, "feedback_timeout": 5,
		"retry_count": 2, "retry_interval": 10, "zm_interval": 60, "retry_notify": false,
		"get_feedback": true, "qingwa_daily_bonus": false, "longpt_daily_lottery": false,
		"thirteencity_auto_buy_blessing": false, "chat_sites": []any{}, "sites_messages": "",
		"zm_cron": "", "timeout_seconds": 20, "allow_private_network": false,
	}
}

func (p *groupChatZonePlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (p *groupChatZonePlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	switch action {
	case "run_main", "run_now":
		hc, err := p.client()
		if err != nil {
			return nil, err
		}
		groupchat.NewEngine(gcRTAdapter{rt: rt}, hc, rt.Config()).Run(ctx)
		return map[string]any{"ok": true}, nil
	case "run_zm":
		hc, err := p.client()
		if err != nil {
			return nil, err
		}
		groupchat.NewEngine(gcRTAdapter{rt: rt}, hc, rt.Config()).RunZm(ctx)
		return map[string]any{"ok": true}, nil
	case "send_message":
		cfg := rt.Config()
		timeout := time.Duration(pluginInt(cfg["timeout_seconds"], 20)) * time.Second
		out, err := mpSendGroupMessage(ctx, input, timeout, asBool(cfg["allow_private_network"]))
		if err != nil {
			return nil, err
		}
		if asBool(cfg["notify"]) {
			_ = rt.Notice(ctx, NoticeInput{Title: "群聊区发送完成", Body: fmt.Sprintf("%s HTTP %v", pluginString(input["base_url"]), out["status_code"]), Level: "info"})
		}
		rt.Log(logInfo, action, "group chat message sent", map[string]any{"base_url": pluginString(input["base_url"]), "status_code": out["status_code"]})
		return out, nil
	case "fetch_messages":
		cfg := rt.Config()
		timeout := time.Duration(pluginInt(cfg["timeout_seconds"], 20)) * time.Second
		out, err := mpFetchGroupMessages(ctx, input, timeout, asBool(cfg["allow_private_network"]))
		if err != nil {
			return nil, err
		}
		rt.Log(logInfo, action, "group chat messages fetched", map[string]any{"base_url": pluginString(input["base_url"]), "status_code": out["status_code"]})
		return out, nil
	default:
		return nil, fmt.Errorf("unknown action %s", action)
	}
}
