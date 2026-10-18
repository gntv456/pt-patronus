package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/site"
)

// #331 端点出口：GET /sites/definitions 必须把「定义诊断」账面带出来，
// 且不能丢掉旧的 definitions 键（mobile/lib/api/site.dart:149 只读它）。
func TestDefinitionsHandlerCarriesDiagnostics(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dir := t.TempDir()
	bad := []byte("id: badkeysite\nname: Bad Key\nbogus_top_key:\n    foo: bar\n")
	if err := os.WriteFile(filepath.Join(dir, "badkey.yaml"), bad, 0o600); err != nil {
		t.Fatal(err)
	}
	reg := site.NewRegistry()
	if err := reg.LoadDefinitions(dir); err != nil {
		t.Fatalf("加载定义失败：%v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/sites/definitions", nil)
	h := NewSiteHandler(nil, nil, reg, nil)
	h.Definitions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("应回 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Definitions []map[string]any `json:"definitions"`
		Diagnostics []struct {
			Kind   string `json:"kind"`
			Site   string `json:"site"`
			File   string `json:"file"`
			Detail string `json:"detail"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应不是预期 JSON：%v body=%s", err, w.Body.String())
	}
	if resp.Definitions == nil {
		t.Errorf("definitions 键必须保留（老读者还在）：%s", w.Body.String())
	}
	if len(resp.Diagnostics) == 0 {
		t.Fatalf("未知键的账面必须从端点读得到，否则界面仍是看不到：%s", w.Body.String())
	}
	found := false
	for _, d := range resp.Diagnostics {
		if d.Kind == site.DiagUnknownKey {
			found = true
			if d.Detail == "" || d.File == "" {
				t.Errorf("诊断必须带可展示文案与文件名：%+v", d)
			}
		}
	}
	if !found {
		t.Errorf("应有一条 unknown_key 诊断，实际 %+v", resp.Diagnostics)
	}
}

// 独立端点必须与 /sites/definitions 里的账面同源（不是第二份实现，避免两处漂移）。
func TestDefinitionDiagnosticsHandlerMatchesRegistry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "badkey.yaml"),
		[]byte("id: badkeysite\nname: Bad Key\nbogus_top_key:\n    foo: bar\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reg := site.NewRegistry()
	if err := reg.LoadDefinitions(dir); err != nil {
		t.Fatalf("加载定义失败：%v", err)
	}
	h := NewSiteHandler(nil, nil, reg, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/sites/definition-diagnostics", nil)
	h.DefinitionDiagnostics(c)

	if w.Code != http.StatusOK {
		t.Fatalf("应回 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Diagnostics []struct {
			Kind string `json:"kind"`
			File string `json:"file"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应不是预期 JSON：%v body=%s", err, w.Body.String())
	}
	if len(resp.Diagnostics) != len(reg.Diagnostics()) {
		t.Errorf("条数必须与注册表账面一致：端点 %d vs 账面 %d", len(resp.Diagnostics), len(reg.Diagnostics()))
	}
	if len(resp.Diagnostics) == 0 {
		t.Errorf("独立端点读不到诊断（等于界面还是看不到）：%s", w.Body.String())
	}
}
