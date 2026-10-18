package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

// localDockerCall 记录假 dockerd 收到的调用（本机这些出口是 dockerd 的 HTTP 请求，不是 CLI）。
type localDockerCall struct {
	method string
	path   string
	query  string
}

// newDetachedDockerCall 起一个假 dockerd，返回已「客户端断开」的 gin 上下文与被调记录。
// DOCKER_HOST=tcp://… 让 NewDockerLocalClient 走真 HTTP 通路，所以这条红证打的是
// 生产调用链（handler → DockerLocalClient.do → http.Client），不是替身。
func newDetachedDockerCall(t *testing.T, body string, params gin.Params) (*DockerHandler, *gin.Context, *httptest.ResponseRecorder, *[]localDockerCall) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var mu sync.Mutex
	var calls []localDockerCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, localDockerCall{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery})
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))

	reqCtx, cancel := context.WithCancel(context.Background())
	cancel() // 前端 15s 超时先到 / 用户切页：请求上下文已死

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = params
	src := strings.NewReader(body)
	if body == "" {
		src = strings.NewReader("{}")
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/ops/docker/local/x", src).WithContext(reqCtx)
	return &DockerHandler{local: NewDockerLocalClient(), runner: nilRunner{}}, c, w, &calls
}

func assertDockerGotIt(t *testing.T, w *httptest.ResponseRecorder, calls *[]localDockerCall, method, suffix string) {
	t.Helper()
	if len(*calls) == 0 {
		t.Fatal("dockerd 一次都没收到请求 ⇒ 动作被客户端取消掐死在半路（界面只会报失败）")
	}
	got := (*calls)[0]
	if got.method != method || !strings.HasSuffix(got.path, suffix) {
		t.Errorf("该发出的 dockerd 调用不对: want %s …%s, got %s %s?%s", method, suffix, got.method, got.path, got.query)
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Errorf("已下发的动作不该被报成失败, body=%q", w.Body.String())
	}
}

// TestLocalMutatingOutletsSurviveClientCancel #212 的执行级红证（本机 dockerd HTTP 通路）：
// 停/删容器、建/删卷这类动作已经发给 daemon 了，若调用挂在请求上下文上，客户端一断开
// 本地就报 502「上游不可达」，而 daemon 侧多半照样执行完——用户看到失败、反复重试，
// 甚至以为没生效。修法是把上下文与断开解绑（detachedAction），保留服务端预算。
func TestLocalMutatingOutletsSurviveClientCancel(t *testing.T) {
	t.Run("容器 stop", func(t *testing.T) {
		h, c, w, calls := newDetachedDockerCall(t, "", gin.Params{{Key: "name", Value: "ctn1"}, {Key: "action", Value: "stop"}})
		h.LocalContainerAction(c)
		assertDockerGotIt(t, w, calls, http.MethodPost, "/containers/ctn1/stop")
	})

	t.Run("容器删除", func(t *testing.T) {
		h, c, w, calls := newDetachedDockerCall(t, `{"confirm_risk":true}`, gin.Params{{Key: "name", Value: "ctn1"}})
		h.LocalRemove(c)
		assertDockerGotIt(t, w, calls, http.MethodDelete, "/containers/ctn1")
	})

	t.Run("卷创建", func(t *testing.T) {
		h, c, w, calls := newDetachedDockerCall(t, `{"name":"vol1","driver":"local"}`, gin.Params{})
		h.LocalVolumeCreate(c)
		assertDockerGotIt(t, w, calls, http.MethodPost, "/volumes/create")
	})

	t.Run("卷删除", func(t *testing.T) {
		h, c, w, calls := newDetachedDockerCall(t, `{"confirm_risk":true}`, gin.Params{{Key: "name", Value: "vol1"}})
		h.LocalVolumeRemove(c)
		assertDockerGotIt(t, w, calls, http.MethodDelete, "/volumes/vol1")
	})
}
