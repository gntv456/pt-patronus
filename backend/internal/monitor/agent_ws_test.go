// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package monitor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/hxpt/ptpatronus/internal/agentrpc"
	"github.com/hxpt/ptpatronus/internal/db/models"
)

// fakeAgentAuth 模拟 AgentAuthenticator：fail=true 时鉴权失败；记录最后 TouchSeen 的 online。
// mu 保护 touchedOnline——readLoop goroutine 写、测试 goroutine 读，-race 必须同步。
type fakeAgentAuth struct {
	agent         *models.Agent
	fail          bool
	mu            sync.Mutex
	touchedOnline *bool
}

func (f *fakeAgentAuth) Authenticate(agentID uint, secret string) (*models.Agent, error) {
	if f.fail {
		return nil, errors.New("bad secret")
	}
	return f.agent, nil
}
func (f *fakeAgentAuth) TouchSeen(agentID uint, online bool, version, hostname, ip string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	o := online
	f.touchedOnline = &o
	return nil
}

// touched 在锁保护下读取最后 TouchSeen 写入的 online 值（跨 goroutine 安全）。
func (f *fakeAgentAuth) touched() *bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.touchedOnline
}

// TestAgentWS_Report 验证：合法 secret 握手成功 + report 消息触发 ingest（Metrics.Online 强制 true）。
func TestAgentWS_Report(t *testing.T) {
	auth := &fakeAgentAuth{agent: &models.Agent{ID: 1, TargetID: 10, Enabled: true}}
	// got/gotTarget 被 readLoop goroutine（ingest 回调）写、测试 goroutine 读，mu 同步过 -race。
	var mu sync.Mutex
	var got Metrics
	var gotTarget uint
	hub := NewAgentWSHub(auth, func(ctx context.Context, tid uint, m Metrics) error {
		mu.Lock()
		defer mu.Unlock()
		got = m
		gotTarget = tid
		return nil
	}, time.Second)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws", hub.HandleWS)
	srv := httptest.NewServer(r)
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?agent_id=1&secret=good"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial should succeed with valid secret: %v", err)
	}
	defer conn.Close()

	// 发 report：metrics 含 CPUUsage=55.5
	metrics, _ := json.Marshal(Metrics{CPUUsage: 55.5, MemTotal: 1000})
	b, _ := json.Marshal(AgentEnvelope{Type: "report", Version: "v1", Metrics: metrics})
	if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("write report: %v", err)
	}

	// 异步 ingest（readLoop goroutine），轮询等待。所有 got/gotTarget 访问都在 mu 下。
	deadline := time.Now().Add(time.Second)
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
	cpuUsage := got.CPUUsage
	online := got.Online
	target := gotTarget
	mu.Unlock()
	if cpuUsage != 55.5 {
		t.Errorf("ingested CPUUsage = %v, want 55.5", cpuUsage)
	}
	if !online {
		t.Error("ingested Metrics.Online should be forced true")
	}
	if target != 10 {
		t.Errorf("ingest targetID = %d, want 10", target)
	}
	if tol := auth.touched(); tol == nil || !*tol {
		t.Error("TouchSeen should be called with online=true on report")
	}
}

// TestAgentWS_BadSecret 验证：错误 secret 握手被拒（401，Dial 失败）。
func TestAgentWS_BadSecret(t *testing.T) {
	auth := &fakeAgentAuth{fail: true}
	hub := NewAgentWSHub(auth, func(context.Context, uint, Metrics) error { return nil }, time.Second)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws", hub.HandleWS)
	srv := httptest.NewServer(r)
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?agent_id=1&secret=bad"
	if _, _, err := websocket.DefaultDialer.Dial(url, nil); err == nil {
		t.Fatal("dial should fail with bad secret")
	}
}

// ---- collectOne 跳过 + IngestAgentReport（push/pull 不冲突）----

// fakeAgentStore 实现 monitor.Store，记录 RecordSnapshot 调用次数。
type fakeAgentStore struct {
	targets   []models.MonitorTarget
	snapshots int
}

func (f *fakeAgentStore) ListEnabled() ([]models.MonitorTarget, error) { return f.targets, nil }
func (f *fakeAgentStore) Get(id uint) (*models.MonitorTarget, error) {
	for i := range f.targets {
		if f.targets[i].ID == id {
			return &f.targets[i], nil
		}
	}
	return nil, errors.New("not found")
}
func (f *fakeAgentStore) Credentials(*models.MonitorTarget) (Credentials, error) {
	return Credentials{}, nil
}
func (f *fakeAgentStore) RecordSnapshot(_ *models.MonitorTarget, m Metrics, _ error) (Metrics, error) {
	f.snapshots++
	return m, nil
}
func (f *fakeAgentStore) RecordProbe(*models.MonitorTarget, bool, error) error     { return nil }
func (f *fakeAgentStore) UpdateRTSPURL(uint, string) error                         { return nil }
func (f *fakeAgentStore) CreateAlert(*models.MonitorAlert) error                   { return nil }
func (f *fakeAgentStore) ResolveOpenAlerts(uint, string) error                     { return nil }
func (f *fakeAgentStore) LastOpenAlert(uint, string) (*models.MonitorAlert, error) { return nil, nil }

func (f *fakeAgentStore) ActiveSilenceUntil(uint, string, time.Time) (*time.Time, error) {
	return nil, nil
}

func noopNotifier(context.Context, string, string, string) {}

// TestCollectOne_SkipsAgentTarget：agent target 不应触发 pull 的 RecordSnapshot。
func TestCollectOne_SkipsAgentTarget(t *testing.T) {
	store := &fakeAgentStore{targets: []models.MonitorTarget{
		{ID: 1, Type: "host", Protocol: "agent", Enabled: true},
	}}
	reg := NewRegistry() // 不注册 host|agent
	svc := NewService(store, reg, noopNotifier, time.Second, nil)

	svc.collectOne(context.Background(), &store.targets[0])

	if store.snapshots != 0 {
		t.Errorf("agent target should NOT be pulled (RecordSnapshot), got %d", store.snapshots)
	}
}

// TestIngestAgentReport：agent target 上报复用 RecordSnapshot；非 agent target 被拒。
func TestIngestAgentReport(t *testing.T) {
	reg := NewRegistry()
	store := &fakeAgentStore{targets: []models.MonitorTarget{
		{ID: 1, Type: "host", Protocol: "agent"},
	}}
	svc := NewService(store, reg, noopNotifier, time.Second, nil)

	if err := svc.IngestAgentReport(context.Background(), 1, Metrics{CPUUsage: 80, Online: true}); err != nil {
		t.Fatalf("IngestAgentReport agent target: %v", err)
	}
	if store.snapshots != 1 {
		t.Errorf("should RecordSnapshot once, got %d", store.snapshots)
	}

	// 非 agent target 应拒绝
	store2 := &fakeAgentStore{targets: []models.MonitorTarget{{ID: 2, Type: "host", Protocol: "ssh"}}}
	svc2 := NewService(store2, reg, noopNotifier, time.Second, nil)
	if err := svc2.IngestAgentReport(context.Background(), 2, Metrics{}); err == nil {
		t.Error("should reject non-agent target")
	}
}

// ---- 文件 RPC（双向 cmd/resp）----

// startAgentHub 启一个挂 HandleWS 的 httptest server，返回 hub、ws url、cleanup。
func startAgentHub(t *testing.T, ingest AgentIngestFunc) (*AgentWSHub, string, func()) {
	t.Helper()
	if ingest == nil {
		ingest = func(context.Context, uint, Metrics) error { return nil }
	}
	auth := &fakeAgentAuth{agent: &models.Agent{ID: 1, TargetID: 10, Enabled: true}}
	hub := NewAgentWSHub(auth, ingest, time.Second)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws", hub.HandleWS)
	srv := httptest.NewServer(r)
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?agent_id=1&secret=good"
	return hub, url, srv.Close
}

// TestAgentWS_RPC_List 验证：hello(caps) → server FileClient.List 发 cmd → agent 回 resp → 服务端收到 entries。
func TestAgentWS_RPC_List(t *testing.T) {
	hub, url, cleanup := startAgentHub(t, nil)
	defer cleanup()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// agent 发 hello 带文件能力
	hello, _ := json.Marshal(AgentEnvelope{Type: "hello", Version: "0.2.0", Caps: []string{"file"}})
	if err := conn.WriteMessage(websocket.TextMessage, hello); err != nil {
		t.Fatalf("write hello: %v", err)
	}
	time.Sleep(100 * time.Millisecond) // 等 server 捕获 caps

	// server 端发起 List（异步）
	type result struct {
		entries []agentrpc.Entry
		err     error
	}
	resCh := make(chan result, 1)
	go func() {
		cli, err := hub.FileClient(1)
		if err != nil {
			resCh <- result{nil, err}
			return
		}
		es, err := cli.List(context.Background(), agentrpc.Command{Op: "list", Path: "/", Base: "/"})
		resCh <- result{es, err}
	}()

	// agent 收到 cmd，回 resp（带一个条目）
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read cmd: %v", err)
	}
	var cmd AgentEnvelope
	_ = json.Unmarshal(payload, &cmd)
	if cmd.Type != "cmd" || cmd.Cmd == nil || cmd.Cmd.Op != "list" {
		t.Fatalf("expected cmd list, got %+v", cmd)
	}
	resp, _ := json.Marshal(AgentEnvelope{Type: "resp", ID: cmd.ID, Resp: &agentrpc.Response{OK: true, Entries: []agentrpc.Entry{{Name: "a.txt", IsDir: false, Size: 42}}}})
	if err := conn.WriteMessage(websocket.TextMessage, resp); err != nil {
		t.Fatalf("write resp: %v", err)
	}

	select {
	case res := <-resCh:
		if res.err != nil {
			t.Fatalf("List err: %v", res.err)
		}
		if len(res.entries) != 1 || res.entries[0].Name != "a.txt" {
			t.Errorf("entries = %+v, want [a.txt]", res.entries)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("List timed out")
	}
}

// TestAgentWS_RPC_OldAgent 验证：旧版 agent（hello 无 caps）→ 文件操作直接返回 ErrAgentNoFileCap，不发 cmd。
func TestAgentWS_RPC_OldAgent(t *testing.T) {
	hub, url, cleanup := startAgentHub(t, nil)
	defer cleanup()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	hello, _ := json.Marshal(AgentEnvelope{Type: "hello", Version: "0.1.0"}) // 无 caps
	conn.WriteMessage(websocket.TextMessage, hello)
	time.Sleep(100 * time.Millisecond)

	cli, err := hub.FileClient(1)
	if err != nil {
		t.Fatalf("FileClient: %v", err)
	}
	if _, err := cli.List(context.Background(), agentrpc.Command{Op: "list"}); !errors.Is(err, ErrAgentNoFileCap) {
		t.Errorf("old agent (no caps) should return ErrAgentNoFileCap, got %v", err)
	}
}

// TestAgentWS_RPC_Disconnect 验证：List 阻塞等 resp 时 agent 断开 → 返回 ErrAgentDisconnected（不泄漏）。
func TestAgentWS_RPC_Disconnect(t *testing.T) {
	hub, url, cleanup := startAgentHub(t, nil)
	defer cleanup()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	hello, _ := json.Marshal(AgentEnvelope{Type: "hello", Version: "0.2.0", Caps: []string{"file"}})
	conn.WriteMessage(websocket.TextMessage, hello)
	time.Sleep(100 * time.Millisecond)

	errCh := make(chan error, 1)
	go func() {
		cli, err := hub.FileClient(1)
		if err != nil {
			errCh <- err
			return
		}
		_, err = cli.List(context.Background(), agentrpc.Command{Op: "list", Path: "/"})
		errCh <- err
	}()
	time.Sleep(100 * time.Millisecond) // 等 server 发出 cmd 并阻塞等 resp

	conn.Close() // agent 断开

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrAgentDisconnected) {
			t.Errorf("disconnect should return ErrAgentDisconnected, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("List should return promptly on disconnect")
	}
}

// writeEnv 序列化并发送一条 WS 消息（测试 helper）。
func writeEnv(t *testing.T, conn *websocket.Conn, env AgentEnvelope) {
	t.Helper()
	b, _ := json.Marshal(env)
	if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// TestAgentWS_RPC_Download 验证：agent 回多个 data 帧 → server io.Pipe 拼接流式返回正确内容。
func TestAgentWS_RPC_Download(t *testing.T) {
	hub, url, cleanup := startAgentHub(t, nil)
	defer cleanup()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	writeEnv(t, conn, AgentEnvelope{Type: "hello", Version: "0.2.0", Caps: []string{"file"}})
	time.Sleep(100 * time.Millisecond)

	type dlResult struct {
		data string
		err  error
	}
	resCh := make(chan dlResult, 1)
	go func() {
		cli, err := hub.FileClient(1)
		if err != nil {
			resCh <- dlResult{"", err}
			return
		}
		rc, err := cli.Download(context.Background(), agentrpc.Command{Op: "download", Path: "/f.bin", Base: "/"})
		if err != nil {
			resCh <- dlResult{"", err}
			return
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		resCh <- dlResult{string(b), err}
	}()

	// agent 收到 download cmd，回两个分块 + Last
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read cmd: %v", err)
	}
	var cmd AgentEnvelope
	if json.Unmarshal(payload, &cmd) != nil || cmd.Type != "cmd" || cmd.Cmd.Op != "download" {
		t.Fatalf("expected cmd download, got %s", payload)
	}
	writeEnv(t, conn, AgentEnvelope{Type: "data", ID: cmd.ID, Chunk: &agentrpc.Chunk{Data: base64.StdEncoding.EncodeToString([]byte("hello "))}})
	writeEnv(t, conn, AgentEnvelope{Type: "data", ID: cmd.ID, Chunk: &agentrpc.Chunk{Data: base64.StdEncoding.EncodeToString([]byte("world"))}})
	writeEnv(t, conn, AgentEnvelope{Type: "data", ID: cmd.ID, Chunk: &agentrpc.Chunk{Last: true}})

	select {
	case res := <-resCh:
		if res.err != nil {
			t.Fatalf("Download err: %v", res.err)
		}
		if res.data != "hello world" {
			t.Errorf("data = %q, want 'hello world'", res.data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Download timed out")
	}
}
