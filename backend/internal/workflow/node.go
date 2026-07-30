// Package workflow 实现可视化节点图工作流引擎：节点契约 + 注册表 + DAG 执行器。
// 借鉴 MoviePilot 的 Workflow（节点+边 JSON、拓扑执行、共享 Context），单 goroutine 顺序执行
// （SQLite 单写、避免竞态，不抄线程池）。节点图 JSON 形状（GraphNode/GraphEdge）是 Web/Flutter 共用契约。
package workflow

import "context"

// Node 是一个工作流节点。节点是无状态单例（构造期注入所需 chain/oper，注册到 Registry）；
// 每次执行通过 params 传配置、通过 *Context 传/改共享状态。节点间用 *Context.Torrents 等集合传递数据。
type Node interface {
	// Type 唯一节点类型键（如 "fetch_rss"），即 GraphNode.Type，Registry 按它查找。
	Type() string
	// Name 中文展示名。
	Name() string
	// Category 分组：资源/过滤/下载/通知/数据/站点/逻辑/流程（前端节点库按此分组）。
	Category() string
	// Description 一句话说明。
	Description() string
	// Icon 图标 key（前端 GlassIcon/图标映射）。
	Icon() string
	// DefaultParams 新建节点时的默认参数（与 ParamSchema 对齐）。
	DefaultParams() map[string]any
	// ParamSchema 配置字段 schema，前端据此动态渲染表单。
	ParamSchema() []ParamField
	// Execute 执行节点逻辑，可读写 *Context。返回 error 触发 fail-fast（默认）。
	Execute(ctx context.Context, params map[string]any, ecx *Context) error
}

// ParamField 描述一个配置字段，供前端渲染表单（type 决定控件）。
type ParamField struct {
	Name     string        `json:"name"`              // params map 的 key
	Label    string        `json:"label"`             // 中文标签
	Type     string        `json:"type"`              // text|number|select|textarea|switch|kv|site_select|downloader_select
	Options  []ParamOption `json:"options,omitempty"` // type=select 时
	Required bool          `json:"required,omitempty"`
	Default  any           `json:"default,omitempty"`
	Help     string        `json:"help,omitempty"`
}

// ParamOption select 选项。
type ParamOption struct {
	Label string `json:"label"`
	Value any    `json:"value"`
}

// NodeInfo 是 /workflows/actions 目录端点返回的节点元信息（不含 Execute）。
type NodeInfo struct {
	Type          string         `json:"type"`
	Name          string         `json:"name"`
	Category      string         `json:"category"`
	Description   string         `json:"description"`
	Icon          string         `json:"icon"`
	DefaultParams map[string]any `json:"default_params"`
	ParamSchema   []ParamField   `json:"param_schema"`
}
