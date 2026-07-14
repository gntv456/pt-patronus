package v1

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/scheduler"
	"github.com/hxpt/ptpatronus/internal/workflow"
	"gorm.io/gorm"
)

// hEchoNode handler e2e 测试用节点：追加固定消息。
type hEchoNode struct{}

func (hEchoNode) Type() string                       { return "h_echo" }
func (hEchoNode) Name() string                       { return "回显" }
func (hEchoNode) Category() string                   { return "测试" }
func (hEchoNode) Description() string                { return "test" }
func (hEchoNode) Icon() string                       { return "test" }
func (hEchoNode) DefaultParams() map[string]any      { return nil }
func (hEchoNode) ParamSchema() []workflow.ParamField { return nil }
func (hEchoNode) Execute(_ context.Context, _ map[string]any, ecx *workflow.Context) error {
	ecx.Messages = append(ecx.Messages, "hi")
	return nil
}

// setupWfHandler 构造接真实 sqlite 的 WorkflowHandler + gin engine，仿 router.go 注册全部工作流路由。
func setupWfHandler(t *testing.T) (*gin.Engine, *oper.WorkflowOper) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/wfh.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&models.Workflow{}, &models.WorkflowRun{}); err != nil {
		t.Fatal(err)
	}
	op := oper.NewWorkflowOper(db)
	runOp := oper.NewWorkflowRunOper(db)
	reg := workflow.NewRegistry()
	reg.Register(hEchoNode{})
	ch := chain.NewWorkflowChain(chain.Base{}, op, runOp, reg, scheduler.New(), event.New(), nil)
	h := NewWorkflowHandler(ch)

	r := gin.New()
	wfs := r.Group("/workflows")
	wfs.GET("", h.List)
	wfs.POST("", h.Create)
	wfs.GET("/actions", h.Catalog)
	wfs.GET("/templates", h.Templates)
	wfs.GET("/stats", h.Stats)
	wfs.GET("/:id", h.Get)
	wfs.PUT("/:id", h.Update)
	wfs.DELETE("/:id", h.Delete)
	wfs.POST("/:id/run", h.Run)
	wfs.POST("/:id/enable", h.Enable)
	wfs.POST("/:id/disable", h.Disable)
	wfs.POST("/:id/reset", h.Reset)
	wfRuns := r.Group("/workflow-runs")
	wfRuns.GET("", h.ListRuns)
	wfRuns.GET("/:rid", h.RunDetail)
	return r, op
}

// TestWorkflowHandlerEarlyReturns nil chain 下触达 chain 前的守卫：bad id / 缺 name → 400。
func TestWorkflowHandlerEarlyReturns(t *testing.T) {
	h := NewWorkflowHandler(nil)
	r := gin.New()
	r.GET("/workflows/:id", h.Get)
	r.PUT("/workflows/:id", h.Update)
	r.DELETE("/workflows/:id", h.Delete)
	r.POST("/workflows/:id/run", h.Run)
	r.POST("/workflows/:id/enable", h.Enable)
	r.POST("/workflows/:id/disable", h.Disable)
	r.POST("/workflows/:id/reset", h.Reset)
	r.GET("/workflow-runs/:rid", h.RunDetail)
	r.POST("/workflows", h.Create)

	cases := []struct{ m, p, b string }{
		{http.MethodGet, "/workflows/abc", ""},
		{http.MethodPut, "/workflows/abc", `{"name":"x"}`},
		{http.MethodDelete, "/workflows/abc", ""},
		{http.MethodPost, "/workflows/abc/run", ""},
		{http.MethodPost, "/workflows/abc/enable", ""},
		{http.MethodPost, "/workflows/abc/disable", ""},
		{http.MethodPost, "/workflows/abc/reset", ""},
		{http.MethodGet, "/workflow-runs/abc", ""},
	}
	for _, c := range cases {
		rec := doReq(t, r, c.m, c.p, c.b)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s: code=%d want 400", c.m, c.p, rec.Code)
		}
	}
	// Create 缺 name → 400（handler 层 name 必填守卫，触达 chain 前）。
	rec := doReq(t, r, http.MethodPost, "/workflows", `{"name":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("Create 空 name: code=%d want 400", rec.Code)
	}
}

// TestWorkflowHandlerE2E 端到端：Create→List→Get→Actions→Templates→Run→ListRuns→Delete。
func TestWorkflowHandlerE2E(t *testing.T) {
	r, op := setupWfHandler(t)

	// Create
	body := `{"name":"e2e","trigger_type":"manual","nodes":[{"id":"n1","type":"h_echo","name":"打招呼"}],"edges":[]}`
	rec := doReq(t, r, http.MethodPost, "/workflows", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("Create code=%d body=%s", rec.Code, rec.Body.String())
	}
	m := bodyJSON(t, rec)
	id := int64(m["id"].(float64))
	if id == 0 {
		t.Fatal("Create 返回 id 应非零")
	}
	idPath := "/workflows/" + strconv.FormatInt(id, 10)

	// List 含 1 条
	rec = doReq(t, r, http.MethodGet, "/workflows", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("List code=%d", rec.Code)
	}
	if wfs := bodyJSON(t, rec)["workflows"].([]any); len(wfs) != 1 {
		t.Fatalf("List 应 1 条，got %d", len(wfs))
	}

	// Get
	if rec := doReq(t, r, http.MethodGet, idPath, ""); rec.Code != http.StatusOK {
		t.Fatalf("Get code=%d", rec.Code)
	}

	// Actions（Catalog）含 h_echo
	rec = doReq(t, r, http.MethodGet, "/workflows/actions", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("Actions code=%d", rec.Code)
	}
	if acts := bodyJSON(t, rec)["actions"].([]any); len(acts) == 0 {
		t.Fatal("Catalog 不应为空")
	}

	// Templates
	if rec := doReq(t, r, http.MethodGet, "/workflows/templates", ""); rec.Code != http.StatusOK {
		t.Fatalf("Templates code=%d", rec.Code)
	}

	// Stats（空库也应有合法响应）
	if rec := doReq(t, r, http.MethodGet, "/workflows/stats", ""); rec.Code != http.StatusOK {
		t.Fatalf("Stats code=%d", rec.Code)
	}

	// Run → ok:true，run_count 累加
	rec = doReq(t, r, http.MethodPost, idPath+"/run", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("Run code=%d body=%s", rec.Code, rec.Body.String())
	}
	if rm := bodyJSON(t, rec); rm["ok"] != true {
		t.Fatalf("Run 应 ok:true，got %v", rm["ok"])
	}
	got, _ := op.Get(uint(id))
	if got.RunCount != 1 {
		t.Fatalf("RunCount=%d want 1", got.RunCount)
	}

	// ListRuns
	rec = doReq(t, r, http.MethodGet, "/workflow-runs?workflow_id="+strconv.FormatInt(id, 10), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("ListRuns code=%d", rec.Code)
	}
	if runs := bodyJSON(t, rec)["runs"].([]any); len(runs) != 1 {
		t.Fatalf("ListRuns 应 1 条，got %d", len(runs))
	}

	// Delete
	rec = doReq(t, r, http.MethodDelete, idPath, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("Delete code=%d", rec.Code)
	}
	list, _ := op.List()
	if len(list) != 0 {
		t.Fatalf("Delete 后应空，got %d", len(list))
	}
}
