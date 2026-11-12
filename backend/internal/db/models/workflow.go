// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Code split from models.go. Package models defines GORM models for the persistence layer.
package models

import "time"

// Workflow — 可视化节点图工作流定义。nodes/edges 为前端节点画布 JSON（见 workflow.GraphNode/GraphEdge）。
// trigger_type 决定触发方式：timer(cron 定时)/event(事件总线)/manual(仅手动)。
// state: idle(空闲)/running(执行中)/success(上次成功)/failed(上次失败)。单 admin 应用，无 user_id。
type Workflow struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	Name            string     `gorm:"size:128;not null" json:"name"`
	Description     string     `gorm:"type:text" json:"description"`
	TriggerType     string     `gorm:"size:16;default:manual;index" json:"trigger_type"` // timer|event|manual
	Timer           string     `gorm:"size:64" json:"timer"`                             // cron 表达式（trigger_type=timer）
	EventType       string     `gorm:"size:64" json:"event_type"`                        // 监听事件类型（trigger_type=event）
	EventConditions string     `gorm:"type:text" json:"event_conditions"`                // 事件触发条件 JSON（[]workflow.EventCondition）
	Enabled         bool       `gorm:"default:false;index" json:"enabled"`
	State           string     `gorm:"size:16;default:idle" json:"state"` // idle|running|success|failed
	RunCount        int        `gorm:"default:0" json:"run_count"`
	LastRunAt       *time.Time `json:"last_run_at"`
	LastResult      string     `gorm:"type:text" json:"last_result"`
	Nodes           string     `gorm:"type:text" json:"nodes"` // []workflow.GraphNode 序列化 JSON
	Edges           string     `gorm:"type:text" json:"edges"` // []workflow.GraphEdge 序列化 JSON
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// WorkflowRun — 单次工作流执行记录。node_executions 为 []workflow.LogEntry 序列化 JSON。
type WorkflowRun struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	WorkflowID     uint       `gorm:"index;not null" json:"workflow_id"`
	State          string     `gorm:"size:16;default:running;index" json:"state"` // running|success|failed
	Trigger        string     `gorm:"size:16" json:"trigger"`                     // manual|timer|event
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	DurationMS     int64      `json:"duration_ms"`
	NodeExecutions string     `gorm:"type:text" json:"node_executions"` // []LogEntry JSON
	Result         string     `gorm:"type:text" json:"result"`          // 最终摘要/错误
	CreatedAt      time.Time  `gorm:"index" json:"created_at"`          // 清理任务按此索引删旧
}

// RssSubscription — RSS 订阅源。可选关联 SiteID（PT 站 cookie 鉴权 RSS）。
// cron 全局扫描（RSS_CHECK_CRON）按各订阅 interval_min vs last_checked_at 决定是否到点抓取。
type RssSubscription struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Name          string     `gorm:"size:128;not null" json:"name"`
	URL           string     `gorm:"size:512;not null" json:"url"`
	SiteID        uint       `gorm:"index" json:"site_id"` // 0=匿名 RSS；非 0=用站点 cookie 鉴权抓取
	Enabled       bool       `gorm:"default:true;index" json:"enabled"`
	IntervalMin   int        `gorm:"default:30" json:"interval_min"` // 独立轮询间隔（分钟）
	TitleInclude  string     `gorm:"type:text" json:"title_include"` // 标题包含关键词（| 分隔）
	TitleExclude  string     `gorm:"type:text" json:"title_exclude"`
	MinSize       int64      `json:"min_size"` // 字节，0 不过滤
	MaxSize       int64      `json:"max_size"`
	Resolution    string     `gorm:"size:32" json:"resolution"` // 1080p/2160p，空不过滤
	AutoDownload  bool       `gorm:"default:false" json:"auto_download"`
	DownloaderID  uint       `gorm:"index" json:"downloader_id"` // 0=默认下载器
	SavePath      string     `gorm:"size:255" json:"save_path"`
	Category      string     `gorm:"size:64" json:"category"`
	Tags          string     `gorm:"type:text" json:"tags"`         // 逗号分隔
	FilterGroups  string     `gorm:"size:255" json:"filter_groups"` // 绑定规则组（| 分隔，空=不过滤）
	LastCheckedAt *time.Time `json:"last_checked_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// RssItem — RSS 条目去重记录。item_key=sha1(guid)[:16]，与 SubscriptionID 联合唯一。
type RssItem struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	SubscriptionID uint       `gorm:"index:idx_rss_item,priority:1;uniqueIndex:uk_rss_item,priority:1" json:"subscription_id"`
	ItemKey        string     `gorm:"size:64;index:idx_rss_item,priority:2;uniqueIndex:uk_rss_item,priority:2" json:"item_key"`
	Title          string     `gorm:"size:255" json:"title"`
	Link           string     `gorm:"size:512" json:"link"`
	TorrentURL     string     `gorm:"size:512" json:"torrent_url"`
	Size           int64      `json:"size"`
	PubDate        *time.Time `json:"pub_date"`
	Downloaded     bool       `gorm:"default:false" json:"downloaded"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Webhook — 进站 Webhook 接收端点（外部系统 POST /api/v1/webhooks/:token 触发）。
// Token 为 URL 内的密钥（随机），即鉴权凭据；可选 Secret 做 HMAC 签名校验（X-Signature 头）。
// TargetWorkflowID 非空时直接触发该工作流（payload 注入 Variables）；同时广播 "webhook" 事件供 event 型工作流监听。
type Webhook struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	Name             string     `gorm:"size:128;not null" json:"name"`
	Token            string     `gorm:"size:64;uniqueIndex;not null" json:"-"` // URL 密钥（随机 32 hex）；凭据不回传（List 脱敏，Create/Rotate 响应单独下发）
	TargetWorkflowID uint       `gorm:"index" json:"target_workflow_id"`       // 0=不直接触发工作流
	SecretEnc        string     `gorm:"type:text" json:"-"`                    // AES-GCM 加密的 HMAC 校验密钥（可选）
	HasSecret        bool       `gorm:"-" json:"has_secret,omitempty"`         // 回显用（是否设了签名密钥）
	Enabled          bool       `gorm:"default:true" json:"enabled"`
	TriggerCount     int        `gorm:"default:0" json:"trigger_count"`
	LastTriggeredAt  *time.Time `json:"last_triggered_at"`
	LastPayload      string     `gorm:"type:text" json:"-"` // 最近一次请求体（截断，调试用）
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// BrushFlowTask — 刷流任务。按规则在指定站点选种加入下载器，监控做种达到删除规则
// （分享率/保种时长/上传量目标，满足任一）后自动删种，循环刷上传量与分享率。
// 识别：加入的种子打 tag "brushflow:<task_id>"，扫删/限流都基于该 tag 在下载器侧统计，
// 不依赖跨系统 infohash 匹配（站点种子 ID ≠ 下载器 infohash，无法直接对应）。
type BrushFlowTask struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	Name         string `gorm:"size:128;not null" json:"name"`
	SiteID       uint   `gorm:"index;not null" json:"site_id"`
	DownloaderID uint   `gorm:"index;not null" json:"downloader_id"`
	// Enabled 不加 gorm default：字段带 default:true 时，GORM 会把「零值 false」当作
	// 未赋值而从 INSERT 里跳过该列，由 DB 补成 true，连结构体都被回填成 true。
	// 后果是用户在「新建任务时就关掉」的开关被静默翻成开，任务立刻进 10 分钟选种 cron
	// 往下载器加种子。缺省方向也应是安全的：不带 enabled = 不启用。
	Enabled bool `json:"enabled"`
	// 选种规则
	Keyword      string `gorm:"size:128" json:"keyword"`        // 搜种关键字，空=综合
	Category     string `gorm:"size:32" json:"category"`        // 传给 Search 的分类(movie/tv/music/"")
	MinSizeMB    int64  `json:"min_size_mb"`                    // 种子大小下限(MB)，0=不限
	MaxSizeMB    int64  `json:"max_size_mb"`                    // 种子大小上限(MB)，0=不限
	MinSeeders   int    `json:"min_seeders"`                    // 做种人数下限
	MaxSeeders   int    `json:"max_seeders"`                    // 做种人数上限，0=不限
	FreeOnly     bool   `gorm:"default:false" json:"free_only"` // 仅免费种
	Paused       bool   `gorm:"default:false" json:"paused"`    // 加入时暂停(手动放行)
	FilterGroups string `gorm:"size:255" json:"filter_groups"`  // 绑定规则组（| 分隔，空=用体积/做种/Free 硬过滤）
	// 删除规则（满足任一即删；全 0 = 永不自动删）
	DeleteRatio     float64 `json:"delete_ratio"`                         // 分享率达此值删，0=不限
	DeleteSeedHours float64 `json:"delete_seed_hours"`                    // 做种时长达此值删，0=不限
	DeleteUploadMB  int64   `json:"delete_upload_mb"`                     // 累计上传达此值删，0=不限
	DeleteFiles     bool    `gorm:"default:false" json:"delete_files"`    // 删种时是否删文件
	UseShareLimit   bool    `gorm:"default:false" json:"use_share_limit"` // 开启：RunSweep 时对任务种子设下载器原生 share limit（达标即暂停，即时保护，补充应用层扫删）；复用 DeleteRatio/DeleteSeedHours 作阈值
	// 删种增强（0/空=不启用；默认 DeleteLogic=or 保持旧行为）
	DeleteMinUPRateKB   int64   `json:"delete_min_up_rate_kb"`                 // 上传速度低于此值(KB/s)且已完成做种 → 删
	DeleteInactiveHours float64 `json:"delete_inactive_hours"`                 // 最后活跃超过此小时 → 删
	DeleteLogic         string  `gorm:"size:8;default:or" json:"delete_logic"` // or=任一命中即删 / and=已配置项全部命中
	DeleteRuleExpr      string  `gorm:"size:512" json:"delete_rule_expr"`      // 可选布尔表达式（非空时优先于基础条件组合）
	MaxTotalSizeGB      float64 `json:"max_total_size_gb"`                     // 本任务种子总体积上限(GB)，超限按最慢上传淘汰
	// 删种 P1：swarm 人数（NumComplete/NumIncomplete，0=不启用）
	DeleteMaxSwarmSeeders  int `json:"delete_max_swarm_seeders"`  // swarm 做种数 > 此值 → 删
	DeleteMinSwarmLeechers int `json:"delete_min_swarm_leechers"` // swarm 下载数 < 此值（且已做种）→ 删
	// 删种 P1：最小做种时长兜底（防止刚完成做种瞬时速度/swarm 为 0 即被误删）
	DeleteMinSeedMinutes int `gorm:"default:0" json:"delete_min_seed_minutes"` // 已完成做种满此分钟数后才允许瞬时规则（低速/swarm）生效，0=不限
	// 并发与分类
	MaxSeeds   int    `gorm:"default:10" json:"max_seeds"` // 该任务最大同时做种数
	SavePath   string `gorm:"size:255" json:"save_path"`
	CategoryDL string `gorm:"size:64" json:"category_dl"` // 下载器内分类
	// 统计（cron 累加）
	TotalAdded   int64      `gorm:"default:0" json:"total_added"`
	TotalDeleted int64      `gorm:"default:0" json:"total_deleted"`
	TotalUpload  int64      `gorm:"default:0" json:"total_upload"` // 累计上传字节（删种时计入）
	LastSelectAt *time.Time `json:"last_select_at"`
	LastSweepAt  *time.Time `json:"last_sweep_at"`
	LastError    string     `gorm:"type:text" json:"last_error"`
	// LastErrorFrom 标记 last_error 写入方（"select"/"sweep"；空=升级前遗留）。选种/扫删
	// 交错运行（10min/5min）共享一个错误字段：成功侧只清同源错误，否则扫删成功会在
	// 5 分钟内擦掉选种刚写入的诊断，用户永远看不到。
	LastErrorFrom string    `gorm:"size:16" json:"last_error_from"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// BrushFlowSeed — 刷流种子加入历史。用于去重（同任务不重复加同一站点种子）与展示，
// 不维护删除状态（删种判定基于下载器 tag，统计累加在 task 上）。
type BrushFlowSeed struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TaskID       uint      `gorm:"index;not null;uniqueIndex:uk_brush_seed_task_source" json:"task_id"`
	DownloaderID uint      `gorm:"index" json:"downloader_id"`
	SourceKey    string    `gorm:"size:255;uniqueIndex:uk_brush_seed_task_source" json:"source_key"` // 站点种子标识(DownloadURL 优先，否则 ID)，(task_id,source_key) 唯一防重复添加
	Title        string    `gorm:"size:512" json:"title"`
	Size         int64     `json:"size"` // bytes
	SiteID       uint      `json:"site_id"`
	AddedAt      time.Time `json:"added_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AgentConversation — AI 助手对话会话（多轮上下文记忆）。
type AgentConversation struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Title     string    `gorm:"size:255" json:"title"` // 首条用户消息摘要
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AgentMessage — 单条对话消息（user/assistant/tool），含思考过程与工具调用记录。
type AgentMessage struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ConversationID uint      `gorm:"index;not null" json:"conversation_id"`
	Role           string    `gorm:"size:16;not null" json:"role"`          // user | assistant | tool
	Content        string    `gorm:"type:text" json:"content"`              // 文本内容
	Reasoning      string    `gorm:"type:text" json:"reasoning,omitempty"`  // 深度思考过程（reasoning_content / thinking）
	ToolCalls      string    `gorm:"type:text" json:"tool_calls,omitempty"` // JSON: [{id,name,arguments}]（assistant 发起的调用）
	ToolCallID     string    `gorm:"size:64" json:"tool_call_id,omitempty"` // role=tool：回应哪个调用
	Name           string    `gorm:"size:64" json:"name,omitempty"`         // role=tool：工具名
	CreatedAt      time.Time `gorm:"index" json:"created_at"`
}
