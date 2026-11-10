// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 本机 Docker 面板的两条出口用例（2026-10-13 真机冒烟实测：Windows 原生运行时
// GET /ops/docker/local/list 回 502「上游服务不可用，请稍后重试」，真实原因是
// dial unix /var/run/docker.sock: connect: A socket operation encountered a dead network.
// —— 引擎根本不在，重试永远不会好）。
//
// 用例走的是生产通路 handler → DockerLocalClient.get → http.Client（DOCKER_HOST 只改拨出地址，
// 不替换任何一层），所以红一定红在状态码与文案上，不是红在替身没搭好。

func newLocalDockerListCall(t *testing.T, dockerHost string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	if dockerHost != "" {
		t.Setenv("DOCKER_HOST", dockerHost)
	}
	if handler != nil {
		srv := httptest.NewServer(handler)
		t.Cleanup(srv.Close)
		t.Setenv("DOCKER_HOST", "tcp://"+strings.TrimPrefix(srv.URL, "http://"))
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/ops/docker/local/list", nil)
	(&DockerHandler{local: NewDockerLocalClient(), runner: nilRunner{}}).LocalList(c)
	return w
}

// TestLocalListEngineAbsent 引擎拨不通 ⇒ 503 本域码，且文案要说清「实际在拨哪个地址」。
func TestLocalListEngineAbsent(t *testing.T) {
	w := newLocalDockerListCall(t, "tcp://127.0.0.1:1", nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("引擎不在时该回 503（重试不会自愈），实际 %d, body=%q", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"docker_engine_unavailable"`) {
		t.Errorf("响应里没有本域码 docker_engine_unavailable（泛化 502 会把「去启动 Docker」这个唯一出路抹掉）: %q", body)
	}
	if !strings.Contains(body, "127.0.0.1:1") {
		t.Errorf("文案没带实际连接地址，设过 DOCKER_HOST 的人只会看到一句指向默认路径的提示: %q", body)
	}
	if !strings.Contains(body, "启动") {
		t.Errorf("文案没给出可执行的下一步: %q", body)
	}
}

// TestLocalListEngineAnsweredWrong 引擎答错了仍算 502「稍后重试」——不能被新分类一口吞掉，
// 否则给一台跑着的引擎贴上「请先启动 Docker」。
func TestLocalListEngineAnsweredWrong(t *testing.T) {
	w := newLocalDockerListCall(t, "", func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusInternalServerError)
		_, _ = rw.Write([]byte(`{"message":"daemon internal error"}`))
	})
	if w.Code != http.StatusBadGateway {
		t.Fatalf("引擎自己报错该回 502（这次确实可以重试），实际 %d, body=%q", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "docker_engine_unavailable") {
		t.Errorf("把「引擎答错了」读成了「引擎不在」: %q", w.Body.String())
	}
}

// TestDockerLocalProbeNamesCheckedPathAndWayOut 探测失败必须说清「实际检查了哪个地址」
// 并给出可用出路：前端只拿 available:false 决定要不要显示「本机模式」，这句 error
// 是用户能看到的唯一线索（同 #219：不说程序实际检查的哪个值，用户只能猜）。
func TestDockerLocalProbeNamesCheckedPathAndWayOut(t *testing.T) {
	missing := t.TempDir() + "/definitely-not-docker.sock"
	t.Setenv("DOCKER_HOST", "unix://"+missing)
	_, err := NewDockerLocalClient().Probe()
	if err == nil {
		t.Fatal("不存在的套接字该探测失败")
	}
	msg := err.Error()
	if !strings.Contains(msg, "definitely-not-docker.sock") {
		t.Errorf("探测失败没说出实际检查的地址: %q", msg)
	}
	if !strings.Contains(msg, "DOCKER_HOST") {
		t.Errorf("探测失败只给了「容器是否挂载 socket」这一条对直接跑 exe 的用户不存在的路: %q", msg)
	}
}

// TestDockerLocalEndpointShape endpoint 必须显示客户端真正使用的地址形态：
// unix 套接字与 TCP 是两种完全不同的排查方向（挂 socket vs 起 TCP 监听）。
func TestDockerLocalEndpointShape(t *testing.T) {
	t.Setenv("DOCKER_HOST", "")
	if got := NewDockerLocalClient().endpoint(); got != "unix:///var/run/docker.sock" {
		t.Errorf("默认地址 = %q, want unix:///var/run/docker.sock", got)
	}
	t.Setenv("DOCKER_HOST", "tcp://192.168.1.9:2375")
	if got := NewDockerLocalClient().endpoint(); got != "http://192.168.1.9:2375" {
		t.Errorf("TCP 地址 = %q, want http://192.168.1.9:2375", got)
	}
}

// TestLocalStatsEngineAbsent 统计出口以前把错误整份丢掉，回 200 + {"stats":[]}
// （2026-10-14 隔离实例 :18099/:18101 两轮实测，同一次 /local/list 是 503，stats 却是 200 空数组）。
// 「引擎不可达」被渲染成「没有容器在跑」——和真的零容器完全同形。
func TestLocalStatsEngineAbsent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:1")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/ops/docker/local/stats", nil)
	(&DockerHandler{local: NewDockerLocalClient(), runner: nilRunner{}}).LocalStats(c)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("引擎不可达时该回 503，实际 %d, body=%q", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"stats":[]`) {
		t.Errorf("失败被画成空读数清单: %q", w.Body.String())
	}
}

// TestLocalDockerOutletsAllClassified 是「判据接满」的结构护栏：分类器只接在 /local/list
// 上等于假覆盖——面板开着时 Docker 退出，用户点「停止/删除/看日志」打的都是别的出口，
// 那些出口以前各自抄了一遍 BadGateway。护栏只扫真源码（先剥 // 注释，免得我自己写的
// 说明文字造假红），数的是「还剩几处裸 BadGateway」。
func TestLocalDockerOutletsAllClassified(t *testing.T) {
	src, err := os.ReadFile("docker_local_handlers.go")
	if err != nil {
		t.Fatalf("读不到被测源文件: %v", err)
	}
	var code strings.Builder
	for _, line := range strings.Split(string(src), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		code.WriteString(line)
		code.WriteByte('\n')
	}
	got := strings.Count(code.String(), "errcode.Respond(c, errcode.BadGateway.WithErr(err))")
	if got != 1 {
		t.Errorf("裸 BadGateway 出口 = %d, want 1（只许剩 respondLocalErr 里那一条兜底）⇒ 又有出口绕开统一判据", got)
	}
	if wired := strings.Count(code.String(), "h.respondLocalErr(c, err)"); wired < 12 {
		t.Errorf("走统一判据的出口 = %d, want ≥12（本机 Docker 的动作/日志/详情出口都该被分类）", wired)
	}
}

// TestLocalActionOutletsEngineAbsent 抽两个「引擎在跑时用户才会点到」的出口验接线：
// 列完表 Docker 才退出，点停止/看日志拿到的必须是 503 + 实际地址，而不是「稍后重试」。
func TestLocalActionOutletsEngineAbsent(t *testing.T) {
	cases := []struct {
		name   string
		invoke func(*DockerHandler, *gin.Context)
	}{
		{"容器 stop", func(h *DockerHandler, c *gin.Context) { h.LocalContainerAction(c) }},
		{"容器日志", func(h *DockerHandler, c *gin.Context) { h.LocalLogs(c) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:1")
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "name", Value: "ctn1"}, {Key: "action", Value: "stop"}}
			c.Request = httptest.NewRequest(http.MethodPost, "/ops/docker/local/containers/ctn1/stop", nil)
			tc.invoke(&DockerHandler{local: NewDockerLocalClient(), runner: nilRunner{}}, c)
			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("%s：引擎不在该回 503，实际 %d, body=%q", tc.name, w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "127.0.0.1:1") {
				t.Errorf("%s：没带实际连接地址: %q", tc.name, w.Body.String())
			}
		})
	}
}
