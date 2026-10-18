package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/monitor"
)

// 会改状态的 compose 命令不该跟着客户端走：浏览器一刷新/切页（或前端 15s 超时先到），
// 挂在请求上下文上的 `docker compose up -d` 会被连子进程一起取消，
// 留下建了一半的容器与网络，而界面只报一句「请求超时」。

// ctxSpy 嵌入接口本身，因此不必知道 DockerCommandRunner 还有哪些方法；
// 只拦 RunCommandNow 记下传进来的 ctx。
type ctxSpy struct {
	DockerCommandRunner
	got context.Context
}

func (s *ctxSpy) RunCommandNow(ctx context.Context, _ uint, _ string, _ int) (monitor.CommandResult, error) {
	s.got = ctx
	return monitor.CommandResult{Success: true, Stdout: "Network created"}, nil
}

func TestRemoteComposeUpSurvivesClientCancel(t *testing.T) {
	spy := &ctxSpy{DockerCommandRunner: nilRunner{}}
	h := &DockerHandler{runner: spy}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/compose/up/:id", h.ComposeUp)

	req := httptest.NewRequest(http.MethodPost, "/compose/up/1", strings.NewReader(`{"path":"/opt/app"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithCancel(req.Context())
	cancel() // 客户端已经断开
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if spy.got == nil {
		t.Fatalf("没打到 runner，请求被更早的校验拦了：code=%d body=%s", w.Code, w.Body.String())
	}
	if err := spy.got.Err(); err != nil {
		t.Fatalf("执行中命令的上下文随客户端取消（旧行为原形），容器会建一半: %v", err)
	}
}

// TestComposeDetachedKeepsValues 钉住助手本身的不变量：
// 取消信号被摘掉，但请求上的取值（trace id 之类）仍要透传下去。
func TestComposeDetachedKeepsValues(t *testing.T) {
	type key struct{}
	parent, cancel := context.WithCancel(context.Background())
	parent = context.WithValue(parent, key{}, "v")
	cancel()

	detached := detachedAction(parent)
	if detached.Err() != nil {
		t.Fatalf("父上下文已取消，detached 不该取消: %v", detached.Err())
	}
	if got, _ := detached.Value(key{}).(string); got != "v" {
		t.Errorf("取值应保留, got %q", got)
	}
}
