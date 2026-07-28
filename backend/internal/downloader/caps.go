package downloader

// PrioOption 文件优先级取值（前端 Select 选项）。
type PrioOption struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

// Caps 描述某下载器类型支持的 UI 能力位。前端 frontend/src/components/downloader/caps.ts
// 与本结构同源；本文件为后端权威，供 GET /downloaders/caps 暴露给 API 消费方（前端/插件/
// Flutter）。新增下载器类型须在此登记，未登记类型按 minimalCaps 兜底（非 qb 全开）——
// 避免新客户端未登记时 UI 显示一堆后端会 ErrNotImplemented 的按钮（治静态表漂移）。
type Caps struct {
	Category     bool         `json:"category"`     // 单种子 SetCategory
	CategoryCRUD bool         `json:"categoryCRUD"` // 分类管理弹窗（Create/Edit/Delete）
	ForceStart   bool         `json:"forceStart"`   // 强制开始
	QueueFull    bool         `json:"queueFull"`    // 队列 top/up/down/bottom 全支持
	Sequential   bool         `json:"sequential"`   // 顺序下载
	FirstLast    bool         `json:"firstLast"`    // 首尾优先
	Swarm        bool         `json:"swarm"`        // 详情显示 swarm 总数/队列位置/可用性
	Tags         bool         `json:"tags"`         // 多标签
	Location     bool         `json:"location"`     // 移动保存路径
	Trackers     bool         `json:"trackers"`     // Tracker 增删查
	Files        bool         `json:"files"`        // 文件列表与优先级
	Peers        bool         `json:"peers"`        // Peer 列表
	FilePrio     []PrioOption `json:"filePrio"`     // 该类型可用文件优先级
}

// minimalCaps 是未知下载器类型的兜底（最保守：仅文件列表），替代旧的 qb 全开默认。
var minimalCaps = Caps{Files: true, FilePrio: []PrioOption{{Label: "正常", Value: 1}}}

// typeCaps 与前端 caps.ts 的 DL_CAPS 同源。改这里时同步前端（或让前端运行时拉取本端点）。
var typeCaps = map[string]Caps{
	"qb": {
		Category: true, CategoryCRUD: true, ForceStart: true, QueueFull: true,
		Sequential: true, FirstLast: true, Swarm: true,
		Tags: true, Location: true, Trackers: true, Files: true, Peers: true,
		FilePrio: []PrioOption{
			{Label: "不下载", Value: 0}, {Label: "正常", Value: 2},
			{Label: "高", Value: 1}, {Label: "最高", Value: 7},
		},
	},
	"tr": {
		Category: false, CategoryCRUD: false, ForceStart: false, QueueFull: false,
		Sequential: false, FirstLast: false, Swarm: false,
		Tags: true, Location: true, Trackers: true, Files: true, Peers: true,
		FilePrio: []PrioOption{
			{Label: "低", Value: -1}, {Label: "正常", Value: 0}, {Label: "高", Value: 1},
		},
	},
	"ut": {
		Category: true, CategoryCRUD: false, ForceStart: true, QueueFull: true,
		Sequential: false, FirstLast: false, Swarm: true,
		Tags: false, Location: false, Trackers: false, Files: true, Peers: true,
		FilePrio: []PrioOption{
			{Label: "跳过", Value: 0}, {Label: "低", Value: 1},
			{Label: "正常", Value: 2}, {Label: "高", Value: 3},
		},
	},
	"dl": {
		Category: true, CategoryCRUD: false, ForceStart: false, QueueFull: true,
		Sequential: false, FirstLast: false, Swarm: true,
		Tags: false, Location: true, Trackers: true, Files: true, Peers: true,
		FilePrio: []PrioOption{
			{Label: "跳过", Value: 0}, {Label: "低", Value: 1},
			{Label: "正常", Value: 2}, {Label: "高", Value: 3},
		},
	},
	"ar": {
		Category: false, CategoryCRUD: false, ForceStart: false, QueueFull: true,
		Sequential: false, FirstLast: false, Swarm: false,
		Tags: false, Location: false, Trackers: false, Files: true, Peers: true,
		FilePrio: []PrioOption{{Label: "跳过", Value: 0}, {Label: "下载", Value: 1}},
	},
	"rt": {
		Category: true, CategoryCRUD: false, ForceStart: false, QueueFull: false,
		Sequential: false, FirstLast: false, Swarm: false,
		Tags: false, Location: false, Trackers: false, Files: true, Peers: true,
		FilePrio: []PrioOption{
			{Label: "不下载", Value: 0}, {Label: "正常", Value: 1}, {Label: "高", Value: 2},
		},
	},
	"fl": {
		Category: true, CategoryCRUD: false, ForceStart: false, QueueFull: false,
		Sequential: true, FirstLast: false, Swarm: true,
		Tags: true, Location: true, Trackers: true, Files: true, Peers: true,
		FilePrio: []PrioOption{
			{Label: "跳过", Value: 0}, {Label: "正常", Value: 1}, {Label: "高", Value: 2},
		},
	},
}

// Capabilities 返回某下载器类型的能力位；未登记返回 minimalCaps（保守兜底）。
func Capabilities(typ string) Caps {
	if c, ok := typeCaps[typ]; ok {
		return c
	}
	return minimalCaps
}

// AllCapabilities 返回全部已登记类型的能力位（供 GET /downloaders/caps）。返回副本，防调用方改全局表。
func AllCapabilities() map[string]Caps {
	out := make(map[string]Caps, len(typeCaps))
	for k, v := range typeCaps {
		out[k] = v
	}
	return out
}
