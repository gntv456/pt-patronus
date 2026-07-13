package plugins

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// 默认单次 tool 调用超时（与 ExecuteAction 的 ~30s 约定一致）。
const defaultHostToolTimeout = 30 * time.Second

// ErrPermissionDenied 由 Dispatch 在调用方缺少 tool 所需权限时返回。
// externalHostServer/Bridge 据此映射为 403 / 错误。
var ErrPermissionDenied = errors.New("permission denied")

// HostToolHandler 执行一个宿主工具。结果必须可 JSON 序列化；
// 返回 json.RawMessage 可透传预序列化的 JSON（adoptAgentTool 用，避免双重转义）。
type HostToolHandler func(ctx context.Context, input map[string]any) (any, error)

// HostTool 是暴露给插件的宿主能力元数据。Register 时连同 Handler 一起注册。
type HostTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema,omitempty"`
	Permission  string         `json:"permission,omitempty"`  // 空=无需权限（公开）
	Category    string         `json:"category,omitempty"`    // downloader/media/site/... 用于 GET /tools 分组
	Timeout     time.Duration  `json:"-"`                      // >0 覆盖默认 30s（慢操作如 scan/organize）
	Destructive bool           `json:"destructive,omitempty"` // 破坏性（删数据/删文件）；仅元数据，供前端标红
}

// RegisteredHostTool 是已注册的 tool（Handler 不参与 JSON 序列化）。
type RegisteredHostTool struct {
	HostTool
	Handler HostToolHandler `json:"-"`
}

// timeout 返回该 tool 的执行超时（未设则默认 30s）。
func (t RegisteredHostTool) timeout() time.Duration {
	if t.Timeout > 0 {
		return t.Timeout
	}
	return defaultHostToolTimeout
}

// HostToolRegistry 是宿主工具注册表。启动后只读（并发安全）。
// 由 app.go 通过 Manager.SetHostTools 注入；externalHostServer 的
// GET /tools、POST /tools/{name} 与 Manager.Bridge 的 tool.list/tool.call 共用同一实例。
type HostToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]RegisteredHostTool
}

func NewHostToolRegistry() *HostToolRegistry {
	return &HostToolRegistry{tools: make(map[string]RegisteredHostTool)}
}

// Register 注册一个工具；Name 空 / Handler nil 时忽略（防御性，便于批量注册）。
// 同名覆盖（后注册胜出），便于测试与重载。
func (r *HostToolRegistry) Register(t HostTool, h HostToolHandler) {
	if r == nil || t.Name == "" || h == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name] = RegisteredHostTool{HostTool: t, Handler: h}
}

// Lookup 取工具。
func (r *HostToolRegistry) Lookup(name string) (RegisteredHostTool, bool) {
	if r == nil {
		return RegisteredHostTool{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// VisibleTools 返回调用方权限可见的工具元数据（按 Category,Name 排序）。GET /tools 用。
// 复用 hasPluginPermission 做权限过滤（支持 *、host:*、downloader:* 等通配符）。
func (r *HostToolRegistry) VisibleTools(permissions []string) []HostTool {
	if r == nil {
		return []HostTool{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]HostTool, 0, len(r.tools))
	for _, t := range r.tools {
		if t.Permission == "" || hasPluginPermission(permissions, t.Permission) {
			out = append(out, t.HostTool)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category != out[j].Category {
			return out[i].Category < out[j].Category
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Dispatch 执行工具：查表→鉴权→（按 tool.Timeout 设超时）→执行。
// 超时派生自传入的 ctx；调用方仍可在最外层再套一个 ctx。
//
// 返回 (result, requiredPermission, notFound, err)：
//   - notFound=true            → 工具不存在（调用方回 404）
//   - err==ErrPermissionDenied → 权限不足（调用方回 403，带 requiredPermission）
//   - err!=nil                 → 执行失败（调用方回 500）
func (r *HostToolRegistry) Dispatch(ctx context.Context, permissions []string, name string, input map[string]any) (any, string, bool, error) {
	t, ok := r.Lookup(name)
	if !ok {
		return nil, "", true, fmt.Errorf("tool %q not found", name)
	}
	if t.Permission != "" && !hasPluginPermission(permissions, t.Permission) {
		return nil, t.Permission, false, ErrPermissionDenied
	}
	if t.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.timeout())
		defer cancel()
	}
	out, err := t.Handler(ctx, input)
	return out, t.Permission, false, err
}
