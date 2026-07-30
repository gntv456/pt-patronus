// Package mcp 实现 Model Context Protocol（MCP）服务端核心子集：JSON-RPC 2.0 over
// streamable HTTP，暴露宿主能力为 MCP 工具（tools/list、tools/call）。
//
// 对标 MoviePilot MCP Server（cyt-666）。协议核心子集（initialize / ping / tools/list /
// tools/call / notifications/initialized）；SSE 流式传输与 resources/prompts 等长尾留后续。
// 仅依赖标准库，便于 httptest 端到端覆盖。
package mcp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ServerName 默认服务端标识。
const ServerName = "ptpatronus-mcp"
const ServerVersion = "1.0.0"

// Settings 控制 MCP 服务端运行态：是否启用 + Bearer 鉴权令牌。
// 由宿主经 SettingsGetter 在每次请求时提供（来源通常是 ptp-mcp-server 插件配置）。
type Settings struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token"` // 非空则要求请求 Authorization: Bearer <token>
}

// Tool 是一个 MCP 工具：名称 + 描述 + JSON Schema 入参 + 处理器。
// 处理器返回文本结果与可选错误（错误转成 isError=true 的内容块，不破坏 JSON-RPC）。
// Timeout 为可选的单工具执行超时；零值由调用方（如 aiagent.ToolRegistry）用默认值。
// Risk/RiskNote 不参与 JSON 序列化（json:"-"）：既不下发到 LLM 的 tools/list，也不进 MCP schema；
// 仅 agent 执行层据此对 destructive 工具做二次确认闸门。
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Timeout     time.Duration  `json:"-"`
	Risk        string         `json:"-"` // RiskRead | RiskWrite | RiskDestructive；空视同 write
	RiskNote    string         `json:"-"` // destructive 工具的一句话危险说明（确认弹窗展示）
	Handler     func(ctx context.Context, args map[string]any) (string, error)
}

// 风险等级。仅 RiskDestructive 驱动 agent 执行层的二次确认闸门；
// read/write 标注供后续工具白名单 UI 分组用（当前非强制全量标注）。
const (
	RiskRead        = "read"        // 只读查询，无副作用
	RiskWrite       = "write"       // 变更可逆/低影响状态
	RiskDestructive = "destructive" // 不可逆/高影响，执行前必须经用户确认
)

// Server 是 MCP JSON-RPC 服务端。线程安全。
type Server struct {
	mu             sync.RWMutex
	tools          map[string]Tool
	settingsGetter func() Settings
}

// NewServer 构造空服务端。
func NewServer() *Server {
	return &Server{tools: make(map[string]Tool), settingsGetter: func() Settings { return Settings{} }}
}

// SetSettingsGetter 注入运行态来源（每次请求调用）。
func (s *Server) SetSettingsGetter(fn func() Settings) {
	if fn == nil {
		fn = func() Settings { return Settings{} }
	}
	s.mu.Lock()
	s.settingsGetter = fn
	s.mu.Unlock()
}

// settings 快照当前运行态。
func (s *Server) settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settingsGetter()
}

// RegisterTool 注册/覆盖一个工具。
func (s *Server) RegisterTool(t Tool) {
	if t.Name == "" || t.Handler == nil {
		return
	}
	s.mu.Lock()
	s.tools[t.Name] = t
	s.mu.Unlock()
}

// ToolList 返回当前工具清单（仅供测试/自省）。
func (s *Server) ToolList() []Tool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Tool, 0, len(s.tools))
	for _, t := range s.tools {
		out = append(out, t)
	}
	return out
}

// ===== JSON-RPC =====

// jsonrpcRequest / jsonrpcResponse 是 JSON-RPC 2.0 信封。
type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
)

// ServeHTTP 实现 http.Handler：POST 单条 JSON-RPC；GET 返服务端能力概要（便于探活）。
// 鉴权：Settings.Enabled=false 返 404；Token 非空则校验 Bearer。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	st := s.settings()
	if !st.Enabled {
		http.NotFound(w, r)
		return
	}
	if st.Token != "" {
		// 常量时间比较：token 可驱动全部 MCP 工具，需防逐字节时序还原。
		if subtle.ConstantTimeCompare([]byte(extractBearer(r)), []byte(st.Token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"server": ServerName, "version": ServerVersion, "tools": len(s.ToolList()),
		})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req jsonrpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPC(w, jsonrpcResponse{JSONRPC: "2.0", ID: nil, Error: &rpcError{codeParseError, "parse error"}})
		return
	}
	resp := s.dispatch(r.Context(), req)
	resp.JSONRPC = "2.0"
	resp.ID = req.ID
	// notifications（无 id）不回包
	if isNotification(req.ID) {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeRPC(w, resp)
}

func writeRPC(w http.ResponseWriter, resp jsonrpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// isNotification：JSON-RPC notification 的 id 为空（null 或缺省）。
func isNotification(id json.RawMessage) bool {
	if len(id) == 0 {
		return true
	}
	return string(id) == "null"
}

// dispatch 路由方法。返回响应（不含 jsonrpc/id 字段，由 ServeHTTP 补齐）。
func (s *Server) dispatch(ctx context.Context, req jsonrpcRequest) jsonrpcResponse {
	switch req.Method {
	case "initialize":
		return jsonrpcResponse{Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo":      map[string]any{"name": ServerName, "version": ServerVersion},
			"capabilities":    map[string]any{"tools": map[string]any{}},
		}}
	case "notifications/initialized":
		// 客户端初始化完成通知，服务端无需回复（ServeHTTP 已处理 202）。
		return jsonrpcResponse{}
	case "ping":
		return jsonrpcResponse{Result: map[string]any{}}
	case "tools/list":
		return jsonrpcResponse{Result: s.toolsListResult()}
	case "tools/call":
		return s.handleToolCall(ctx, req.Params)
	default:
		return jsonrpcResponse{Error: &rpcError{codeMethodNotFound, "method not found: " + req.Method}}
	}
}

// toolsListResult 组装 tools/list 响应。
func (s *Server) toolsListResult() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tools := make([]map[string]any, 0, len(s.tools))
	for _, t := range s.tools {
		schema := t.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		tools = append(tools, map[string]any{
			"name": t.Name, "description": t.Description, "inputSchema": schema,
		})
	}
	return map[string]any{"tools": tools}
}

// toolsCallParams 是 tools/call 的标准入参。
type toolsCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

// handleToolCall 执行工具调用，结果以 MCP content 文本块返回。
func (s *Server) handleToolCall(ctx context.Context, params json.RawMessage) jsonrpcResponse {
	var p toolsCallParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return jsonrpcResponse{Error: &rpcError{codeInvalidParams, "invalid params"}}
		}
	}
	s.mu.RLock()
	t, ok := s.tools[p.Name]
	s.mu.RUnlock()
	if !ok {
		return jsonrpcResponse{Error: &rpcError{codeMethodNotFound, "unknown tool: " + p.Name}}
	}
	out, err := t.Handler(ctx, p.Arguments)
	if err != nil {
		// MCP 约定：工具执行错误以 isError=true 的 content 返回，不破坏 JSON-RPC 成功信封。
		return jsonrpcResponse{Result: map[string]any{
			"isError": true,
			"content": []map[string]any{{"type": "text", "text": "error: " + err.Error()}},
		}}
	}
	if out == "" {
		out = "ok"
	}
	return jsonrpcResponse{Result: map[string]any{
		"content": []map[string]any{{"type": "text", "text": out}},
	}}
}

// extractBearer 取 Authorization: Bearer <token> 的 token 部分。
func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}

// Call（供测试/内部使用）直接以方法+参数调用，返回响应。
func (s *Server) Call(ctx context.Context, id any, method string, params any) jsonrpcResponse {
	raw, _ := json.Marshal(params)
	idRaw, _ := json.Marshal(id)
	return s.dispatch(ctx, jsonrpcRequest{JSONRPC: "2.0", ID: idRaw, Method: method, Params: raw})
}

// CompactJSON 把任意值序列化为压缩 JSON 字符串（宿主注册工具结果文本块时复用）。
func CompactJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
