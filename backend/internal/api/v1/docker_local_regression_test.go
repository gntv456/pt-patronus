package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 回归：容器名必须走 ?name= query——Engine API 忽略 body 里的 Name 字段，
// 不传则 daemon 生成随机名，前端「名称」输入被静默丢弃。
func TestLocalCreateContainerPassesNameQuery(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Id":"abc123"}`))
	}))
	defer srv.Close()

	c := &DockerLocalClient{scheme: "http", host: strings.TrimPrefix(srv.URL, "http://")}
	c.tr = c.buildTransport()
	c.client = &http.Client{Transport: c.tr}
	id, err := c.LocalCreateContainer(context.Background(), map[string]any{"Image": "nginx", "Name": "my-web"})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if id != "abc123" {
		t.Fatalf("id=%q, want abc123", id)
	}
	if gotPath != "/containers/create?name=my-web" {
		t.Fatalf("create path=%q, want /containers/create?name=my-web", gotPath)
	}

	// 无 Name 时保持裸路径
	gotPath = ""
	if _, err := c.LocalCreateContainer(context.Background(), map[string]any{"Image": "nginx"}); err != nil {
		t.Fatalf("create without name failed: %v", err)
	}
	if gotPath != "/containers/create" {
		t.Fatalf("path without name=%q, want /containers/create", gotPath)
	}
}

// 回归：pull 路径按 ref 形态组装——无条件 tag=latest 会覆盖内嵌 tag（拉错版本），
// digest+tag 同给会被 daemon 拒绝。
func TestPullCreatePath(t *testing.T) {
	cases := []struct{ ref, want string }{
		{"nginx", "/images/create?fromImage=nginx&tag=latest"},
		{"nginx:1.25", "/images/create?fromImage=nginx&tag=1.25"},
		// 仓库端口含 ":" 但非 tag：保持原名 + tag=latest（urlEncode 只转义 "/"，":" 在 query 中合法）
		{"registry:5000/app", "/images/create?fromImage=registry:5000%2Fapp&tag=latest"},
		{"registry:5000/app:1.25", "/images/create?fromImage=registry:5000%2Fapp&tag=1.25"},
		// digest 引用：不得再带 tag
		{"nginx@sha256:abc", "/images/create?fromImage=nginx@sha256:abc"},
	}
	for _, tc := range cases {
		if got := pullCreatePath(tc.ref); got != tc.want {
			t.Errorf("pullCreatePath(%q) = %q, want %q", tc.ref, got, tc.want)
		}
	}
}

// 回归：compose 目录校验必须用 clean 后的路径前缀——裸 HasPrefix 可被
// "/host/compose-evil"（同级目录）与 "/host/compose/../etc"（穿越）绕过。
func TestComposeDirWithin(t *testing.T) {
	t.Setenv("COMPOSE_SCAN_DIR", "/host/compose")
	cases := []struct {
		dir  string
		want bool
	}{
		{"/host/compose", true},
		{"/host/compose/app", true},
		{"/host/compose/app/sub", true},
		{"/host/compose-evil/app", false}, // 同级目录绕过
		{"/host/compose/../etc", false},   // 穿越
		{"/etc", false},
		{"app", false},
	}
	for _, tc := range cases {
		if got := composeDirWithin(tc.dir); got != tc.want {
			t.Errorf("composeDirWithin(%q) = %v, want %v", tc.dir, got, tc.want)
		}
	}
}
