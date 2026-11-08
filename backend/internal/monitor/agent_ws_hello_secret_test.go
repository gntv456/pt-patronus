package monitor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/hxpt/ptpatronus/internal/db/models"
)

// fakeAgentAuthSecret 模拟真实 Authenticate：空 secret 一律失败（真实实现走 bcrypt），
// 只有 "good" 通过。用来覆盖 0.3.2+ agent 把 secret 放在 hello 消息体的路径。
type fakeAgentAuthSecret struct {
	mu      sync.Mutex
	touches []bool
}

func (f *fakeAgentAuthSecret) Authenticate(agentID uint, secret string) (*models.Agent, error) {
	if secret != "good" {
		return nil, errors.New("bad agent secret")
	}
	return &models.Agent{ID: agentID, TargetID: 10, Enabled: true}, nil
}

func (f *fakeAgentAuthSecret) TouchSeen(_ uint, online bool, _, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.touches = append(f.touches, online)
	return nil
}

func (f *fakeAgentAuthSecret) lastTouch() (online bool, ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.touches) == 0 {
		return false, false
	}
	return f.touches[len(f.touches)-1], true
}

// TestAgentWS_HelloSecretStaysConnected 回归：0.3.2+ agent 不在 URL 带 secret，
// 靠 hello 消息体鉴权（明文 ws 下 secret 不进反代日志）。
//
// 这条路径曾连上即断：HandleWS 把未鉴权的连接预注册进 conns，readLoop 里
// hello 鉴权通过后调 kickExisting(agentID)「踢掉旧连接」——拿到的正是它自己，
// 于是每次握手成功后立刻 Close 自己的连接。agent 端表现为每秒重连、
// 面板 online 永远 false（但 last_seen/version 会被心跳刷到）。
func TestAgentWS_HelloSecretStaysConnected(t *testing.T) {
	var mu sync.Mutex
	var got Metrics
	var gotTarget uint
	hub := NewAgentWSHub(&fakeAgentAuthSecret{}, func(_ context.Context, tid uint, m Metrics) error {
		mu.Lock()
		got, gotTarget = m, tid
		mu.Unlock()
		return nil
	}, time.Second)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws", hub.HandleWS)
	srv := httptest.NewServer(r)
	defer srv.Close()

	// 故意不带 secret：真实 agent 就是这么连的。
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?agent_id=1"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	hello, _ := json.Marshal(AgentEnvelope{Type: "hello", Version: "0.3.2", Secret: "good", Caps: []string{"file"}})
	if err := conn.WriteMessage(websocket.TextMessage, hello); err != nil {
		t.Fatalf("write hello: %v", err)
	}

	// 鉴权通过后应正式入表（FileClient 能拿到且可发 RPC）。
	deadline := time.Now().Add(2 * time.Second)
	var registered bool
	for time.Now().Before(deadline) {
		if _, ferr := hub.FileClient(1); ferr == nil {
			registered = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !registered {
		t.Fatal("hello 鉴权通过后连接未入表（FileClient 取不到）")
	}

	// 连接必须还活着：发一条 report，ingest 必须收到。
	metrics, _ := json.Marshal(Metrics{CPUUsage: 42.5, MemTotal: 2048, Hostname: "hello-host"})
	report, _ := json.Marshal(AgentEnvelope{Type: "report", Version: "0.3.2", Metrics: metrics})
	if err := conn.WriteMessage(websocket.TextMessage, report); err != nil {
		t.Fatalf("write report: %v（连接已被动关闭＝hello 后自己踢自己）", err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		cpu := got.CPUUsage
		mu.Unlock()
		if cpu != 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	cpu, target := got.CPUUsage, gotTarget
	mu.Unlock()
	if cpu != 42.5 {
		t.Fatalf("ingest CPUUsage = %v, want 42.5（连接没撑过 hello 鉴权后的自踢）", cpu)
	}
	if target != 10 {
		t.Errorf("ingest targetID = %d, want 10", target)
	}
}

// TestAgentWS_KickedConnDoesNotMarkOffline 回归：重连时新连接踢掉旧连接，
// 旧连接的断开清理不得把 agent 标离线 / 灌一条离线快照——新连接还活着。
// 旧行为：unregister 后无条件 TouchSeen(false)+ingest(Online:false)，
// 面板会闪一下离线并可能触发一次离线告警（重连本来不该告警）。
func TestAgentWS_KickedConnDoesNotMarkOffline(t *testing.T) {
	auth := &fakeAgentAuthSecret{}
	var mu sync.Mutex
	offlineIngests := 0
	hub := NewAgentWSHub(auth, func(_ context.Context, _ uint, m Metrics) error {
		if !m.Online {
			mu.Lock()
			offlineIngests++
			mu.Unlock()
		}
		return nil
	}, time.Second)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws", hub.HandleWS)
	srv := httptest.NewServer(r)
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?agent_id=1"

	dial := func() *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		hello, _ := json.Marshal(AgentEnvelope{Type: "hello", Version: "0.3.2", Secret: "good", Caps: []string{"file"}})
		if err := conn.WriteMessage(websocket.TextMessage, hello); err != nil {
			t.Fatalf("write hello: %v", err)
		}
		return conn
	}

	c1 := dial()
	defer c1.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := hub.FileClient(1); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("c1 未入表")
		}
		time.Sleep(10 * time.Millisecond)
	}

	c2 := dial()
	defer c2.Close()
	// 等服务端真正踢掉 c1：读侧报错即说明旧连接已关闭。
	_ = c1.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := c1.ReadMessage(); err == nil {
		t.Fatal("旧连接应被新连接踢掉")
	}
	time.Sleep(200 * time.Millisecond) // 留出 HandleWS 清理窗口

	mu.Lock()
	n := offlineIngests
	mu.Unlock()
	if n != 0 {
		t.Errorf("踢旧连接产生了 %d 次离线快照（新连接仍在线，不该标离线）", n)
	}
	if on, ok := auth.lastTouch(); !ok || !on {
		t.Errorf("最后一次 TouchSeen online=%v（ok=%v），want true：旧连接的清理把在线的 agent 标离线了", on, ok)
	}
	if _, err := hub.FileClient(1); err != nil {
		t.Errorf("踢旧连接后 FileClient 应仍可用：%v", err)
	}
}
