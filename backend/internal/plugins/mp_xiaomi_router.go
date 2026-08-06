package plugins

import (
	"context"
	"fmt"
	"strings"
)

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
