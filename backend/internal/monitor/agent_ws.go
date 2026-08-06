package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/hxpt/ptpatronus/internal/agentrpc"
	"github.com/hxpt/ptpatronus/internal/db/models"
)

// 文件 RPC 超时。
const (
	rpcShortTimeout = 30 * time.Second // 小操作 list/mkdir/delete
	rpcLongTimeout  = 10 * time.Minute // 流式 upload/download
)

// 流式分块大小（payload；base64 后约 43KB，< gorilla 默认帧限制，内存可控、可反压）。

// 流式分块大小（payload；base64 后约 43KB，< gorilla 默认帧限制，内存可控、可反压）。
const rpcChunkSize = 32 * 1024

// 文件 RPC 错误。

// 文件 RPC 错误。
var (
	ErrAgentOffline      = errors.New("agent 离线")
	ErrAgentNoFileCap    = errors.New("agent 不支持文件管理（请升级 agent 到 0.2.0+）")
	ErrAgentDisconnected = errors.New("agent 连接已断开")
)

// rpcIDSeq 跨连接共享的 RPC 请求 ID 自增（仅需全局唯一）。

// rpcIDSeq 跨连接共享的 RPC 请求 ID 自增（仅需全局唯一）。
var rpcIDSeq uint64

// AgentAuthenticator 抽象 oper.AgentOper 的鉴权/状态方法（由 oper 层注入，避免 monitor→oper 反向依赖，与 Store 接口同理）。

// AgentAuthenticator 抽象 oper.AgentOper 的鉴权/状态方法（由 oper 层注入，避免 monitor→oper 反向依赖，与 Store 接口同理）。
type AgentAuthenticator interface {
	Authenticate(agentID uint, secret string) (*models.Agent, error)
	TouchSeen(agentID uint, online bool, version, hostname, ip string) error
}

// AgentIngestFunc 接收 agent 上报的 Metrics（= Service.IngestAgentReport），由 service 层注入。
type AgentIngestFunc func(ctx context.Context, targetID uint, m Metrics) error

// AgentEnvelope 是 agent ↔ server 的 WS 消息信封（agent 端 internal/agent/protocol.go 独立定义，JSON 对齐）。

// AgentEnvelope 是 agent ↔ server 的 WS 消息信封（agent 端 internal/agent/protocol.go 独立定义，JSON 对齐）。
type AgentEnvelope struct {
	Type    string             `json:"type"`              // hello | report | ping | cmd | resp | data | error
	Version string             `json:"version,omitempty"` // agent 版本号（report/hello 带上）
	ID      string             `json:"id,omitempty"`      // RPC 请求 ID（cmd/resp/data/error 带）
	Metrics json.RawMessage    `json:"metrics,omitempty"` // sysinfo.Metrics 序列化（report 带上）
	Caps    []string           `json:"caps,omitempty"`    // hello 带的能力清单
	Cmd     *agentrpc.Command  `json:"cmd,omitempty"`     // server→agent 命令
	Resp    *agentrpc.Response `json:"resp,omitempty"`    // agent→server 小操作响应
	Chunk   *agentrpc.Chunk    `json:"chunk,omitempty"`   // 双向分块数据
	Error   string             `json:"error,omitempty"`   // 异常终止/取消
}

// AgentWSHub 管理所有 agent 的 WS 连接（单实例，进程内 map；多副本部署需上 Redis，与 ws.Broadcaster 同约束）。
// 职责：握手鉴权（secret bcrypt）、读循环上报 Metrics→Ingest、文件 RPC 转发（cmd→agent、resp/data→调用方）、在线状态维护。
type AgentWSHub struct {
	mu          sync.Mutex
	conns       map[uint]*agentConn // agentID → 连接
	auth        AgentAuthenticator
	ingest      AgentIngestFunc
	readTimeout time.Duration // 读超时（兜底半开连接）
}

type agentConn struct {
	conn     *websocket.Conn
	agentID  uint
	targetID uint

	writeMu sync.Mutex // 串行化写（gorilla Conn 非并发写安全）

	capsMu  sync.Mutex
	caps    map[string]bool // 从 hello 捕获的能力
	version string          // 从 hello/report 捕获

	pendingMu sync.Mutex
	pending   map[string]chan AgentEnvelope // requestID → 小操作/upload 完成 resp 通道（buffered 1）

	streamMu sync.Mutex
	streams  map[string]*streamState // download 流式：requestID → pipe writer

	closeCh   chan struct{}
	closeOnce sync.Once
}

// streamState 一次 download 流的状态（pipe writer 由 routeStreamData 写、上层 io.Copy 读）。
type streamState struct {
	pw *io.PipeWriter
}

// streamReader 包装 io.PipeReader：Close 时通知等待 goroutine 退出（防泄漏）+ 关 pipe。
type streamReader struct {
	*io.PipeReader
	cancel chan struct{}
	once   sync.Once
}

func (r *streamReader) Close() error {
	r.once.Do(func() { close(r.cancel) })
	return r.PipeReader.Close()
}

// NewAgentWSHub 构造。readTimeout<=0 默认 30s。
func NewAgentWSHub(auth AgentAuthenticator, ingest AgentIngestFunc, readTimeout time.Duration) *AgentWSHub {
	if readTimeout <= 0 {
		readTimeout = 30 * time.Second
	}
	return &AgentWSHub{conns: map[uint]*agentConn{}, auth: auth, ingest: ingest, readTimeout: readTimeout}
}

var agentUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true }, // agent 是非浏览器客户端，鉴权靠 secret 不靠 Origin
}

// HandleWS 是 agent WS 升级入口（GET /api/v1/monitor/agents/ws?agent_id=&secret=）。
// 路由放 guarded 外（WS 不能走 header JWT 鉴权）。生产强制 wss://（反代 TLS），secret 走 query 非 TLS 会被嗅探。
func (h *AgentWSHub) HandleWS(c *gin.Context) {
	agentID, _ := strconv.ParseUint(c.Query("agent_id"), 10, 64)
	secret := c.Query("secret")
	if agentID == 0 || secret == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing agent_id or secret"})
		return
	}
	agent, err := h.auth.Authenticate(uint(agentID), secret)
	if err != nil || agent == nil || !agent.Enabled || agent.TargetID == 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid agent credentials"})
		return
	}
	// 拒绝重复连接：同 agent 新连接踢掉旧连接（防多副本/僵尸连接）。
	h.kickExisting(agent.ID)

	conn, err := agentUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	ac := &agentConn{
		conn:     conn,
		agentID:  agent.ID,
		targetID: agent.TargetID,
		caps:     map[string]bool{},
		pending:  map[string]chan AgentEnvelope{},
		streams:  map[string]*streamState{},
		closeCh:  make(chan struct{}),
	}
	h.register(ac)
	_ = h.auth.TouchSeen(ac.agentID, true, "", "", c.ClientIP())

	h.readLoop(ac) // 阻塞至断开

	// 断开清理：标离线 + 灌 Metrics{Online:false} 走 Evaluate 触发离线告警（复用 notifyCooldown 防抖动）。
	h.unregister(ac)
	_ = h.auth.TouchSeen(ac.agentID, false, "", "", "")
	_ = h.ingest(context.Background(), ac.targetID, Metrics{Online: false})
}

// readLoop 读 agent 上报的消息；读超时或错误即返回（触发上层断开清理）。
func (h *AgentWSHub) readLoop(ac *agentConn) {
	defer ac.conn.Close()
	defer ac.shutdown() // 关 closeCh + 清 pending/stream，唤醒所有阻塞的 RPC 返回 ErrAgentDisconnected
	for {
		_ = ac.conn.SetReadDeadline(time.Now().Add(h.readTimeout))
		_, payload, err := ac.conn.ReadMessage()
		if err != nil {
			return
		}
		var env AgentEnvelope
		if json.Unmarshal(payload, &env) != nil {
			continue
		}
		switch env.Type {
		case "hello":
			ac.captureCaps(env)
			_ = h.auth.TouchSeen(ac.agentID, true, env.Version, "", "")
		case "report":
			var m Metrics
			if err := json.Unmarshal(env.Metrics, &m); err != nil {
				continue
			}
			m.Online = true // agent 能上报即在线
			if err := h.ingest(context.Background(), ac.targetID, m); err != nil {
				slog.Warn("ingest agent report failed", "agent", ac.agentID, "err", err)
			}
			_ = h.auth.TouchSeen(ac.agentID, true, env.Version, m.Hostname, "")
		case "ping":
			_ = h.auth.TouchSeen(ac.agentID, true, "", "", "")
		case "resp", "error":
			ac.routeResponse(env)
		case "data":
			ac.routeStreamData(env)
		}
	}
}
