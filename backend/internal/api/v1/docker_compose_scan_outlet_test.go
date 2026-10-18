package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// scanProjectsOnce 按生产形状打一次扫描接口（LocalComposeHandler 是无字段结构体，
// 直接构造即可；根由 COMPOSE_SCAN_DIR 决定，用 t.Setenv 隔离）。
func scanProjectsOnce(t *testing.T) composeProjectListResp {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/ops/docker/local/compose/projects", nil)
	(&LocalComposeHandler{}).ScanProjects(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp composeProjectListResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body %q: %v", w.Body.String(), err)
	}
	return resp
}

// TestScanProjectsExplainsWhyRootUnreadable 缺陷本体：根读不出来时只回
// available=false，原因（不存在？是文件？权限拒绝？）全程无声，界面把三种真相
// 画成同一句「你没挂载目录」。
func TestScanProjectsExplainsWhyRootUnreadable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-root")
	t.Setenv("COMPOSE_SCAN_DIR", missing)

	resp := scanProjectsOnce(t)
	if resp.Available {
		t.Fatal("根不存在应 available=false")
	}
	if resp.ScanError == "" {
		t.Fatal("扫描失败必须带原因（旧行为只回 available=false）")
	}
	if !strings.Contains(resp.ScanError, missing) {
		t.Errorf("原因里要点名检查的是哪个根, got %q", resp.ScanError)
	}
}

// TestScanProjectsNamesHiddenProjects compose 文件读不出来的项目从前被整条丢掉：
// 列表里的数字变小，界面照常显示「扫描目录 X · N 个项目」，用户看不出是被隐藏了
// 还是这里真的没有。
func TestScanProjectsNamesHiddenProjects(t *testing.T) {
	root := t.TempDir()

	good := filepath.Join(root, "good-stack")
	if err := os.MkdirAll(good, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 坏项目：叫 docker-compose.yml 的其实是个目录——findComposeFile 用 Stat 判定，
	// 命中后 ReadFile 必然失败（Linux EISDIR / Windows 拒绝访问）。
	broken := filepath.Join(root, "broken-stack")
	if err := os.MkdirAll(filepath.Join(broken, "docker-compose.yml"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("COMPOSE_SCAN_DIR", root)

	resp := scanProjectsOnce(t)
	if !resp.Available {
		t.Fatalf("根可访问却 available=false, scan_error=%q", resp.ScanError)
	}
	if len(resp.Projects) != 1 {
		t.Fatalf("只应列出读得出的 1 个项目, got %d", len(resp.Projects))
	}
	if !strings.Contains(resp.ScanError, "broken-stack") {
		t.Errorf("被隐藏的项目必须点名（旧行为让它凭空消失）, got %q", resp.ScanError)
	}
	if !strings.Contains(resp.ScanError, "1") {
		t.Errorf("要带上被隐藏的数量, got %q", resp.ScanError)
	}
}
