package workflow

import "github.com/hxpt/ptpatronus/internal/site"

// Context 是单次工作流执行的共享可变状态，在节点间顺序传递。Executor 单线程串行执行，
// 节点直接读写无需加锁。资源类节点产出 ecx.Torrents，过滤类节点裁剪它，下载类节点消费它。
type Context struct {
	// Torrents 资源集合（搜索/RSS 产出，过滤/下载消费）。复用 site.Torrent。
	Torrents []site.Torrent
	// Messages 累积的可读消息（send_message 可读取展示）。
	Messages []string
	// Content 通用文本载荷（http_request 响应体、RSS 原始 XML 等），extract_fields 消费。
	Content string
	// Variables 自由变量（set_variable 存取，供后续节点 params 模板插值）。
	Variables map[string]any
	// Log 运行日志（每个节点执行后由 Executor 追加，最终落 WorkflowRun.NodeExecutions）。
	Log []LogEntry
	// Next 条件节点选中的分支标签（对应 GraphEdge.SourcePort）。Executor 据此激活分支边。
	// Executor 在执行每个节点前清空它，由 condition 节点在 Execute 内重新设置。
	Next []string
	// Stop 节点可置 true 提前成功终止整条流（fail-fast 之外的成功短路）。
	Stop bool
}

// LogEntry 单节点执行日志（落 WorkflowRun.NodeExecutions）。
type LogEntry struct {
	NodeID     string `json:"node_id"`
	NodeType   string `json:"node_type"`
	NodeName   string `json:"node_name"`
	Status     string `json:"status"` // success|failed|skipped
	Message    string `json:"message"`
	DurationMS int64  `json:"duration_ms"`
	At         string `json:"at"` // RFC3339
}
