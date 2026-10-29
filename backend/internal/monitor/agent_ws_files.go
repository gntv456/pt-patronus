package monitor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"github.com/hxpt/ptpatronus/internal/agentrpc"
	"github.com/hxpt/ptpatronus/internal/core/safego"
)

// ── 文件 RPC（*agentConn 实现 agentrpc.AgentFileRPC；AgentWSHub 实现 agentrpc.AgentRPCHub）──

// FileClient 按 agentID 取在线连接的文件 RPC 客户端（实现 agentrpc.AgentRPCHub）。
func (h *AgentWSHub) FileClient(agentID uint) (agentrpc.AgentFileRPC, error) {
	h.mu.Lock()
	ac, ok := h.conns[agentID]
	h.mu.Unlock()
	if !ok {
		return nil, ErrAgentOffline
	}
	return ac, nil
}

// List 列目录。
func (ac *agentConn) List(ctx context.Context, cmd agentrpc.Command) ([]agentrpc.Entry, error) {
	resp, err := ac.sendRPC(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, errors.New(resp.Err)
	}
	return resp.Entries, nil
}

// Mkdir 建目录。
func (ac *agentConn) Mkdir(ctx context.Context, cmd agentrpc.Command) error {
	resp, err := ac.sendRPC(ctx, cmd)
	if err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Err)
	}
	return nil
}

// Delete 删除文件/目录。
func (ac *agentConn) Delete(ctx context.Context, cmd agentrpc.Command) error {
	resp, err := ac.sendRPC(ctx, cmd)
	if err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Err)
	}
	return nil
}

// Rename 同目录改名（小操作，走 sendRPC；NewName 在 cmd 内）。
func (ac *agentConn) Rename(ctx context.Context, cmd agentrpc.Command) error {
	resp, err := ac.sendRPC(ctx, cmd)
	if err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Err)
	}
	return nil
}

// Move 移动（DstDir 在 cmd 内）。
func (ac *agentConn) Move(ctx context.Context, cmd agentrpc.Command) error {
	resp, err := ac.sendRPC(ctx, cmd)
	if err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Err)
	}
	return nil
}

// Copy 复制（DstDir 在 cmd 内）。
func (ac *agentConn) Copy(ctx context.Context, cmd agentrpc.Command) error {
	resp, err := ac.sendRPC(ctx, cmd)
	if err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Err)
	}
	return nil
}

// Download 发 download 命令，用 io.Pipe 桥接 agent 回传的 data 帧，返回流式 ReadCloser 给上层 io.Copy。
func (ac *agentConn) Download(ctx context.Context, cmd agentrpc.Command) (io.ReadCloser, error) {
	if !ac.hasCap(agentrpc.CapabilityFile) {
		return nil, ErrAgentNoFileCap
	}
	id := strconv.FormatUint(atomic.AddUint64(&rpcIDSeq, 1), 10)
	pr, pw := io.Pipe()
	ac.registerStream(id, &streamState{pw: pw})
	if err := ac.writeJSON(AgentEnvelope{Type: "cmd", ID: id, Cmd: &cmd}); err != nil {
		ac.unregisterStream(id)
		return nil, err
	}
	r := &streamReader{PipeReader: pr, cancel: make(chan struct{})}
	safego.Go("agent_ws:302", func() { // 监听断开/调用方关闭：关 pipe 终止 io.Copy，并通知 agent 取消
		select {
		case <-ac.closeCh:
			pw.CloseWithError(ErrAgentDisconnected)
		case <-r.cancel:
			_ = ac.writeJSON(AgentEnvelope{Type: "error", ID: id, Error: "client closed"})
		}
	})
	return r, nil
}

// Upload 发 upload 命令，从 io.Reader 分块读（base64）发 data 帧，Last 后等 agent 完成 resp。
func (ac *agentConn) Upload(ctx context.Context, cmd agentrpc.Command, r io.Reader) error {
	if !ac.hasCap(agentrpc.CapabilityFile) {
		return ErrAgentNoFileCap
	}
	ctx, cancel := context.WithTimeout(ctx, rpcLongTimeout)
	defer cancel()

	id := strconv.FormatUint(atomic.AddUint64(&rpcIDSeq, 1), 10)
	ch := ac.registerPending(id)
	defer ac.unregisterPending(id)

	if err := ac.writeJSON(AgentEnvelope{Type: "cmd", ID: id, Cmd: &cmd}); err != nil {
		return err
	}
	buf := make([]byte, rpcChunkSize)
	for {
		n, rerr := r.Read(buf)
		if n > 0 {
			enc := base64.StdEncoding.EncodeToString(buf[:n])
			if werr := ac.writeJSON(AgentEnvelope{Type: "data", ID: id, Chunk: &agentrpc.Chunk{Data: enc}}); werr != nil {
				return werr
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			_ = ac.writeJSON(AgentEnvelope{Type: "error", ID: id, Error: rerr.Error()})
			return rerr
		}
	}
	if err := ac.writeJSON(AgentEnvelope{Type: "data", ID: id, Chunk: &agentrpc.Chunk{Last: true}}); err != nil {
		return err
	}
	select {
	case env, ok := <-ch:
		if !ok {
			return ErrAgentDisconnected
		}
		if env.Type == "error" {
			return errors.New(env.Error)
		}
		if env.Resp != nil && !env.Resp.OK {
			return errors.New(env.Resp.Err)
		}
		return nil
	case <-ac.closeCh:
		return ErrAgentDisconnected
	case <-ctx.Done():
		_ = ac.writeJSON(AgentEnvelope{Type: "error", ID: id, Error: ctx.Err().Error()})
		return ctx.Err()
	}
}

// sendRPC 发小操作命令（list/mkdir/delete）并等单次 resp；含能力预检查与超时/断开处理。
func (ac *agentConn) sendRPC(ctx context.Context, cmd agentrpc.Command) (*agentrpc.Response, error) {
	if !ac.hasCap(agentrpc.CapabilityFile) {
		return nil, ErrAgentNoFileCap
	}
	ctx, cancel := context.WithTimeout(ctx, rpcShortTimeout)
	defer cancel()

	id := strconv.FormatUint(atomic.AddUint64(&rpcIDSeq, 1), 10)
	ch := ac.registerPending(id)
	defer ac.unregisterPending(id)

	if err := ac.writeJSON(AgentEnvelope{Type: "cmd", ID: id, Cmd: &cmd}); err != nil {
		return nil, err
	}
	select {
	case env, ok := <-ch:
		if !ok {
			return nil, ErrAgentDisconnected
		}
		if env.Type == "error" {
			return nil, errors.New(env.Error)
		}
		if env.Resp == nil {
			return nil, errors.New("agent 返回空响应")
		}
		return env.Resp, nil
	case <-ac.closeCh:
		return nil, ErrAgentDisconnected
	case <-ctx.Done():
		_ = ac.writeJSON(AgentEnvelope{Type: "error", ID: id, Error: ctx.Err().Error()})
		return nil, ctx.Err()
	}
}

func (ac *agentConn) writeJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	ac.writeMu.Lock()
	defer ac.writeMu.Unlock()
	return ac.conn.WriteMessage(websocket.TextMessage, b)
}

func (ac *agentConn) registerPending(id string) chan AgentEnvelope {
	ch := make(chan AgentEnvelope, 1)
	ac.pendingMu.Lock()
	ac.pending[id] = ch
	ac.pendingMu.Unlock()
	return ch
}

func (ac *agentConn) unregisterPending(id string) {
	ac.pendingMu.Lock()
	delete(ac.pending, id)
	ac.pendingMu.Unlock()
}

func (ac *agentConn) registerStream(id string, st *streamState) {
	ac.streamMu.Lock()
	ac.streams[id] = st
	ac.streamMu.Unlock()
}

func (ac *agentConn) unregisterStream(id string) {
	ac.streamMu.Lock()
	delete(ac.streams, id)
	ac.streamMu.Unlock()
}

func (ac *agentConn) captureCaps(env AgentEnvelope) {
	ac.capsMu.Lock()
	defer ac.capsMu.Unlock()
	ac.caps = map[string]bool{}
	for _, c := range env.Caps {
		ac.caps[c] = true
	}
	if env.Version != "" {
		ac.version = env.Version
	}
}

func (ac *agentConn) hasCap(name string) bool {
	ac.capsMu.Lock()
	defer ac.capsMu.Unlock()
	return ac.caps[name]
}

// routeResponse 把 agent 回的 resp/error 路由到等待的小操作/upload（按 requestID）。
func (ac *agentConn) routeResponse(env AgentEnvelope) {
	if env.ID == "" {
		return
	}
	ac.pendingMu.Lock()
	ch, ok := ac.pending[env.ID]
	if ok {
		delete(ac.pending, env.ID)
	}
	ac.pendingMu.Unlock()
	if ok {
		select {
		case ch <- env:
		default:
		}
	}
}

// routeStreamData 把 download 回传的 data 帧解码写入对应 pipe；Last 时关 pipe 结束流。
// 写 pipe 阻塞即反压（readLoop 暂停读下一帧），report 可能短暂延迟——已知限制。
func (ac *agentConn) routeStreamData(env AgentEnvelope) {
	if env.Chunk == nil {
		return
	}
	ac.streamMu.Lock()
	st, ok := ac.streams[env.ID]
	ac.streamMu.Unlock()
	if !ok {
		return
	}
	if data, err := base64.StdEncoding.DecodeString(env.Chunk.Data); err == nil && len(data) > 0 {
		if _, werr := st.pw.Write(data); werr != nil {
			return // reader 已关，忽略
		}
	}
	if env.Chunk.Last {
		st.pw.Close()
		ac.unregisterStream(env.ID)
	}
}

// shutdown 关 closeCh（幂等）+ 关闭所有进行中的 download pipe，唤醒阻塞的 RPC 返回 ErrAgentDisconnected。
func (ac *agentConn) shutdown() {
	ac.closeOnce.Do(func() { close(ac.closeCh) })
	ac.streamMu.Lock()
	for id, st := range ac.streams {
		st.pw.CloseWithError(ErrAgentDisconnected)
		delete(ac.streams, id)
	}
	ac.streamMu.Unlock()
}

func (h *AgentWSHub) register(ac *agentConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[ac.agentID] = ac
}

// registerAuthed 登记已鉴权连接并踢掉同 agent 的旧连接。
// 换手与踢旧在同一把锁内完成：两个并发 hello 不会互相 kick 后留下一条未登记的活连接。
func (h *AgentWSHub) registerAuthed(ac *agentConn) {
	h.mu.Lock()
	old := h.conns[ac.agentID]
	h.conns[ac.agentID] = ac
	h.mu.Unlock()
	if old != nil && old != ac {
		_ = old.conn.Close()
	}
}

// unregister 从表中移除本连接；返回 true 表示它仍是该 agent 的当前连接。
// 被新连接踢掉时返回 false——调用方不得再标离线/灌离线快照（新连接还活着）。
func (h *AgentWSHub) unregister(ac *agentConn) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cur, ok := h.conns[ac.agentID]; ok && cur == ac {
		delete(h.conns, ac.agentID)
		return true
	}
	return false
}

// kickExisting 踢掉同 agent 的旧连接（注册新连接前调用）。
func (h *AgentWSHub) kickExisting(agentID uint) {
	h.mu.Lock()
	old := h.conns[agentID]
	delete(h.conns, agentID)
	h.mu.Unlock()
	if old != nil {
		_ = old.conn.Close()
	}
}
