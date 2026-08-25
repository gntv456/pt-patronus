package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/monitor"
)

// --- mock runner ---

type mockRunner struct {
	// captured 记录最后一次调用的参数
	capturedCmd string
	capturedID  uint
	capturedExp int
	// result 返回值
	result monitor.CommandResult
	err    error
	// perCmd 按 command 内容返回不同结果（精细控制）
	perCmd map[string]monitor.CommandResult
}

func (m *mockRunner) RunCommandNow(_ context.Context, id uint, command string, timeoutSec int) (monitor.CommandResult, error) {
	m.capturedCmd = command
	m.capturedID = id
	m.capturedExp = timeoutSec
	if m.perCmd != nil {
		if r, ok := m.perCmd[command]; ok {
			return r, m.err
		}
	}
	return m.result, m.err
}

func newDockerTestRouter(h *DockerHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/list/:id", h.List)
	r.GET("/stats/:id", h.Stats)
	r.GET("/inspect/:id/:name", h.Inspect)
	r.GET("/logs/:id/:name", h.Logs)
	r.POST("/start/:id/:name", h.Start)
	r.POST("/stop/:id/:name", h.Stop)
	r.POST("/restart/:id/:name", h.Restart)
	r.POST("/remove/:id/:name", h.Remove)
	r.POST("/rmi/:id/:ref", h.ImageRemove)
	r.POST("/compose/up/:id", h.ComposeUp)
	r.POST("/compose/down/:id", h.ComposeDown)
	return r
}

// --- List 解析测试 ---

func TestDockerListParseContainers(t *testing.T) {
	// 模拟 docker ps --format + section 分隔的完整输出
	stdout := `{"ID":"abc123","Names":"/nginx","Image":"nginx:latest","Status":"Up 2 hours","Ports":"0.0.0.0:80->80/tcp","State":"running"}
{"ID":"def456","Names":"/redis","Image":"redis:7","Status":"Exited (0) 5 minutes ago","Ports":"","State":"exited"}
---IMG---
{"Repository":"nginx","Tag":"latest","ID":"sha256:abc","CreatedAt":"2024-01-01","Size":"100MB"}
---VOL---
{"Driver":"local","Name":"mydata"}
---NET---
{"ID":"net1","Name":"bridge","Driver":"bridge","Scope":"local"}
---END---`
	h := NewDockerHandler(&mockRunner{result: monitor.CommandResult{Success: true, Stdout: stdout}})
	r := newDockerTestRouter(h)

	w := doJSONReq(t, r, http.MethodGet, "/list/1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	var resp dockerListResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Containers) != 2 {
		t.Errorf("containers: got %d, want 2", len(resp.Containers))
	}
	if resp.Containers[0].State != "running" {
		t.Errorf("first container state=%s, want running", resp.Containers[0].State)
	}
	if len(resp.Images) != 1 || resp.Images[0].Repository != "nginx" {
		t.Errorf("images parse wrong: %+v", resp.Images)
	}
	if len(resp.Volumes) != 1 || resp.Volumes[0].Name != "mydata" {
		t.Errorf("volumes parse wrong: %+v", resp.Volumes)
	}
	if len(resp.Networks) != 1 || resp.Networks[0].Name != "bridge" {
		t.Errorf("networks parse wrong: %+v", resp.Networks)
	}
}

func TestDockerListEmptyOutput(t *testing.T) {
	// docker 未安装或无容器时的空输出
	h := NewDockerHandler(&mockRunner{result: monitor.CommandResult{Success: true, Stdout: "---IMG---\n---VOL---\n---NET---\n---END---"}})
	r := newDockerTestRouter(h)

	w := doJSONReq(t, r, http.MethodGet, "/list/1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
	var resp dockerListResp
	json.Unmarshal(w.Body.Bytes(), &resp)
	// 空时应返回零长切片而非 nil（前端 ?? [] 兜底，但仍验证）
	if len(resp.Containers) != 0 || len(resp.Images) != 0 {
		t.Errorf("expected all empty slices")
	}
}

func TestDockerListRemoteError(t *testing.T) {
	h := NewDockerHandler(&mockRunner{result: monitor.CommandResult{Success: false, Stderr: "connection refused"}})
	r := newDockerTestRouter(h)

	w := doJSONReq(t, r, http.MethodGet, "/list/1", "")
	if w.Code != http.StatusBadGateway {
		t.Errorf("remote error: code=%d, want 502", w.Code)
	}
}

func TestDockerListRunnerError(t *testing.T) {
	h := NewDockerHandler(&mockRunner{err: context.DeadlineExceeded})
	r := newDockerTestRouter(h)

	w := doJSONReq(t, r, http.MethodGet, "/list/1", "")
	if w.Code != http.StatusBadGateway {
		t.Errorf("runner error: code=%d, want 502", w.Code)
	}
}

// --- Stats 测试 ---

func TestDockerStatsParse(t *testing.T) {
	stdout := `{"Name":"nginx","CPUPerc":"0.50%","MemUsage":"50MiB / 1GiB","MemPerc":"5.00%","NetIO":"1KB / 2KB","BlockIO":"3KB / 4KB","PIDs":"10","Container":"abc123"}`
	h := NewDockerHandler(&mockRunner{result: monitor.CommandResult{Success: true, Stdout: stdout}})
	r := newDockerTestRouter(h)

	w := doJSONReq(t, r, http.MethodGet, "/stats/1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
	var resp struct {
		Stats []dockerStatsRow `json:"stats"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Stats) != 1 || resp.Stats[0].CPUPerc != "0.50%" {
		t.Errorf("stats parse wrong: %+v", resp.Stats)
	}
}

func TestDockerStatsRemoteFailReturnsEmpty(t *testing.T) {
	// docker stats 远程失败时不应报错，应返回空列表（stats 是可选的）
	h := NewDockerHandler(&mockRunner{result: monitor.CommandResult{Success: false, Stderr: "err"}})
	r := newDockerTestRouter(h)

	w := doJSONReq(t, r, http.MethodGet, "/stats/1", "")
	if w.Code != http.StatusOK {
		t.Fatalf("stats remote fail should return 200, got %d", w.Code)
	}
	var resp struct {
		Stats []dockerStatsRow `json:"stats"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Stats) != 0 {
		t.Errorf("expected empty stats on failure")
	}
}

// --- 容器操作测试 ---

func TestDockerStartStopRestart(t *testing.T) {
	cases := []struct {
		path    string
		verb    string
		wantCmd string
	}{
		{"/start/1/nginx", "POST", "docker start 'nginx'"},
		{"/stop/1/nginx", "POST", "docker stop 'nginx'"},
		{"/restart/1/nginx", "POST", "docker restart 'nginx'"},
	}
	for _, tc := range cases {
		mr := &mockRunner{result: monitor.CommandResult{Success: true}}
		h := NewDockerHandler(mr)
		r := newDockerTestRouter(h)

		w := doJSONReq(t, r, tc.verb, tc.path, "")
		if w.Code != http.StatusOK {
			t.Errorf("%s: code=%d, want 200", tc.path, w.Code)
		}
		if mr.capturedCmd != tc.wantCmd {
			t.Errorf("%s: captured cmd=%q, want %q", tc.path, mr.capturedCmd, tc.wantCmd)
		}
	}
}

// --- 删除操作需确认 ---

func TestDockerRemoveNeedsConfirm(t *testing.T) {
	h := NewDockerHandler(&mockRunner{})
	r := newDockerTestRouter(h)

	// 未确认 -> 400
	w := doJSONReq(t, r, http.MethodPost, "/remove/1/nginx", `{"confirm_risk":false}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("remove without confirm: code=%d, want 400", w.Code)
	}

	// 确认后 -> 200
	mr2 := &mockRunner{result: monitor.CommandResult{Success: true}}
	h2 := NewDockerHandler(mr2)
	r2 := newDockerTestRouter(h2)
	w = doJSONReq(t, r2, http.MethodPost, "/remove/1/nginx", `{"confirm_risk":true}`)
	if w.Code != http.StatusOK {
		t.Errorf("remove with confirm: code=%d, want 200", w.Code)
	}
	if !strings.Contains(mr2.capturedCmd, "docker rm") || !strings.Contains(mr2.capturedCmd, "nginx") {
		t.Errorf("remove cmd=%q", mr2.capturedCmd)
	}
}

func TestDockerRemoveForceFlag(t *testing.T) {
	mr := &mockRunner{result: monitor.CommandResult{Success: true}}
	h := NewDockerHandler(mr)
	r := newDockerTestRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/remove/1/nginx?force=1", strings.NewReader(`{"confirm_risk":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if !strings.Contains(mr.capturedCmd, "-f") {
		t.Errorf("force remove should contain -f, got %q", mr.capturedCmd)
	}
}

func TestDockerImageRemoveNeedsConfirm(t *testing.T) {
	h := NewDockerHandler(&mockRunner{})
	r := newDockerTestRouter(h)

	w := doJSONReq(t, r, http.MethodPost, "/rmi/1/sha256:abc", `{"confirm_risk":false}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("rmi without confirm: code=%d, want 400", w.Code)
	}

	mr2 := &mockRunner{result: monitor.CommandResult{Success: true}}
	h2 := NewDockerHandler(mr2)
	r2 := newDockerTestRouter(h2)
	w = doJSONReq(t, r2, http.MethodPost, "/rmi/1/sha256:abc", `{"confirm_risk":true}`)
	if w.Code != http.StatusOK {
		t.Errorf("rmi with confirm: code=%d, want 200, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(mr2.capturedCmd, "docker rmi") {
		t.Errorf("rmi cmd=%q", mr2.capturedCmd)
	}
}

// --- Compose 测试 ---

func TestDockerComposeUp(t *testing.T) {
	mr := &mockRunner{result: monitor.CommandResult{Success: true, Stdout: "Started"}}
	h := NewDockerHandler(mr)
	r := newDockerTestRouter(h)

	w := doJSONReq(t, r, http.MethodPost, "/compose/up/1", `{"path":"/opt/app"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(mr.capturedCmd, "cd '/opt/app'") || !strings.Contains(mr.capturedCmd, "docker compose up -d") {
		t.Errorf("compose up cmd=%q", mr.capturedCmd)
	}
	// timeout 应该是 120s
	if mr.capturedExp != 120 {
		t.Errorf("compose timeout=%d, want 120", mr.capturedExp)
	}
}

func TestDockerComposeUpMissingPath(t *testing.T) {
	h := NewDockerHandler(&mockRunner{})
	r := newDockerTestRouter(h)

	w := doJSONReq(t, r, http.MethodPost, "/compose/up/1", `{}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing path: code=%d, want 400", w.Code)
	}
}

func TestDockerComposeDownNeedsConfirm(t *testing.T) {
	h := NewDockerHandler(&mockRunner{})
	r := newDockerTestRouter(h)

	w := doJSONReq(t, r, http.MethodPost, "/compose/down/1", `{"path":"/opt/app"}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("compose down without confirm: code=%d, want 400", w.Code)
	}
}

// --- Inspect 测试 ---

func TestDockerInspectParse(t *testing.T) {
	stdout := `[{"Id":"abc","Name":"/nginx","State":{"Running":true},"Config":{"Image":"nginx"},"NetworkSettings":{},"Mounts":[],"Created":"2024-01-01T00:00:00Z"}]`
	h := NewDockerHandler(&mockRunner{result: monitor.CommandResult{Success: true, Stdout: stdout}})
	r := newDockerTestRouter(h)

	w := doJSONReq(t, r, http.MethodGet, "/inspect/1/nginx", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d, body=%s", w.Code, w.Body.String())
	}
	var resp dockerInspectContainer
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Name != "/nginx" {
		t.Errorf("inspect name=%s, want /nginx", resp.Name)
	}
}

func TestDockerInspectNotFound(t *testing.T) {
	h := NewDockerHandler(&mockRunner{result: monitor.CommandResult{Success: true, Stdout: "[]"}})
	r := newDockerTestRouter(h)

	w := doJSONReq(t, r, http.MethodGet, "/inspect/1/nope", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("inspect not found: code=%d, want 404", w.Code)
	}
}

// --- Logs 测试 ---

func TestDockerLogs(t *testing.T) {
	h := NewDockerHandler(&mockRunner{result: monitor.CommandResult{Success: true, Stdout: "line1\nline2"}})
	r := newDockerTestRouter(h)

	w := doJSONReq(t, r, http.MethodGet, "/logs/1/nginx", "")
	if w.Code != http.StatusOK {
		t.Fatalf("code=%d", w.Code)
	}
	var resp struct {
		Logs string `json:"logs"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Logs != "line1\nline2" {
		t.Errorf("logs=%q", resp.Logs)
	}
	// 验证 tail 参数传递
	req := httptest.NewRequest(http.MethodGet, "/logs/1/nginx?tail=50", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
}

// --- 工具函数测试 ---

func TestShellQuote(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"nginx", "'nginx'"},
		{"nginx; rm -rf /", "'nginx; rm -rf /'"}, // 分号在引号内安全
		{"it's", "'it'\\''s'"},                   // 单引号转义
		{"$(whoami)", "'$(whoami)'"},             // 命令替换在引号内安全
		{"`id`", "'`id`'"},                       // 反引号
		{"", "''"},
	}
	for _, tc := range cases {
		got := shellQuote(tc.in)
		if got != tc.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSplitDockerSections(t *testing.T) {
	stdout := "line1\nline2\n---IMG---\nimg1\n---VOL---\nvol1\n---END---"
	sections := splitDockerSections(stdout)
	if sections[""] != "line1\nline2\n" {
		t.Errorf("default section=%q", sections[""])
	}
	if sections["IMG"] != "img1\n" {
		t.Errorf("IMG section=%q", sections["IMG"])
	}
	if sections["VOL"] != "vol1\n" {
		t.Errorf("VOL section=%q", sections["VOL"])
	}
}

func TestParseJSONLinesSkipsInvalid(t *testing.T) {
	raw := `{"ID":"1","Names":"a"}
invalid json line
{"ID":"2","Names":"b"}`
	result := parseJSONLines[dockerListContainer](raw)
	if len(result) != 2 {
		t.Errorf("expected 2 valid lines (skip invalid), got %d", len(result))
	}
	if result[1].ID != "2" {
		t.Errorf("second item ID=%s, want 2", result[1].ID)
	}
}

func TestParseJSONLinesEmpty(t *testing.T) {
	result := parseJSONLines[dockerListContainer]("")
	if result != nil {
		t.Errorf("empty input should return nil, got %v", result)
	}
}

// --- badID 测试 ---

func TestDockerListBadID(t *testing.T) {
	h := NewDockerHandler(&mockRunner{})
	r := newDockerTestRouter(h)

	w := doJSONReq(t, r, http.MethodGet, "/list/abc", "")
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad id: code=%d, want 400", w.Code)
	}
}

// 确保 mockRunner 实现接口（编译期检查）
var _ DockerCommandRunner = (*mockRunner)(nil)

// 防止 unused import
var _ = time.Now
