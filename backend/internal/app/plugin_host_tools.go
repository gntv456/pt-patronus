package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/mcp"
	"github.com/hxpt/ptpatronus/internal/plugins"
)


// 本文件把宿主全部 chain 能力批量注册成 plugins.HostTool，经统一 Tool RPC（GET /tools、
// POST /tools/{name}、Bridge tool.list/tool.call）暴露给外部插件。两条来源：
//  (1) adopt registerAgentTools 已构建的 35 个 mcp.Tool——零 handler 重复，仅补
//      Permission/Category/Destructive 元数据；handler 返回 json.RawMessage 透传预序列化 JSON。
//  (2) 新增 host-only tool，覆盖 agent_tools 未触及的 chain（mediaserver/cloud/brushflow/
//      rss/subtitle/file/workflow + 下载器高级操作）。
//
// registerAgentTools 是 agent 与 plugin 共享的唯一真源；此处再次调用为相同 chain 构建全新
// 闭包，无全局状态，安全。未在 agentToolMeta 声明权限的 tool 不暴露（安全默认）。

// adoptAgentTool 把一个 mcp.Tool 适配为 plugins.HostTool。
// mcp.Tool.Handler 返回 JSON 字符串，这里包成 json.RawMessage——json.Marshal 会把它当作
// 已序列化的合法 JSON 直接写入，不会双重转义（RawMessage.MarshalJSON 返回原始字节）。
func adoptAgentTool(t mcp.Tool, permission, category string, destructive bool) (plugins.HostTool, plugins.HostToolHandler) {
	return plugins.HostTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
			Permission:  permission,
			Category:    category,
			Destructive: destructive,
		}, func(ctx context.Context, in map[string]any) (any, error) {
			s, err := t.Handler(ctx, in)
			if err != nil {
				return nil, err
			}
			if s == "" {
				return json.RawMessage("null"), nil
			}
			return json.RawMessage(s), nil
		}
}

// toolMeta 是 agent tool 的插件侧元数据补充（agent tool 自身不带权限/分类）。
type toolMeta struct {
	permission  string
	category    string
	destructive bool
}

// agentToolMeta 为每个被 adopt 的 agent tool 指定插件权限、分类、是否破坏性。
// 权限沿用 plugins 包的域级约定（downloader:read|write 等），与 host_capabilities.go 对齐。
var agentToolMeta = map[string]toolMeta{
	"system.info":                {"", "system", false},
	"sites.list":                 {"site:read", "site", false},
	"site_messages":              {"site:read", "site", false},
	"site_freeleech":             {"site:read", "site", false},
	"site_checkin_all":           {"site:action", "site", false},
	"torrent_search":             {"site:read", "site", false},
	"media_search":               {"media:read", "media", false},
	"media_detail":               {"media:read", "media", false},
	"recommend_feeds":            {"media:read", "media", false},
	"subscribe_list":             {"subscribe:read", "subscribe", false},
	"subscribe_create":           {"subscribe:write", "subscribe", false},
	"subscribe_delete":           {"subscribe:write", "subscribe", false},
	"downloaders.list":           {"downloader:read", "downloader", false},
	"torrents_list":              {"downloader:read", "downloader", false},
	"torrents_stats":             {"downloader:read", "downloader", false},
	"torrents_pause":             {"downloader:write", "downloader", false},
	"torrents_resume":            {"downloader:write", "downloader", false},
	"torrents_delete":            {"downloader:write", "downloader", true},
	"torrents_add":               {"downloader:write", "downloader", false},
	"transfer_history":           {"transfer:read", "transfer", false},
	"transfer_organize":          {"transfer:write", "transfer", false},
	"reseed_list":                {"reseed:read", "reseed", false},
	"reseed_run":                 {"reseed:write", "reseed", false},
	"dashboard_overview":         {"dashboard:read", "dashboard", false},
	"dashboard_trend":            {"dashboard:read", "dashboard", false},
	"send_notification":          {"notify:send", "notify", false},
	"filter_builtin_rules_query": {"filter:read", "filter", false},
	"filter_custom_rules_query":  {"filter:read", "filter", false},
	"filter_custom_rule_create":  {"filter:write", "filter", false},
	"filter_custom_rule_update":  {"filter:write", "filter", false},
	"filter_custom_rule_delete":  {"filter:write", "filter", true},
	"filter_rule_groups_query":   {"filter:read", "filter", false},
	"filter_rule_group_create":   {"filter:write", "filter", false},
	"filter_rule_group_update":   {"filter:write", "filter", false},
	"filter_rule_group_delete":   {"filter:write", "filter", true},
}

// registerPluginHostTools 构建 plugin HostTool 注册表并注入 Manager。由 app.go 在所有 chain
// 构造完成后调用（放置顺序是硬约束：闭包捕获 chain 指针，nil 会 panic）。
// agentTools 是 app.go 已构建的 registerAgentTools(...) 结果（同一份闭包共享，agent 侧与
// plugin 侧独立调用，无全局状态）。
func registerPluginHostTools(
	mgr *plugins.Manager,
	agentTools []mcp.Tool,
	dlCh *chain.DownloadChain, subCh *chain.SubscribeChain,
	mediaServerCh *chain.MediaServerChain, cloudCh *chain.CloudChain,
	brushCh *chain.BrushFlowChain, rssCh *chain.RssChain,
	subtitleCh *chain.SubtitleChain, transferCh *chain.TransferChain,
	keepSeedCh *chain.KeepSeedChain, fileCh *chain.FileManagerChain,
	wfCh *chain.WorkflowChain,
) {
	reg := plugins.NewHostToolRegistry()

	// (1) adopt 35 个 agent 工具（未在 agentToolMeta 声明权限的不暴露=安全默认）。
	AdoptAgentTools(reg, agentTools)

	// (2) 新增 host-only 工具（按域）。
	registerDownloaderTools(reg, dlCh)
	registerMediaServerTools(reg, mediaServerCh)
	registerSubscribeTools(reg, subCh)
	registerTransferTools(reg, transferCh)
	registerReseedTools(reg, keepSeedCh)
	registerCloudTools(reg, cloudCh)
	registerBrushFlowTools(reg, brushCh)
	registerRssTools(reg, rssCh)
	registerSubtitleTools(reg, subtitleCh)
	registerFileTools(reg, fileCh)
	registerWorkflowTools(reg, wfCh)

	mgr.SetHostTools(reg)
}

// AdoptAgentTools 把一批已构建的 agent mcp.Tool 按 agentToolMeta adopt 进注册表。
// 由 app.go 在调用 registerAgentTools(...) 拿到真实 tool 列表后调用。
func AdoptAgentTools(reg *plugins.HostToolRegistry, tools []mcp.Tool) {
	for _, t := range tools {
		meta, ok := agentToolMeta[t.Name]
		if !ok {
			continue
		}
		ht, hh := adoptAgentTool(t, meta.permission, meta.category, meta.destructive)
		reg.Register(ht, hh)
	}
}

// ===== 参数/schema 辅助（agent_tools.go 的 noArgs/hashesSchema 是局部或同包；这里补 objSchema/noArgsSchema 包级版） =====

// noArgsSchema 返回无参 tool 的 JSON Schema。与 agent_tools.go 的局部 noArgs 等价（独立副本，互不影响）。
func noArgsSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

// objSchema 构造带属性的对象 schema。
func objSchema(properties map[string]any, required []string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": properties, "required": required}
}

// argFloat 从 LLM/插件输入安全取 float64（JSON 数字默认 float64）。
func argFloat(a map[string]any, key string) float64 {
	v, ok := a[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f
	}
	return 0
}

// intProp 构造一个 integer 属性 schema。
func intProp(desc string) map[string]any {
	m := map[string]any{"type": "integer"}
	if desc != "" {
		m["description"] = desc
	}
	return m
}

// strProp 构造一个 string 属性 schema。
func strProp(desc string) map[string]any {
	m := map[string]any{"type": "string"}
	if desc != "" {
		m["description"] = desc
	}
	return m
}

func boolProp(desc string) map[string]any {
	m := map[string]any{"type": "boolean"}
	if desc != "" {
		m["description"] = desc
	}
	return m
}

func numberProp(desc string) map[string]any {
	m := map[string]any{"type": "number"}
	if desc != "" {
		m["description"] = desc
	}
	return m
}

func decodeToolInput[T any](input map[string]any, out *T) error {
	b, err := json.Marshal(input)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("invalid tool input: %w", err)
	}
	return nil
}

