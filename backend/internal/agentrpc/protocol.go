// Package agentrpc 定义 agent 文件管理的协议类型与接口（纯类型叶子包，无 oper/monitor 依赖）。
// 供 monitor（实现 AgentFileRPC）、filemanager（消费，AgentBackend 转发）、agent（JSON 对齐，独立定义）共享，
// 以打破 monitor→filemanager→oper→monitor 的 import 循环。
package agentrpc

import (
	"context"
	"io"
	"time"
)

// CapabilityFile 文件管理能力标识（agent hello 的 Caps 带上，服务端据此判断是否可做文件 RPC）。
const CapabilityFile = "file"

// Command server→agent 文件命令（agent 端 agent.Command 的 JSON 对齐镜像，agent 独立定义不 import 本包）。
type Command struct {
	Op        string `json:"op"`                  // list | mkdir | delete | download | upload | rename | move | copy | readfile | writefile | archive | extract | chmod | dirsize
	Path      string `json:"path"`                // 用户视角路径（服务端已 resolvePath 钳制）
	Base      string `json:"base,omitempty"`      // agent 端钳制根（FileStorage.base_path，深度防御二次校验）
	NewName   string `json:"new_name,omitempty"`  // op=rename：新文件名（纯名）
	DstDir    string `json:"dst_dir,omitempty"`   // op=move|copy：目标目录（服务端已钳制）
	Dst       string `json:"dst,omitempty"`       // op=archive|extract：目标压缩包路径 / 解压目录（服务端已钳制）
	Content   string `json:"content,omitempty"`   // op=writefile：文件内容（小文件 ≤10MB）
	Mode      uint32 `json:"mode,omitempty"`      // op=chmod：八进制权限位
	Recursive bool   `json:"recursive,omitempty"` // op=chmod：是否递归
	Checksum  string `json:"checksum,omitempty"`  // op=upgrade：期望 sha256（校验下载完整性）
}

// Response agent→server 小操作响应（list/mkdir/delete）。
type Response struct {
	OK      bool      `json:"ok"`
	Err     string    `json:"err,omitempty"`
	Entries []Entry   `json:"entries"`           // op=list
	Content string    `json:"content,omitempty"` // op=readfile：文件内容
	Size    int64     `json:"size,omitempty"`    // op=dirsize：目录大小
	ModTime time.Time `json:"mod_time,omitempty"`
}

// Entry 文件条目（filemanager.Entry 以 type alias 复用，本地后端与 agent 后端共用）。
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

// AgentFileRPC 单个在线 agent 的文件操作客户端（由 monitor.AgentWSHub 的活跃连接实现）。
type AgentFileRPC interface {
	List(ctx context.Context, cmd Command) ([]Entry, error)
	Mkdir(ctx context.Context, cmd Command) error
	Delete(ctx context.Context, cmd Command) error
	Download(ctx context.Context, cmd Command) (io.ReadCloser, error)
	Upload(ctx context.Context, cmd Command, r io.Reader) error
	Rename(ctx context.Context, cmd Command) error
	Move(ctx context.Context, cmd Command) error
	Copy(ctx context.Context, cmd Command) error
	ReadFile(ctx context.Context, cmd Command) (string, error)
	WriteFile(ctx context.Context, cmd Command) error
	Archive(ctx context.Context, cmd Command) error
	Extract(ctx context.Context, cmd Command) error
	Chmod(ctx context.Context, cmd Command) error
	DirSize(ctx context.Context, cmd Command) (int64, error)
	Upgrade(ctx context.Context, cmd Command) error
}

// AgentRPCHub 按 agentID 取在线 agent 的文件 RPC 客户端（由 *monitor.AgentWSHub 实现）。
type AgentRPCHub interface {
	FileClient(agentID uint) (AgentFileRPC, error)
}
