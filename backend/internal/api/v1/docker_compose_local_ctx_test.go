package v1

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// composeExecProbe 是 LocalComposeHandler 的执行替身，记下「命令正在跑的那一刻」上下文的状态。
// 判「客户端断开会不会掐掉已下发的动作」只能在执行瞬间取：handler 自己的 `defer cancel()`
// 会在返回时取消派生的超时上下文，那时读到的 canceled 是收尾顺序，不是客户端断开。
type composeExecProbe struct {
	called    bool
	errAtExec error
	hasBudget bool
	args      []string
}

func (p *composeExecProbe) fn(out string) composeOutputFunc {
	return func(ctx context.Context, cmd *exec.Cmd) (string, error) {
		p.called = true
		p.errAtExec = ctx.Err()
		_, p.hasBudget = ctx.Deadline()
		p.args = cmd.Args
		return out, nil
	}
}

// newComposeCall 造一次已「客户端断开」的调用：返回前已经 cancel 掉请求上下文。
func newComposeCall(body string) (*composeExecProbe, *LocalComposeHandler, *gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	p := &composeExecProbe{}
	h := &LocalComposeHandler{output: p.fn("cli output")}
	reqCtx, cancel := context.WithCancel(context.Background())
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/ops/docker/local/compose/x", strings.NewReader(body)).WithContext(reqCtx)
	cancel() // 前端 15s 超时先到 / 用户切页
	return p, h, c, w
}

func assertActionDetached(t *testing.T, p *composeExecProbe, wantSub string) {
	t.Helper()
	if !p.called {
		t.Fatal("没走到执行层（替身未被调用）⇒ 这条出口根本没被验到")
	}
	if p.errAtExec != nil {
		t.Errorf("已下发的 compose 动作不该被客户端取消掐掉, got %v", p.errAtExec)
	}
	if !p.hasBudget {
		t.Error("解绑取消的同时必须保留服务端预算（超时），否则 compose 卡死无人管")
	}
	if !strings.Contains(strings.Join(p.args, " "), wantSub) {
		t.Errorf("替身接到的应是 %q 子命令, got %v", wantSub, p.args)
	}
}

func assertRespondedOK(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200, body=%q", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body %q: %v", w.Body.String(), err)
	}
	if resp["ok"] != true {
		t.Fatalf("执行成功应回 ok:true, got %#v（body=%q）", resp["ok"], w.Body.String())
	}
	if !strings.Contains(string(resp["output"].(string)), "cli output") {
		t.Errorf("CLI 输出要原样带回界面, got %v", resp["output"])
	}
}

// TestLocalComposeUpSurvivesClientCancel #212 的执行级红证：`docker compose up -d` 是分钟级
// 多步动作，请求上下文被客户端取消时 CLI 会被 SIGKILL 在半路，留下建了一半的容器与网络，
// 界面却报失败。从前这条只有「代码里写了 detachedAction」的推断——handler 没有能抓住 ctx 的缝。
// skipIfComposeDirUnwritable CI 沙箱（GitHub runner）没有 /app 写权限，
// writeComposeFile 在替身缝之前就失败（compose_write_failed: mkdir /app:
// permission denied），测不到目标行为——环境不配合时跳过，本地/容器内照跑。
func skipIfComposeDirUnwritable(t *testing.T) {
	t.Helper()
	probe := filepath.Join(localComposeDir, ".probe")
	if err := os.MkdirAll(localComposeDir, 0o755); err != nil {
		t.Skipf("compose 目录不可建（%v），CI 沙箱跳过", err)
	}
	if err := os.WriteFile(probe, []byte("x"), 0o644); err != nil {
		t.Skipf("compose 目录不可写（%v），CI 沙箱跳过", err)
	}
	_ = os.Remove(probe)
}

func TestLocalComposeUpSurvivesClientCancel(t *testing.T) {
	skipIfComposeDirUnwritable(t)
	p, h, c, w := newComposeCall(`{"content":"services:\n  app:\n    image: nginx\n","name":"ptdemo"}`)
	h.ComposeUp(c)
	assertActionDetached(t, p, "up")
	assertRespondedOK(t, w)
}

// TestLocalComposeDownSurvivesClientCancel down 会停删容器与网络：被掐在半路最脏。
func TestLocalComposeDownSurvivesClientCancel(t *testing.T) {
	skipIfComposeDirUnwritable(t)
	p, h, c, w := newComposeCall(`{"content":"services:\n  app:\n    image: nginx\n","name":"ptdemo","confirm_risk":true}`)
	h.ComposeDown(c)
	assertActionDetached(t, p, "down")
	assertRespondedOK(t, w)
}

// TestLocalProjectActionSurvivesClientCancel 扫描项目页的 up/down/restart/pull 走另一条出口
// （runComposeCLIWithFlags），同样必须解绑取消但保留预算。
func TestLocalProjectActionSurvivesClientCancel(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "media-stack")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPOSE_SCAN_DIR", root)

	p, h, c, w := newComposeCall(`{"dir":"` + filepath.ToSlash(proj) + `","action":"pull","confirm_risk":true}`)
	h.ProjectAction(c)
	assertActionDetached(t, p, "pull")
	assertRespondedOK(t, w)
}
