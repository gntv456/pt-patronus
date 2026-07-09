package plugins

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/plugins/groupchat"
)

type cloudflareSubscribePlugin struct{}

func NewCloudflareSubscribePlugin() Plugin { return cloudflareSubscribePlugin{} }

func (cloudflareSubscribePlugin) Manifest() Manifest {
	return Manifest{
		ID:          "cloudflare-subscribe",
		Name:        "Cloudflare订阅",
		Version:     "1.0.5",
		Author:      "KoWming / PTPatronus",
		Description: "迁移自 MoviePilot：拉取 Cloudflare hosts 订阅，清洗校验后生成可合并的 hosts 内容。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"network:http", "action:execute", "notice:write"},
		Actions: []ActionSpec{
			{
				Name:        "update_hosts",
				Label:       "拉取订阅",
				Description: "按“名称|URL”配置拉取 hosts 订阅，过滤 HTML 和无效记录，输出合并后的 hosts。",
				InputSchema: []ConfigField{
					{Key: "subscriptions", Label: "订阅地址", Type: "textarea", Help: "每行：订阅名称|https://example.com/hosts；留空使用插件配置。"},
					{Key: "existing_hosts", Label: "已有 hosts", Type: "textarea", Help: "可粘贴当前 hosts，插件会移除旧订阅块并追加新订阅块。"},
				},
			},
			{
				Name:        "validate_hosts",
				Label:       "校验 hosts",
				Description: "校验一段 hosts 文本，返回有效记录和无效行。",
				InputSchema: []ConfigField{
					{Key: "hosts", Label: "hosts 内容", Type: "textarea", Required: true},
				},
			},
		},
		ConfigSchema: []ConfigField{
			{Key: "subscriptions", Label: "订阅地址", Type: "textarea", Group: "订阅", Help: "每行：订阅名称|订阅地址。"},
			{Key: "timeout_seconds", Label: "超时时间(秒)", Type: "number", Group: "网络", Default: 30},
			{Key: "max_retries", Label: "失败重试次数", Type: "number", Group: "网络", Default: 2},
			{Key: "allow_private_network", Label: "允许内网订阅源", Type: "switch", Group: "网络", Default: false},
			{Key: "notify", Label: "完成后通知", Type: "switch", Group: "通知", Default: false},
			{Key: "cron_update", Label: "定时拉取cron", Type: "text", Group: "定时", Default: "0 6 * * *", Help: "到点自动拉取订阅更新 hosts；留空=不定时。默认每天 06:00。"},
		},
		Schedule: []ScheduleSpec{
			{ID: "daily_update", Label: "每日拉取订阅", Action: "update_hosts", Cron: "0 6 * * *", CronConfigKey: "cron_update"},
		},
	}
}

func (cloudflareSubscribePlugin) DefaultConfig() map[string]any {
	return map[string]any{
		"subscriptions":         "",
		"timeout_seconds":       30,
		"max_retries":           2,
		"notify":                false,
		"allow_private_network": false,
		"cron_update":           "0 6 * * *",
	}
}

func (cloudflareSubscribePlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (cloudflareSubscribePlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	switch action {
	case "update_hosts":
		cfg := rt.Config()
		rawSubs := strings.TrimSpace(pluginString(input["subscriptions"]))
		if rawSubs == "" {
			rawSubs = pluginString(cfg["subscriptions"])
		}
		subs, err := mpParseSubscriptions(rawSubs)
		if err != nil {
			return nil, err
		}
		timeout := time.Duration(pluginInt(cfg["timeout_seconds"], 30)) * time.Second
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		retries := pluginInt(cfg["max_retries"], 2)
		if retries < 0 {
			retries = 0
		}
		allowPrivate := asBool(cfg["allow_private_network"])
		existing := pluginString(input["existing_hosts"])
		finalHosts, results := mpFetchHostsSubscriptions(ctx, subs, timeout, retries, allowPrivate)
		merged := mpMergeSubscriptionHosts(existing, finalHosts)
		total := 0
		failures := 0
		for _, r := range results {
			total += pluginInt(r["records"], 0)
			if r["ok"] != true {
				failures++
			}
		}
		if asBool(cfg["notify"]) {
			level := "success"
			if failures > 0 {
				level = "warning"
			}
			_ = rt.Notice(ctx, NoticeInput{
				Title: "Cloudflare订阅更新完成",
				Body:  fmt.Sprintf("有效记录：%d，失败订阅：%d", total, failures),
				Level: level,
			})
		}
		rt.Log(logInfo, action, "cloudflare hosts subscription updated", map[string]any{"records": total, "failures": failures})
		return map[string]any{
			"hosts":         merged,
			"records":       total,
			"failures":      failures,
			"subscriptions": results,
		}, nil
	case "validate_hosts":
		records, invalid := mpParseHosts(pluginString(input["hosts"]))
		return map[string]any{"records": len(records), "valid": records, "invalid": invalid}, nil
	default:
		return nil, fmt.Errorf("unknown action %s", action)
	}
}

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
			{Name: "run_now", Label: "立即执行", Description: "立即触发一次主喊话任务(对标 onlyonce)。织梦在首次执行时一并初始化独立调度。"},
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

type twoFAHelperPlugin struct{}

func NewTwoFAHelperPlugin() Plugin { return twoFAHelperPlugin{} }

func (twoFAHelperPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "twofa-helper",
		Name:        "两步验证助手",
		Version:     "1.2.7",
		Author:      "madrays / PTPatronus",
		Description: "迁移自 MoviePilot：管理 TOTP 密钥并生成当前 6 位验证码。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"action:execute"},
		Actions: []ActionSpec{
			{
				Name:        "generate_codes",
				Label:       "生成验证码",
				Description: "按配置或输入的账号列表生成当前 TOTP 验证码。",
				InputSchema: []ConfigField{
					{Key: "accounts", Label: "账号列表", Type: "textarea", Help: "每行：名称|Base32密钥|URL，可留空使用插件配置。"},
				},
			},
			{
				Name:        "generate_one",
				Label:       "单个验证码",
				Description: "输入一个 Base32 密钥生成当前 TOTP 验证码。",
				InputSchema: []ConfigField{
					{Key: "name", Label: "名称", Type: "text", Default: "Demo"},
					{Key: "secret", Label: "Base32密钥", Type: "password", Required: true},
				},
			},
			{
				Name:        "otpauth_uri",
				Label:       "生成 otpauth URI",
				Description: "生成可导入认证器的 otpauth://totp URI。",
				InputSchema: []ConfigField{
					{Key: "issuer", Label: "签发方", Type: "text", Default: "PTPatronus"},
					{Key: "name", Label: "账号", Type: "text", Required: true},
					{Key: "secret", Label: "Base32密钥", Type: "password", Required: true},
				},
			},
		},
		ConfigSchema: []ConfigField{
			{Key: "accounts", Label: "账号列表", Type: "textarea", Group: "账号", Help: "每行：名称|Base32密钥|URL。配置会随插件配置保存，请只在可信环境使用。"},
			{Key: "digits", Label: "验证码位数", Type: "number", Group: "参数", Default: 6},
			{Key: "period_seconds", Label: "刷新周期(秒)", Type: "number", Group: "参数", Default: 30},
		},
	}
}

func (twoFAHelperPlugin) DefaultConfig() map[string]any {
	return map[string]any{"accounts": "", "digits": 6, "period_seconds": 30}
}

func (twoFAHelperPlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (twoFAHelperPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	_ = ctx
	cfg := rt.Config()
	digits := pluginInt(cfg["digits"], 6)
	period := pluginInt(cfg["period_seconds"], 30)
	switch action {
	case "generate_codes":
		raw := strings.TrimSpace(pluginString(input["accounts"]))
		if raw == "" {
			raw = pluginString(cfg["accounts"])
		}
		accounts, err := mpParseTOTPAccounts(raw)
		if err != nil {
			return nil, err
		}
		codes := make([]map[string]any, 0, len(accounts))
		for _, account := range accounts {
			code, remain, err := mpTOTPCode(account.Secret, time.Now(), digits, period)
			if err != nil {
				codes = append(codes, map[string]any{"name": account.Name, "ok": false, "error": err.Error()})
				continue
			}
			codes = append(codes, map[string]any{
				"name":              account.Name,
				"code":              code,
				"url":               account.URL,
				"remaining_seconds": remain,
				"period_seconds":    period,
				"ok":                true,
			})
		}
		rt.Log(logInfo, action, "totp codes generated", map[string]any{"count": len(codes)})
		return map[string]any{"codes": codes}, nil
	case "generate_one":
		code, remain, err := mpTOTPCode(pluginString(input["secret"]), time.Now(), digits, period)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(pluginString(input["name"]))
		if name == "" {
			name = "TOTP"
		}
		rt.Log(logInfo, action, "single totp code generated", map[string]any{"name": name})
		return map[string]any{"name": name, "code": code, "remaining_seconds": remain, "period_seconds": period}, nil
	case "otpauth_uri":
		issuer := strings.TrimSpace(pluginString(input["issuer"]))
		name := strings.TrimSpace(pluginString(input["name"]))
		secret := mpCleanTOTPSecret(pluginString(input["secret"]))
		if name == "" || secret == "" {
			return nil, errors.New("账号和密钥不能为空")
		}
		u := url.URL{Scheme: "otpauth", Host: "totp", Path: "/" + issuer + ":" + name}
		q := u.Query()
		q.Set("secret", secret)
		q.Set("issuer", issuer)
		q.Set("digits", fmt.Sprint(digits))
		q.Set("period", fmt.Sprint(period))
		u.RawQuery = q.Encode()
		return map[string]any{"uri": u.String()}, nil
	default:
		return nil, fmt.Errorf("unknown action %s", action)
	}
}

type xiaomiRouterPlugin struct{}

func NewXiaomiRouterPlugin() Plugin { return xiaomiRouterPlugin{} }

func (xiaomiRouterPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "xiaomi-router",
		Name:        "小米路由器监控",
		Version:     "1.0.0",
		Author:      "madrays / PTPatronus",
		Description: "迁移自 MoviePilot：登录小米路由器，查看运行状态并管理端口映射。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"network:http", "action:execute", "notice:write"},
		Actions: []ActionSpec{
			{Name: "status", Label: "刷新状态", Description: "登录路由器并读取在线设备、上下行、CPU、内存和运行时间。"},
			{Name: "port_list", Label: "端口映射列表", Description: "读取当前端口映射列表。"},
			{
				Name:        "port_add",
				Label:       "添加端口映射",
				Description: "添加一条端口映射规则。",
				InputSchema: []ConfigField{
					{Key: "name", Label: "名称", Type: "text", Required: true},
					{Key: "proto", Label: "协议", Type: "select", Default: "both", Options: []ConfigOption{
						{Label: "tcp", Value: "tcp"},
						{Label: "udp", Value: "udp"},
						{Label: "both", Value: "both"},
					}},
					{Key: "sport", Label: "外部端口", Type: "number", Required: true},
					{Key: "ip", Label: "内网 IP", Type: "text", Required: true},
					{Key: "dport", Label: "内部端口", Type: "number", Required: true},
				},
			},
			{
				Name:        "port_delete",
				Label:       "删除端口映射",
				Description: "按外部端口删除映射，协议可自动推断。",
				InputSchema: []ConfigField{
					{Key: "port", Label: "外部端口", Type: "number", Required: true},
					{Key: "proto", Label: "协议", Type: "select", Default: "auto", Options: []ConfigOption{
						{Label: "自动", Value: "auto"},
						{Label: "tcp", Value: "tcp"},
						{Label: "udp", Value: "udp"},
						{Label: "both", Value: "both"},
					}},
				},
			},
			{Name: "quick_toggle", Label: "快捷切换", Description: "根据配置中的快捷预设，存在则删除，不存在则添加。"},
		},
		ConfigSchema: []ConfigField{
			{Key: "router_url", Label: "路由器地址", Type: "text", Group: "连接", Default: "http://192.168.31.1", Required: true},
			{Key: "password", Label: "登录密码", Type: "password", Group: "连接", Required: true},
			{Key: "timeout_seconds", Label: "超时时间(秒)", Type: "number", Group: "连接", Default: 10},
			{Key: "allow_public_router", Label: "允许公网地址", Type: "switch", Group: "连接", Default: false, Help: "默认只允许本机、内网或链路本地地址，避免误把密码发到公网。"},
			{Key: "notify", Label: "动作完成后通知", Type: "switch", Group: "通知", Default: false},
			{Key: "q_name", Label: "快捷名称", Type: "text", Group: "快捷预设"},
			{Key: "q_proto", Label: "快捷协议", Type: "select", Group: "快捷预设", Default: "tcp", Options: []ConfigOption{
				{Label: "tcp", Value: "tcp"},
				{Label: "udp", Value: "udp"},
				{Label: "both", Value: "both"},
			}},
			{Key: "q_sport", Label: "快捷外部端口", Type: "number", Group: "快捷预设"},
			{Key: "q_ip", Label: "快捷内网 IP", Type: "text", Group: "快捷预设"},
			{Key: "q_dport", Label: "快捷内部端口", Type: "number", Group: "快捷预设"},
			{Key: "cron_status", Label: "定时刷新cron", Type: "text", Group: "定时", Default: "0 */6 * * *", Help: "到点自动登录并读取路由器状态；留空=不定时。默认每 6 小时。"},
		},
		Schedule: []ScheduleSpec{
			{ID: "status_refresh", Label: "定时刷新状态", Action: "status", Cron: "0 */6 * * *", CronConfigKey: "cron_status"},
		},
	}
}

func (xiaomiRouterPlugin) DefaultConfig() map[string]any {
	return map[string]any{
		"router_url":          "http://192.168.31.1",
		"password":            "",
		"timeout_seconds":     10,
		"allow_public_router": false,
		"notify":              false,
		"q_name":              "",
		"q_proto":             "tcp",
		"q_sport":             0,
		"q_ip":                "",
		"q_dport":             0,
		"cron_status":         "0 */6 * * *",
	}
}

func (xiaomiRouterPlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (xiaomiRouterPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	cfg := rt.Config()
	client, err := mpNewXiaomiClient(ctx, cfg)
	if err != nil {
		return nil, err
	}
	switch action {
	case "status":
		status, ports, err := client.Status(ctx)
		if err != nil {
			return nil, err
		}
		if asBool(cfg["notify"]) {
			_ = rt.Notice(ctx, NoticeInput{Title: "小米路由器状态", Body: mpXiaomiStatusText(status), Level: "info"})
		}
		rt.Log(logInfo, action, "xiaomi router status refreshed", map[string]any{"online_count": status["online_count"]})
		return map[string]any{"status": status, "ports": ports}, nil
	case "port_list":
		ports, err := client.PortList(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ports": ports, "count": len(ports)}, nil
	case "port_add":
		res, err := client.PortAdd(ctx, pluginString(input["name"]), mpXiaomiProtoInt(input["proto"]), pluginInt(input["sport"], 0), pluginString(input["ip"]), pluginInt(input["dport"], 0))
		if err != nil {
			return nil, err
		}
		return map[string]any{"result": res}, nil
	case "port_delete":
		proto := mpXiaomiProtoInt(input["proto"])
		if strings.EqualFold(pluginString(input["proto"]), "auto") {
			ports, _ := client.PortList(ctx)
			for _, p := range ports {
				if pluginInt(p["srcport"], -1) == pluginInt(input["port"], 0) {
					proto = pluginInt(p["proto"], 3)
					break
				}
			}
		}
		res, err := client.PortDelete(ctx, pluginInt(input["port"], 0), proto)
		if err != nil {
			return nil, err
		}
		return map[string]any{"result": res, "proto": mpXiaomiProtoText(proto)}, nil
	case "quick_toggle":
		res, err := client.QuickToggle(ctx, cfg)
		if err != nil {
			return nil, err
		}
		return res, nil
	default:
		return nil, fmt.Errorf("unknown action %s", action)
	}
}

type autoSpeedPlugin struct{}

func NewAutoSpeedPlugin() Plugin { return autoSpeedPlugin{} }

func (autoSpeedPlugin) Manifest() Manifest {
	return Manifest{
		ID:          "auto-speed",
		Name:        "Vue-网络测速",
		Version:     "1.1.0",
		Author:      "KoWming,鱼丸粗面 / PTPatronus",
		Description: "迁移自 MoviePilot：执行轻量下载/上传测速，记录结果到插件日志。",
		Runtime:     RuntimeBuiltin,
		Builtin:     true,
		Permissions: []string{"network:http", "action:execute", "notice:write"},
		Actions: []ActionSpec{
			{
				Name:        "run_speedtest",
				Label:       "开始测速",
				Description: "使用下载 URL 和可选上传 URL 进行轻量测速；默认读取有限字节，适合快速判断网络质量。",
				InputSchema: []ConfigField{
					{Key: "download_url", Label: "下载测试 URL", Type: "text", Help: "留空使用插件配置。"},
					{Key: "upload_url", Label: "上传测试 URL", Type: "text", Help: "留空使用插件配置；为空则跳过上传。"},
				},
			},
			{Name: "presets", Label: "测速预设", Description: "查看可直接复制使用的测速端点。"},
		},
		ConfigSchema: []ConfigField{
			{Key: "download_url", Label: "下载测试 URL", Type: "text", Group: "测速", Default: "https://speed.cloudflare.com/__down?bytes=25000000"},
			{Key: "upload_url", Label: "上传测试 URL", Type: "text", Group: "测速", Default: "https://speed.cloudflare.com/__up"},
			{Key: "download_bytes", Label: "下载字节上限", Type: "number", Group: "测速", Default: 10000000},
			{Key: "upload_bytes", Label: "上传字节数", Type: "number", Group: "测速", Default: 2000000},
			{Key: "timeout_seconds", Label: "超时时间(秒)", Type: "number", Group: "网络", Default: 20},
			{Key: "allow_private_network", Label: "允许内网地址", Type: "switch", Group: "网络", Default: false},
			{Key: "notify", Label: "测速完成后通知", Type: "switch", Group: "通知", Default: false},
			{Key: "cron_speedtest", Label: "定时测速cron", Type: "text", Group: "定时", Default: "0 */6 * * *", Help: "到点自动跑一次测速并记入日志；留空=不定时。默认每 6 小时。"},
		},
		Schedule: []ScheduleSpec{
			{ID: "speedtest", Label: "定时测速", Action: "run_speedtest", Cron: "0 */6 * * *", CronConfigKey: "cron_speedtest"},
		},
	}
}

func (autoSpeedPlugin) DefaultConfig() map[string]any {
	return map[string]any{
		"download_url":          "https://speed.cloudflare.com/__down?bytes=25000000",
		"upload_url":            "https://speed.cloudflare.com/__up",
		"download_bytes":        10000000,
		"upload_bytes":          2000000,
		"timeout_seconds":       20,
		"allow_private_network": false,
		"notify":                false,
		"cron_speedtest":        "0 */6 * * *",
	}
}

func (autoSpeedPlugin) HandleEvent(context.Context, Runtime, Event) error { return nil }

func (autoSpeedPlugin) ExecuteAction(ctx context.Context, rt Runtime, action string, input map[string]any) (map[string]any, error) {
	cfg := rt.Config()
	switch action {
	case "run_speedtest":
		out, err := mpRunSpeedtest(ctx, cfg, input)
		if err != nil {
			return nil, err
		}
		if asBool(cfg["notify"]) {
			_ = rt.Notice(ctx, NoticeInput{Title: "网络测速完成", Body: mpSpeedtestText(out), Level: "success"})
		}
		rt.Log(logInfo, action, "network speedtest completed", out)
		return out, nil
	case "presets":
		return map[string]any{
			"download": []string{
				"https://speed.cloudflare.com/__down?bytes=25000000",
				"https://speed.cloudflare.com/__down?bytes=100000000",
			},
			"upload": []string{"https://speed.cloudflare.com/__up"},
		}, nil
	default:
		return nil, fmt.Errorf("unknown action %s", action)
	}
}

type mpSubscription struct {
	Name string
	URL  string
}

func mpParseSubscriptions(raw string) ([]mpSubscription, error) {
	var out []mpSubscription
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, rawURL, ok := strings.Cut(line, "|")
		if !ok {
			return nil, fmt.Errorf("订阅配置格式错误：%s", line)
		}
		name = strings.TrimSpace(name)
		rawURL = strings.TrimSpace(rawURL)
		u, err := url.Parse(rawURL)
		if name == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, fmt.Errorf("订阅配置无效：%s", line)
		}
		out = append(out, mpSubscription{Name: name, URL: rawURL})
	}
	if len(out) == 0 {
		return nil, errors.New("没有有效订阅配置")
	}
	return out, nil
}

func mpFetchHostsSubscriptions(ctx context.Context, subs []mpSubscription, timeout time.Duration, retries int, allowPrivate bool) (string, []map[string]any) {
	var blocks []string
	results := make([]map[string]any, 0, len(subs))
	for _, sub := range subs {
		result := map[string]any{"name": sub.Name, "url": sub.URL, "ok": false, "records": 0}
		if err := mpValidateHTTPURL(ctx, sub.URL, allowPrivate); err != nil {
			result["error"] = err.Error()
			results = append(results, result)
			continue
		}
		var body []byte
		var err error
		for attempt := 0; attempt <= retries; attempt++ {
			_, _, body, err = mpHTTPRequest(ctx, http.MethodGet, sub.URL, nil, map[string]string{
				"Accept":        "text/plain,text/html,*/*",
				"Cache-Control": "no-cache",
			}, "", timeout)
			if err == nil {
				break
			}
			if attempt < retries {
				time.Sleep(time.Duration(attempt+1) * time.Second)
			}
		}
		if err != nil {
			result["error"] = err.Error()
			results = append(results, result)
			continue
		}
		cleaned := mpCleanHostsContent(string(body))
		records, invalid := mpParseHosts(cleaned)
		if len(records) == 0 {
			result["error"] = "未找到有效 hosts 记录"
			result["invalid"] = invalid
			results = append(results, result)
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "# ===== %s 订阅开始 ===== #\n", sub.Name)
		for _, rec := range records {
			fmt.Fprintf(&b, "%s %s\n", rec["ip"], rec["host"])
		}
		fmt.Fprintf(&b, "# ===== %s 订阅结束 (%d 条记录) ===== #\n", sub.Name, len(records))
		blocks = append(blocks, b.String())
		result["ok"] = true
		result["records"] = len(records)
		result["invalid_count"] = len(invalid)
		results = append(results, result)
	}
	return strings.Join(blocks, "\n"), results
}

func mpCleanHostsContent(content string) string {
	pre := regexp.MustCompile(`(?is)<pre[^>]*>(.*?)</pre>`).FindStringSubmatch(content)
	if len(pre) > 1 {
		content = pre[1]
	}
	content = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(content, "")
	content = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(content, "")
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func mpParseHosts(content string) ([]map[string]string, []string) {
	var records []map[string]string
	var invalid []string
	for _, line := range strings.Split(content, "\n") {
		original := strings.TrimSpace(line)
		if original == "" || strings.HasPrefix(original, "#") {
			continue
		}
		line = strings.SplitN(original, "#", 2)[0]
		parts := strings.Fields(line)
		if len(parts) < 2 {
			invalid = append(invalid, original)
			continue
		}
		ip := net.ParseIP(parts[0])
		if ip == nil {
			invalid = append(invalid, original)
			continue
		}
		host := strings.TrimSpace(parts[1])
		if host == "" || strings.ContainsAny(host, `/\`) {
			invalid = append(invalid, original)
			continue
		}
		records = append(records, map[string]string{"ip": ip.String(), "host": host})
	}
	return records, invalid
}

func mpMergeSubscriptionHosts(existing, subscriptions string) string {
	if strings.TrimSpace(existing) == "" {
		return strings.TrimSpace(subscriptions)
	}
	var kept []string
	inBlock := false
	for _, line := range strings.Split(existing, "\n") {
		if strings.Contains(line, "# =====") && strings.Contains(line, "订阅开始") {
			inBlock = true
			continue
		}
		if strings.Contains(line, "# =====") && strings.Contains(line, "订阅结束") && inBlock {
			inBlock = false
			continue
		}
		if !inBlock {
			kept = append(kept, strings.TrimRight(line, "\r"))
		}
	}
	base := strings.TrimRight(strings.Join(kept, "\n"), "\n")
	if strings.TrimSpace(subscriptions) == "" {
		return base
	}
	if base != "" {
		return base + "\n\n" + strings.TrimSpace(subscriptions)
	}
	return strings.TrimSpace(subscriptions)
}

func mpSendGroupMessage(ctx context.Context, input map[string]any, timeout time.Duration, allowPrivate bool) (map[string]any, error) {
	baseURL := pluginString(input["base_url"])
	cookie := pluginString(input["cookie"])
	message := pluginString(input["message"])
	endpoint := pluginString(input["endpoint"])
	if endpoint == "" {
		endpoint = "/shoutbox.php"
	}
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(cookie) == "" || strings.TrimSpace(message) == "" {
		return nil, errors.New("站点地址、Cookie、消息不能为空")
	}
	target, err := mpJoinURL(baseURL, endpoint)
	if err != nil {
		return nil, err
	}
	if err := mpValidateHTTPURL(ctx, target, allowPrivate); err != nil {
		return nil, err
	}
	headers := pluginParseHeaders(pluginString(input["headers"]))
	headers["Referer"] = strings.TrimRight(baseURL, "/") + "/"
	mode := pluginString(input["mode"])
	field := pluginString(input["message_field"])
	if field == "" {
		field = "shbox_text"
	}
	var status int
	var h http.Header
	var body []byte
	switch mode {
	case "json_post":
		payload, _ := json.Marshal(map[string]string{field: message})
		headers["Content-Type"] = "application/json"
		status, h, body, err = mpHTTPRequest(ctx, http.MethodPost, target, bytes.NewReader(payload), headers, cookie, timeout)
	case "form_post":
		form := url.Values{}
		form.Set(field, message)
		headers["Content-Type"] = "application/x-www-form-urlencoded"
		status, h, body, err = mpHTTPRequest(ctx, http.MethodPost, target, strings.NewReader(form.Encode()), headers, cookie, timeout)
	default:
		u, _ := url.Parse(target)
		q := u.Query()
		q.Set(field, message)
		q.Set("sent", "yes")
		q.Set("shout", "我喊")
		q.Set("type", "shoutbox")
		u.RawQuery = q.Encode()
		status, h, body, err = mpHTTPRequest(ctx, http.MethodGet, u.String(), nil, headers, cookie, timeout)
	}
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"status_code":  status,
		"content_type": h.Get("Content-Type"),
		"body_snippet": truncate(string(body), 1600),
	}
	if asBool(input["fetch_feedback"]) {
		feedback, _ := mpFetchGroupMessages(ctx, map[string]any{
			"base_url": baseURL,
			"cookie":   cookie,
			"endpoint": endpoint,
			"headers":  pluginString(input["headers"]),
		}, timeout, allowPrivate)
		out["feedback"] = feedback["lines"]
	}
	return out, nil
}

func mpFetchGroupMessages(ctx context.Context, input map[string]any, timeout time.Duration, allowPrivate bool) (map[string]any, error) {
	target, err := mpJoinURL(pluginString(input["base_url"]), pluginString(input["endpoint"]))
	if err != nil {
		return nil, err
	}
	if err := mpValidateHTTPURL(ctx, target, allowPrivate); err != nil {
		return nil, err
	}
	headers := pluginParseHeaders(pluginString(input["headers"]))
	status, h, body, err := mpHTTPRequest(ctx, http.MethodGet, target, nil, headers, pluginString(input["cookie"]), timeout)
	if err != nil {
		return nil, err
	}
	text := mpHTMLToText(string(body))
	return map[string]any{
		"status_code":  status,
		"content_type": h.Get("Content-Type"),
		"lines":        mpTopLines(text, 20),
		"body_snippet": truncate(text, 1600),
	}, nil
}

type mpTOTPAccount struct {
	Name   string
	Secret string
	URL    string
}

func mpParseTOTPAccounts(raw string) ([]mpTOTPAccount, error) {
	var out []mpTOTPAccount
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			return nil, fmt.Errorf("账号配置格式错误：%s", line)
		}
		account := mpTOTPAccount{Name: strings.TrimSpace(parts[0]), Secret: strings.TrimSpace(parts[1])}
		if len(parts) > 2 {
			account.URL = strings.TrimSpace(parts[2])
		}
		if account.Name == "" || account.Secret == "" {
			return nil, fmt.Errorf("账号配置无效：%s", line)
		}
		out = append(out, account)
	}
	if len(out) == 0 {
		return nil, errors.New("没有有效账号配置")
	}
	return out, nil
}

func mpTOTPCode(secret string, at time.Time, digits, period int) (string, int, error) {
	if digits <= 0 {
		digits = 6
	}
	if digits > 10 {
		digits = 10
	}
	if period <= 0 {
		period = 30
	}
	key, err := mpDecodeTOTPSecret(secret)
	if err != nil {
		return "", 0, err
	}
	counter := uint64(at.Unix() / int64(period))
	code := mpHOTP(key, counter, digits)
	remaining := period - int(at.Unix()%int64(period))
	return code, remaining, nil
}

func mpCleanTOTPSecret(secret string) string {
	secret = strings.ToUpper(strings.TrimSpace(secret))
	secret = strings.NewReplacer(" ", "", "-", "", "=", "").Replace(secret)
	return secret
}

func mpDecodeTOTPSecret(secret string) ([]byte, error) {
	secret = mpCleanTOTPSecret(secret)
	if secret == "" {
		return nil, errors.New("TOTP 密钥不能为空")
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
}

func mpHOTP(key []byte, counter uint64, digits int) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	bin := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, bin%mod)
}

type mpXiaomiClient struct {
	baseURL  string
	password string
	timeout  time.Duration
	hc       *http.Client
	token    string
}

func mpNewXiaomiClient(ctx context.Context, cfg map[string]any) (*mpXiaomiClient, error) {
	raw := strings.TrimSpace(pluginString(cfg["router_url"]))
	if raw == "" {
		return nil, errors.New("路由器地址不能为空")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return nil, errors.New("路由器地址无效")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("路由器地址仅支持 http/https")
	}
	if !asBool(cfg["allow_public_router"]) {
		if err := mpValidateRouterHost(ctx, u.Hostname()); err != nil {
			return nil, err
		}
	}
	password := pluginString(cfg["password"])
	if password == "" {
		return nil, errors.New("路由器登录密码不能为空")
	}
	timeout := time.Duration(pluginInt(cfg["timeout_seconds"], 10)) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &mpXiaomiClient{
		baseURL:  strings.TrimRight(u.String(), "/"),
		password: password,
		timeout:  timeout,
		hc:       &http.Client{Timeout: timeout},
	}, nil
}

func (c *mpXiaomiClient) Status(ctx context.Context) (map[string]any, []map[string]any, error) {
	if err := c.login(ctx); err != nil {
		return nil, nil, err
	}
	var raw map[string]any
	if err := c.getJSON(ctx, fmt.Sprintf("/cgi-bin/luci/;stok=%s/api/misystem/status", c.token), &raw); err != nil {
		return nil, nil, err
	}
	status := mpParseXiaomiStatus(raw)
	ports, _ := c.PortList(ctx)
	return status, ports, nil
}

func (c *mpXiaomiClient) PortList(ctx context.Context) ([]map[string]any, error) {
	if err := c.login(ctx); err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := c.getJSON(ctx, fmt.Sprintf("/cgi-bin/luci/;stok=%s/api/xqnetwork/portforward?ftype=1", c.token), &raw); err != nil {
		return nil, err
	}
	list, _ := raw["list"].([]any)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

func (c *mpXiaomiClient) PortAdd(ctx context.Context, name string, proto, sport int, ip string, dport int) (map[string]any, error) {
	if name == "" || proto == 0 || sport == 0 || ip == "" || dport == 0 {
		return nil, errors.New("端口映射参数不完整")
	}
	if err := c.login(ctx); err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("name", name)
	form.Set("proto", fmt.Sprint(proto))
	form.Set("sport", fmt.Sprint(sport))
	form.Set("ip", ip)
	form.Set("dport", fmt.Sprint(dport))
	var out map[string]any
	err := c.postFormJSON(ctx, fmt.Sprintf("/cgi-bin/luci/;stok=%s/api/xqnetwork/add_redirect", c.token), form, &out)
	return out, err
}

func (c *mpXiaomiClient) PortDelete(ctx context.Context, port, proto int) (map[string]any, error) {
	if port == 0 {
		return nil, errors.New("端口不能为空")
	}
	if err := c.login(ctx); err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("port", fmt.Sprint(port))
	form.Set("proto", fmt.Sprint(proto))
	var out map[string]any
	err := c.postFormJSON(ctx, fmt.Sprintf("/cgi-bin/luci/;stok=%s/api/xqnetwork/delete_redirect", c.token), form, &out)
	return out, err
}

func (c *mpXiaomiClient) QuickToggle(ctx context.Context, cfg map[string]any) (map[string]any, error) {
	name := pluginString(cfg["q_name"])
	proto := mpXiaomiProtoInt(cfg["q_proto"])
	sport := pluginInt(cfg["q_sport"], 0)
	ip := pluginString(cfg["q_ip"])
	dport := pluginInt(cfg["q_dport"], 0)
	if name == "" || proto == 0 || sport == 0 || ip == "" || dport == 0 {
		return nil, errors.New("快捷预设不完整")
	}
	ports, _ := c.PortList(ctx)
	for _, p := range ports {
		if pluginInt(p["srcport"], -1) == sport {
			res, err := c.PortDelete(ctx, sport, pluginInt(p["proto"], proto))
			return map[string]any{"action": "delete", "port": sport, "result": res}, err
		}
	}
	res, err := c.PortAdd(ctx, name, proto, sport, ip, dport)
	return map[string]any{"action": "add", "port": sport, "result": res}, err
}

func (c *mpXiaomiClient) login(ctx context.Context) error {
	if c.token != "" {
		return nil
	}
	var init map[string]any
	if err := c.getJSON(ctx, "/cgi-bin/luci/api/xqsystem/init_info", &init); err != nil {
		return fmt.Errorf("获取 init_info 失败: %w", err)
	}
	key := pluginString(init["key"])
	if key == "" {
		key = pluginString(init["salt"])
	}
	if key == "" {
		key = pluginString(init["pwd"])
	}
	if key == "" {
		key = c.fetchLoginKey(ctx)
	}
	deviceID := strings.NewReplacer(":", "", "-", "").Replace(pluginString(init["routerId"]))
	if deviceID == "" {
		deviceID = strings.NewReplacer(":", "", "-", "").Replace(pluginString(init["id"]))
	}
	if deviceID == "" {
		deviceID = strings.NewReplacer(":", "", "-", "").Replace(pluginString(init["deviceId"]))
	}
	nonce := fmt.Sprintf("0_%s_%d_%d", deviceID, time.Now().Unix(), rand.Intn(10000))
	hashed := mpSHA1Hex(nonce + mpSHA1Hex(c.password+key))
	if asBool(init["newEncryptMode"]) || pluginInt(init["newEncryptMode"], 0) == 1 {
		hashed = mpSHA256Hex(nonce + mpSHA256Hex(c.password+key))
	}
	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", hashed)
	form.Set("logtype", "2")
	form.Set("nonce", nonce)
	var out map[string]any
	if err := c.postFormJSON(ctx, "/cgi-bin/luci/api/xqsystem/login", form, &out); err != nil {
		return err
	}
	if pluginInt(out["code"], -1) != 0 || pluginString(out["token"]) == "" {
		return fmt.Errorf("登录失败: %v", out)
	}
	c.token = pluginString(out["token"])
	return nil
}

func (c *mpXiaomiClient) fetchLoginKey(ctx context.Context) string {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/cgi-bin/luci/web", nil)
	res, err := c.hc.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	text := string(body)
	for _, pattern := range []string{`key\s*[:=]\s*"([A-Za-z0-9]+)"`, `key\s*[:=]\s*'([A-Za-z0-9]+)'`, `Encrypt\([^\)]*["']([A-Za-z0-9]+)["']`} {
		m := regexp.MustCompile(pattern).FindStringSubmatch(text)
		if len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

func (c *mpXiaomiClient) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", c.baseURL+"/cgi-bin/luci/web")
	res, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func (c *mpXiaomiClient) postFormJSON(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("Referer", c.baseURL+"/cgi-bin/luci/web")
	res, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(out)
}

func mpParseXiaomiStatus(data map[string]any) map[string]any {
	wan, _ := data["wan"].(map[string]any)
	cpu, _ := data["cpu"].(map[string]any)
	mem, _ := data["mem"].(map[string]any)
	count, _ := data["count"].(map[string]any)
	online := pluginInt(count["online"], 0)
	return map[string]any{
		"router_name":     firstNonEmpty(pluginString(data["displayName"]), pluginString(data["routername"]), "Xiaomi Router"),
		"hardware":        pluginAnyString(data["hardware"]),
		"online_count":    online,
		"upload":          wan["upload"],
		"download":        wan["download"],
		"upspeed":         wan["upspeed"],
		"downspeed":       wan["downspeed"],
		"upload_human":    mpFormatBytes(mpFloat(wan["upload"])),
		"download_human":  mpFormatBytes(mpFloat(wan["download"])),
		"upspeed_human":   mpFormatSpeed(mpFloat(wan["upspeed"])),
		"downspeed_human": mpFormatSpeed(mpFloat(wan["downspeed"])),
		"cpu_load":        cpu["load"],
		"cpu_temp":        data["temperature"],
		"mem_usage":       mem["usage"],
		"uptime":          data["upTime"],
		"uptime_human":    mpUptime(pluginInt(data["upTime"], 0)),
	}
}

func mpRunSpeedtest(ctx context.Context, cfg, input map[string]any) (map[string]any, error) {
	downloadURL := strings.TrimSpace(pluginString(input["download_url"]))
	if downloadURL == "" {
		downloadURL = pluginString(cfg["download_url"])
	}
	uploadURL := strings.TrimSpace(pluginString(input["upload_url"]))
	if uploadURL == "" {
		uploadURL = pluginString(cfg["upload_url"])
	}
	timeout := time.Duration(pluginInt(cfg["timeout_seconds"], 20)) * time.Second
	allowPrivate := asBool(cfg["allow_private_network"])
	downloadBytes := pluginInt(cfg["download_bytes"], 10000000)
	if downloadBytes <= 0 || downloadBytes > 100000000 {
		downloadBytes = 10000000
	}
	uploadBytes := pluginInt(cfg["upload_bytes"], 2000000)
	if uploadBytes < 0 || uploadBytes > 20000000 {
		uploadBytes = 2000000
	}
	out := map[string]any{}
	if err := mpValidateHTTPURL(ctx, downloadURL, allowPrivate); err != nil {
		return nil, err
	}
	start := time.Now()
	status, _, body, err := mpHTTPRequest(ctx, http.MethodGet, downloadURL, nil, nil, "", timeout)
	elapsed := time.Since(start)
	if err != nil {
		return nil, err
	}
	if len(body) > downloadBytes {
		body = body[:downloadBytes]
	}
	out["download_status"] = status
	out["download_bytes"] = len(body)
	out["download_seconds"] = elapsed.Seconds()
	out["download_mbps"] = mpMbps(len(body), elapsed)
	if uploadURL != "" && uploadBytes > 0 {
		if err := mpValidateHTTPURL(ctx, uploadURL, allowPrivate); err != nil {
			return nil, err
		}
		payload := bytes.Repeat([]byte("p"), uploadBytes)
		start = time.Now()
		status, _, _, err = mpHTTPRequest(ctx, http.MethodPost, uploadURL, bytes.NewReader(payload), map[string]string{"Content-Type": "application/octet-stream"}, "", timeout)
		elapsed = time.Since(start)
		if err != nil {
			out["upload_error"] = err.Error()
		} else {
			out["upload_status"] = status
			out["upload_bytes"] = uploadBytes
			out["upload_seconds"] = elapsed.Seconds()
			out["upload_mbps"] = mpMbps(uploadBytes, elapsed)
		}
	}
	out["tested_at"] = time.Now().Format(time.RFC3339)
	return out, nil
}

func mpHTTPRequest(ctx context.Context, method, rawURL string, body io.Reader, headers map[string]string, cookie string, timeout time.Duration) (int, http.Header, []byte, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "PTPatronus-MoviePilotPlugin/1.0")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if strings.TrimSpace(cookie) != "" {
		req.Header.Set("Cookie", cookie)
	}
	client := &http.Client{Timeout: timeout}
	res, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 20<<20))
	if err != nil {
		return res.StatusCode, res.Header, nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 400 {
		return res.StatusCode, res.Header, data, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	return res.StatusCode, res.Header, data, nil
}

func mpValidateHTTPURL(ctx context.Context, rawURL string, allowPrivate bool) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return errors.New("URL 无效")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("仅支持 HTTP/HTTPS")
	}
	if !allowPrivate {
		return pluginValidatePublicHost(ctx, u.Hostname())
	}
	return nil
}

func mpValidateRouterHost(ctx context.Context, host string) error {
	if ip := net.ParseIP(host); ip != nil {
		if pluginIsPrivateIP(ip) || ip.IsLoopback() {
			return nil
		}
		return fmt.Errorf("已阻止公网路由器地址 %s；如确认需要请开启允许公网地址", host)
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".local") {
		return nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil {
		return err
	}
	for _, addr := range addrs {
		if !(pluginIsPrivateIP(addr.IP) || addr.IP.IsLoopback()) {
			return fmt.Errorf("已阻止解析到公网 IP %s；如确认需要请开启允许公网地址", addr.IP)
		}
	}
	return nil
}

func mpJoinURL(baseURL, endpoint string) (string, error) {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = "/"
	}
	b, err := url.Parse(baseURL)
	if err != nil || b.Hostname() == "" {
		return "", errors.New("站点地址无效")
	}
	e, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	return b.ResolveReference(e).String(), nil
}

func mpHTMLToText(html string) string {
	html = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?is)<br\s*/?>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`(?is)</(tr|div|p|li|td)>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`(?is)<[^>]+>`).ReplaceAllString(html, " ")
	html = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&#34;", `"`, "&#39;", "'").Replace(html)
	return html
}

func mpTopLines(text string, limit int) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func mpSHA1Hex(s string) string {
	sum := sha1.Sum([]byte(s))
	return fmt.Sprintf("%x", sum[:])
}

func mpSHA256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum[:])
}

func mpXiaomiProtoInt(v any) int {
	switch strings.ToLower(strings.TrimSpace(pluginString(v))) {
	case "1", "tcp":
		return 1
	case "2", "udp":
		return 2
	default:
		return 3
	}
}

func mpXiaomiProtoText(v int) string {
	switch v {
	case 1:
		return "tcp"
	case 2:
		return "udp"
	default:
		return "both"
	}
}

func mpFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		f, _ := strconvParseFloat(x)
		return f
	default:
		return 0
	}
}

func strconvParseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscan(strings.TrimSpace(s), &f)
	return f, err
}

func mpFormatBytes(v float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	return fmt.Sprintf("%.2f %s", v, units[i])
}

func mpFormatSpeed(v float64) string {
	return mpFormatBytes(v) + "/s"
}

func mpUptime(seconds int) string {
	if seconds <= 0 {
		return "-"
	}
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	mins := (seconds % 3600) / 60
	return fmt.Sprintf("%d天 %d小时 %d分钟", days, hours, mins)
}

func mpXiaomiStatusText(status map[string]any) string {
	return fmt.Sprintf("路由器：%s\n在线设备：%v\n实时下行：%s\n实时上行：%s\n本期下载：%s\n本期上传：%s\n运行：%s",
		status["router_name"], status["online_count"], status["downspeed_human"], status["upspeed_human"], status["download_human"], status["upload_human"], status["uptime_human"])
}

func mpMbps(bytesLen int, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(bytesLen) * 8 / d.Seconds() / 1000000
}

func mpSpeedtestText(out map[string]any) string {
	lines := []string{fmt.Sprintf("下行：%.2f Mbps", out["download_mbps"])}
	if v, ok := out["upload_mbps"]; ok {
		lines = append(lines, fmt.Sprintf("上行：%.2f Mbps", v))
	}
	return strings.Join(lines, "\n")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
