package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/filter"
	"gorm.io/gorm"
)

// setupFilterServer 构造 FilterRuleHandler 测试路由（真实 SQLite 持久层 + 过滤引擎）。
func setupFilterServer(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/filter.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	store := filter.NewStore(oper.NewSystemConfigOper(db))
	h := NewFilterRuleHandler(store, filter.NewEngine(store))
	r := gin.New()
	r.GET("/builtin", h.ListBuiltins)
	r.GET("/custom", h.ListCustom)
	r.POST("/custom", h.CreateCustom)
	r.PUT("/custom/:id", h.UpdateCustom)
	r.DELETE("/custom/:id", h.DeleteCustom)
	r.GET("/groups", h.ListGroups)
	r.POST("/groups", h.CreateGroup)
	r.PUT("/groups/:name", h.UpdateGroup)
	r.DELETE("/groups/:name", h.DeleteGroup)
	r.GET("/bindings", h.ListBindings)
	r.PUT("/bindings", h.SetBindings)
	r.POST("/test", h.Test)
	return r
}

func doFilter(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req, _ = http.NewRequest(method, path, nil)
	} else {
		req, _ = http.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestFilterRule_BuiltinList 内置规则清单非空（开箱可用）。
func TestFilterRule_BuiltinList(t *testing.T) {
	r := setupFilterServer(t)
	rec := doFilter(t, r, http.MethodGet, "/builtin", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "rules") || rec.Body.String() == `{"rules":[]}` {
		t.Fatalf("内置规则应非空: %s", rec.Body.String())
	}
}

// TestFilterRule_CustomCRUD 自定义规则增改删全链路。
func TestFilterRule_CustomCRUD(t *testing.T) {
	r := setupFilterServer(t)

	// 空 starter：[]（不返 null）
	rec := doFilter(t, r, http.MethodGet, "/custom", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"rules":[]`) {
		t.Fatalf("空库应返回空数组: %s", rec.Body.String())
	}

	// 新建
	rec = doFilter(t, r, http.MethodPost, "/custom",
		`{"id":"myrule","name":"1080p","include":"1080","exclude":"cam","size_range":">500"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("创建失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 列表可见
	rec = doFilter(t, r, http.MethodGet, "/custom", "")
	if !strings.Contains(rec.Body.String(), "myrule") {
		t.Fatalf("创建后应可列出: %s", rec.Body.String())
	}
	// 更新
	rec = doFilter(t, r, http.MethodPut, "/custom/myrule", `{"id":"myrule","name":"720p+"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("更新失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 重复创建同 id → 400（校验冲突）
	rec = doFilter(t, r, http.MethodPost, "/custom", `{"id":"myrule","name":"dup"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("重复 id 应 400，got %d", rec.Code)
	}
	// 非法输入（缺 id）→ 400
	rec = doFilter(t, r, http.MethodPost, "/custom", `{"name":"no-id"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("缺 id 应 400，got %d", rec.Code)
	}
	// 删除
	rec = doFilter(t, r, http.MethodDelete, "/custom/myrule", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "myrule") {
		t.Fatalf("删除失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 再删 → 400
	rec = doFilter(t, r, http.MethodDelete, "/custom/myrule", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("删除不存在应 400，got %d", rec.Code)
	}
}

// TestFilterRule_GroupCRUDAndBindings 规则组增改删 + 全局绑定读写。
func TestFilterRule_GroupCRUDAndBindings(t *testing.T) {
	r := setupFilterServer(t)

	// 先建规则再建组（组引用规则 id）
	doFilter(t, r, http.MethodPost, "/custom", `{"id":"gr1","name":"R1"}`)
	doFilter(t, r, http.MethodPost, "/custom", `{"id":"gr2","name":"R2"}`)

	rec := doFilter(t, r, http.MethodPost, "/groups", `{"name":"我的组","rule_string":"gr1 & gr2"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("建组失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 列表可见
	rec = doFilter(t, r, http.MethodGet, "/groups", "")
	if !strings.Contains(rec.Body.String(), "我的组") {
		t.Fatalf("组应可列出: %s", rec.Body.String())
	}
	// 更新
	rec = doFilter(t, r, http.MethodPut, "/groups/我的组", `{"name":"改名组","rule_string":"gr1"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("改组失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	// 引用不存在规则 → 400（悬空校验）
	rec = doFilter(t, r, http.MethodPost, "/groups", `{"name":"坏组","rule_string":"nope_rule"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("悬空引用应 400，got %d", rec.Code)
	}

	// 绑定读写
	rec = doFilter(t, r, http.MethodPut, "/bindings", `{"search":["改名组"],"subscribe":[],"best_version":[]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("写绑定失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = doFilter(t, r, http.MethodGet, "/bindings", "")
	if !strings.Contains(rec.Body.String(), "改名组") {
		t.Fatalf("绑定应可读回: %s", rec.Body.String())
	}

	// 删组（级联清理绑定悬空引用）
	rec = doFilter(t, r, http.MethodDelete, "/groups/改名组", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("删组失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestFilterRule_TestEvaluate 规则试算：include 命中 + exclude 剔除。
func TestFilterRule_TestEvaluate(t *testing.T) {
	r := setupFilterServer(t)
	doFilter(t, r, http.MethodPost, "/custom", `{"id":"tr1","name":"R","include":"1080"}`)

	// 直接对 rule_string 求值（不依赖组）：表达式 = 规则 id
	body := `{"rule_string":"tr1","torrents":[{"title":"Movie 1080p BluRay","size":"8000"},{"title":"Movie 720p","size":"2000"}]}`
	rec := doFilter(t, r, http.MethodPost, "/test", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("试算失败 code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "matched") {
		t.Fatalf("响应应含 matched: %s", rec.Body.String())
	}

	// 非法表达式 → 400
	rec = doFilter(t, r, http.MethodPost, "/test", `{"rule_string":"&bad("}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法表达式应 400，got %d", rec.Code)
	}
	// 缺 rule_string → 400
	rec = doFilter(t, r, http.MethodPost, "/test", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("缺 rule_string 应 400，got %d", rec.Code)
	}
}
