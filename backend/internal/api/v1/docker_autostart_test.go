// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/monitor"
)

// 「创建并自动启动」的账面：容器建好后 start 失败，从前是 `_ = LocalContainerAction(...)`，
// 响应一律 {ok:true} —— 用户勾了自动启动、界面说创建成功，容器其实躺在退出态里。
// 同时钉住 docker 动作失败时的原因回显（空 stderr 不能变成一条没有原因的 502）。

// newFakeDockerDaemon 造一个假 docker daemon：create 返回固定 id，start 按 startFails 决定成/败。
func newFakeDockerDaemon(t *testing.T, startFails bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/containers/create"):
			_, _ = w.Write([]byte(`{"Id":"abc123","Warnings":[]}`))
		case strings.HasSuffix(r.URL.Path, "/start"):
			if startFails {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"message":"OCI runtime create failed: addr in use"}`))
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}))
}

func postCreate(t *testing.T, startFails bool, body string) map[string]any {
	t.Helper()
	srv := newFakeDockerDaemon(t, startFails)
	t.Cleanup(srv.Close)
	c := &DockerLocalClient{scheme: "http", host: strings.TrimPrefix(srv.URL, "http://")}
	c.tr = c.buildTransport()
	c.client = &http.Client{Transport: c.tr}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &DockerHandler{local: c}
	r.POST("/ops/docker/local/containers/create", h.LocalCreateContainer)
	req := httptest.NewRequest(http.MethodPost, "/ops/docker/local/containers/create", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("创建应返回 200（容器确实建好了），got %d %s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return out
}

const createWithAutoStart = `{"image":"nginx","name":"web","auto_start":true}`

// TestLocalCreateReportsAutoStartFailure 自动启动失败必须回在响应里。
func TestLocalCreateReportsAutoStartFailure(t *testing.T) {
	out := postCreate(t, true, createWithAutoStart)
	if out["ok"] != true {
		t.Fatalf("创建本身成功，ok 不该翻成 false：%v", out)
	}
	if out["id"] != "abc123" {
		t.Fatalf("id=%v want abc123", out["id"])
	}
	if started, _ := out["started"].(bool); started {
		t.Fatalf("start 失败却报 started=true：%v", out)
	}
	reason, _ := out["start_error"].(string)
	if !strings.Contains(reason, "addr in use") {
		t.Fatalf("start_error 要带真实原因（daemon message），got %q", out["start_error"])
	}
}

// TestLocalCreateAutoStartSuccessAccount 正控：起得来时如实报 started=true 且不编原因。
func TestLocalCreateAutoStartSuccessAccount(t *testing.T) {
	out := postCreate(t, false, createWithAutoStart)
	started, ok := out["started"].(bool)
	if !ok || !started {
		t.Fatalf("启动成功应报 started=true：%v", out)
	}
	if _, bad := out["start_error"]; bad {
		t.Fatalf("成功时不该带 start_error：%v", out)
	}
}

// TestLocalCreateWithoutAutoStartHasNoStartAccount 没勾自动启动时不凭空造 started 字段
// （否则「started 缺失」会被界面读成「启动失败」）。
func TestLocalCreateWithoutAutoStartHasNoStartAccount(t *testing.T) {
	out := postCreate(t, true, `{"image":"nginx","name":"web"}`)
	if _, bad := out["started"]; bad {
		t.Fatalf("未勾选自动启动不该出现 started：%v", out)
	}
}

// TestDockerActionErrorBody 失败原因的三级兜底：stderr > stdout > 明确说没输出原因。
func TestDockerActionErrorBody(t *testing.T) {
	cases := []struct {
		name   string
		res    monitor.CommandResult
		want   string
		hasErr bool
	}{
		{"stderr 优先", monitor.CommandResult{Stderr: "Error: No such container: x", Stdout: "pulled"}, "Error: No such container: x", true},
		{"stderr 空时退到 stdout", monitor.CommandResult{Stdout: "cannot kill: permission denied"}, "cannot kill: permission denied", true},
		{"两头都空也不能回空串", monitor.CommandResult{}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dockerActionErrorBody(tc.res)
			if tc.hasErr {
				if got != tc.want {
					t.Errorf("原因=%q want %q", got, tc.want)
				}
				return
			}
			if strings.TrimSpace(got) == "" {
				t.Error("空 stderr 时回了空原因——界面弹一条没有内容的 502")
			}
			if !strings.Contains(got, "退出码") {
				t.Errorf("兜底文案要说明是怎么失败的，got %q", got)
			}
		})
	}
}
