package agent

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hxpt/ptpatronus/internal/sysinfo"
)

const (
	initialBackoff = 1 * time.Second
	maxBackoff     = 60 * time.Second
	// writeDeadline 单帧写超时：半开 TCP（NAT 超时/拔线）时内核缓冲写满会永久阻塞，
	// 必须设 deadline 让写失败触发重连，否则 agent 假死直到 OS 级 TCP 超时（15min+）。
	writeDeadline = 15 * time.Second
	// readDeadline 读超时：远超正常 ticker 间隔，只为兜底半开连接；超时即重连。
	readDeadline = 3 * time.Minute
	// pingInterval 应用层保活：间隔须小于 readDeadline 且大于上报周期常见值。
	pingInterval = 60 * time.Second
)

// Run 运行 agent：外层指数退避重连，连上后定时采集上报。阻塞至 ctx 取消。
func Run(ctx context.Context, cfg Config) {
	backoff := initialBackoff
	for {
		if ctx.Err() != nil {
			return
		}
		connected, err := dialAndServe(ctx, cfg)
		if ctx.Err() != nil {
			return
		}
		if connected {
			backoff = initialBackoff // 连上过则重置退避（短暂断线快速重连）
		}
		slog.Warn("agent disconnected, reconnecting", "err", err, "backoff", backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// agentConn 封装一次 WS 连接：串行写（gorilla Conn 非并发写安全）+ 命令分发。
type agentConn struct {
	conn     *websocket.Conn
	writeMu  sync.Mutex
	cfg      Config
	uploadMu sync.Mutex
	uploads  map[string]*os.File // upload 流式：requestID → 写盘文件
}

// writeJSON 串行化写（readLoop 的命令响应与 ticker 的上报共用一把 writeMu）。
// 每帧设写超时：对端不读时 WriteMessage 会在 writeDeadline 内失败而非永久阻塞。
func (a *agentConn) writeJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	_ = a.conn.SetWriteDeadline(time.Now().Add(writeDeadline))
	return a.conn.WriteMessage(websocket.TextMessage, b)
}

// dialAndServe 建立一次 WS 连接并服务：发 hello → 起读泵 → 定时上报。
// connected=true 表示建连成功过（即使后来断开），供 Run 决定是否重置退避。
func dialAndServe(ctx context.Context, cfg Config) (connected bool, err error) {
	u := buildURL(cfg.ServerURL, cfg.AgentID, cfg.Secret)
	hdr := http.Header{}
	hdr.Set("X-Agent-ID", strconv.FormatUint(uint64(cfg.AgentID), 10))

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(ctx, u, hdr)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	a := &agentConn{conn: conn, cfg: cfg, uploads: map[string]*os.File{}}
	if e := a.sendHello(); e != nil {
		return true, e
	}

	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()
	// 读超时兜底 + 应用层 ping 保活：服务端正常不会主动发 ping，
	// readDeadline 到期或 write 失败都会触发重连，消除半开连接假死。
	_ = conn.SetReadDeadline(time.Now().Add(readDeadline))
	go a.readLoop(cancel) // 读泵：解析服务端消息并分发；出错即 cancel → 上报循环退出

	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	ticker := time.NewTicker(cfg.ReportInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx2.Done():
			return true, ctx2.Err()
		case <-ticker.C:
			if e := a.sendReport(); e != nil {
				return true, e
			}
		case <-ping.C:
			if e := a.writeJSON(Envelope{Type: TypePing}); e != nil {
				return true, e
			}
		}
	}
}

// readLoop 读服务端消息并分发（命令/取消）；出错即 cancel 触发重连。
// 与上报循环并发写 conn，均经 writeMu 串行化。任一消息到达即续读超时。
func (a *agentConn) readLoop(cancel context.CancelFunc) {
	defer cancel()
	// 断连/退出时清理所有 pending upload：关闭文件句柄 + 删半成品，防 FD 与磁盘泄漏
	// （原先仅 cancel()，a.uploads 内 *os.File 不 Close、半成品文件残留磁盘）。
	defer a.cleanupPendingUploads()
	for {
		_, payload, err := a.conn.ReadMessage()
		if err != nil {
			return
		}
		_ = a.conn.SetReadDeadline(time.Now().Add(readDeadline))
		var env Envelope
		if json.Unmarshal(payload, &env) != nil {
			continue
		}
		a.dispatch(env)
	}
}

// cleanupPendingUploads 关闭并删除所有未完成的上传文件（readLoop 退出/断连时调用）。
func (a *agentConn) cleanupPendingUploads() {
	a.uploadMu.Lock()
	ids := make([]string, 0, len(a.uploads))
	for id := range a.uploads {
		ids = append(ids, id)
	}
	a.uploadMu.Unlock()
	for _, id := range ids {
		a.cancelUpload(id)
	}
}

// dispatch 按消息类型分发。
func (a *agentConn) dispatch(env Envelope) {
	switch env.Type {
	case TypeCmd:
		a.handleCmd(env)
	case TypeData:
		a.handleData(env)
	case TypeError:
		a.cancelUpload(env.ID)
	}
}

func (a *agentConn) sendHello() error {
	// secret 只走消息体（0.3.2+ 服务端已就绪）。曾同时在 URL query 双发，
	// ws:// 明文部署下会进反代/访问日志，已移除 query 通道。
	return a.writeJSON(Envelope{Type: TypeHello, Version: AgentVersion, Caps: []string{CapabilityFile}, Secret: a.cfg.Secret})
}

func (a *agentConn) sendReport() error {
	metrics, err := json.Marshal(sysinfo.CollectSystemMetrics())
	if err != nil {
		return err
	}
	return a.writeJSON(Envelope{Type: TypeReport, Version: AgentVersion, Metrics: metrics})
}

// buildURL 拼 WS 上报地址：serverURL + /api/v1/monitor/agents/ws?agent_id=。
// secret 不再放 query（明文 ws 下会泄漏进反代/访问日志），鉴权走 hello 消息体。
func buildURL(serverURL string, agentID uint, secret string) string {
	q := url.Values{}
	q.Set("agent_id", strconv.FormatUint(uint64(agentID), 10))
	sep := "/"
	if strings.HasSuffix(serverURL, "/") {
		sep = ""
	}
	return serverURL + sep + "api/v1/monitor/agents/ws?" + q.Encode()
}
