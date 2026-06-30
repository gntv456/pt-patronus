package workflow

// GraphNode 是工作流图中的一个节点（前端 Vue Flow / Flutter 画布的节点 JSON，GORM 存为 text）。
// Position 仅前端画布坐标，Executor 完全忽略；Params 由节点 ParamSchema 约束。
type GraphNode struct {
	ID       string         `json:"id"`                 // 唯一节点 id（前端生成）
	Type     string         `json:"type"`               // 节点类型键 → Registry.Get
	Name     string         `json:"name"`               // 用户可编辑的实例名（展示用）
	Params   map[string]any `json:"params"`             // 配置参数
	Position Position       `json:"position"`           // 画布坐标（仅前端）
}

// Position 画布坐标。
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// GraphEdge 连接两个节点。条件分支用 SourcePort 编码输出端口标签（如 "true"/"false"）：
// Executor 执行完条件节点后，按 ecx.Next 裁剪——仅保留 SourcePort ∈ ecx.Next 的出边。
type GraphEdge struct {
	ID         string `json:"id"`
	Source     string `json:"source"`                 // 源节点 id
	Target     string `json:"target"`                 // 目标节点 id
	SourcePort string `json:"source_port,omitempty"`  // 条件节点输出端口标签
	TargetPort string `json:"target_port,omitempty"`  // 目标输入端口（多数节点单输入，留空）
}

// EventCondition 事件触发的过滤条件（存 Workflow.EventConditions，[]EventCondition JSON）。
// event.Bus 触发时，按 Field 从事件 data 取值，与 Value 按 Operator 比较，全部命中才运行工作流。
type EventCondition struct {
	Field    string `json:"field"`    // 事件 data 的 key
	Operator string `json:"operator"` // eq|ne|contains|regex
	Value    string `json:"value"`
}
