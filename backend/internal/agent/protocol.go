package agent

import (
	"encoding/json"
	"time"
)

// 消息类型（与服务端 monitor.AgentEnvelope 的 JSON 对齐；agent 端独立定义，不 import monitor）。
const (
	TypeHello  = "hello"  // 连接建立后第一条（带版本+能力）
	TypeReport = "report" // 定时上报指标
	TypePing   = "ping"   // 应用级心跳

	TypeCmd   = "cmd"   // server→agent 文件命令
	TypeResp  = "resp"  // agent→server 小操作响应（list/mkdir/delete）
	TypeData  = "data"  // 双向分块数据（download/upload）
	TypeError = "error" // 任一方异常终止（流式中断/取消）
)

// AgentVersion agent 版本号（hello/report 带上；0.2.0 起支持文件管理能力）。
const AgentVersion = "0.2.0"

// CapabilityFile 文件管理能力标识（hello 的 Caps 带上，服务端据此判断是否可做文件 RPC）。
const CapabilityFile = "file"

// Envelope WS 消息信封（agent↔server 双向）。
type Envelope struct {
	Type    string          `json:"type"`
	Version string          `json:"version,omitempty"`
	ID      string          `json:"id,omitempty"`      // RPC 请求 ID（cmd/resp/data/error 带）
	Metrics json.RawMessage `json:"metrics,omitempty"` // sysinfo.Metrics 序列化（report 带上）
	Caps    []string        `json:"caps,omitempty"`    // hello 带的能力清单
	Cmd     *Command        `json:"cmd,omitempty"`     // Type==cmd
	Resp    *Response       `json:"resp,omitempty"`    // Type==resp
	Chunk   *Chunk          `json:"chunk,omitempty"`   // Type==data
	Error   string          `json:"error,omitempty"`   // Type==error
}

// Command server→agent 文件命令（JSON 对齐 agentrpc.Command，agent 独立定义避免 import filemanager）。
type Command struct {
	Op      string `json:"op"`                        // list | mkdir | delete | download | upload | rename | move | copy
	Path    string `json:"path"`                      // 用户视角路径（服务端已钳制）
	Base    string `json:"base,omitempty"`            // agent 端钳制根（深度防御二次校验）
	NewName string `json:"new_name,omitempty"`        // op=rename：新文件名（纯名）
	DstDir  string `json:"dst_dir,omitempty"`         // op=move|copy：目标目录（服务端已钳制）
}

// Response agent→server 小操作响应（JSON 对齐 filemanager.Response）。
type Response struct {
	OK      bool      `json:"ok"`
	Err     string    `json:"err,omitempty"`
	Entries []Entry   `json:"entries"` // op=list
	ModTime time.Time `json:"mod_time,omitempty"`
}

// Entry 文件条目（JSON 对齐 filemanager.Entry，服务端转码）。
type Entry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	IsDir   bool      `json:"is_dir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// Chunk 流式数据块（download 回传 / upload 发送，base64 编码）。
type Chunk struct {
	Seq  int    `json:"seq"`
	Data string `json:"data"`
	Last bool   `json:"last"`
}
