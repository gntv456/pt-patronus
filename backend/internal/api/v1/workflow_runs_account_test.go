package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/workflow"
	"gorm.io/gorm"
)

// setupRunList 接真实 sqlite 的 ListRuns（默认 20、封顶 500 都在真库里跑）。
func setupRunList(t *testing.T, perWorkflow map[uint]int) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/runlist.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.Workflow{}, &models.WorkflowRun{}); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	seq := 0
	for wfID, n := range perWorkflow {
		for i := 0; i < n; i++ {
			seq++
			if err := db.Create(&models.WorkflowRun{
				WorkflowID: wfID, State: "success", Trigger: "manual",
				StartedAt: base.Add(time.Duration(seq) * time.Minute),
				Result:    fmt.Sprintf("run-%d", seq),
			}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	op := oper.NewWorkflowOper(db)
	ch := chain.NewWorkflowChain(chain.Base{}, op, oper.NewWorkflowRunOper(db),
		workflow.NewRegistry(), nil, event.New(), nil)
	h := NewWorkflowHandler(ch)
	r := gin.New()
	r.GET("/workflow-runs", h.ListRuns)
	return r, db
}

func callRuns(t *testing.T, r *gin.Engine, query string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/workflow-runs"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestWorkflowRunsAccount 运行历史取数账面：默认只给 20 条，
// 报不出 total/truncated 时，编辑器的「运行历史」面板就把 20 条画成「就跑过这些」，
// 失败被更早的运行淹掉时用户也无从知道还有多少条可翻。
func TestWorkflowRunsAccount(t *testing.T) {
	r, _ := setupRunList(t, map[uint]int{7: 25})

	out := callRuns(t, r, "?workflow_id=7&limit=20")
	if runs, _ := out["runs"].([]any); len(runs) != 20 {
		t.Errorf("清单应 20 条，got %v", len(runs))
	}
	if out["total"] != float64(25) {
		t.Errorf("total 应为 25，got %v", out["total"])
	}
	if out["truncated"] != true {
		t.Errorf("只回 20 条时必须报截断，got %v", out["truncated"])
	}
	if out["limit"] != float64(20) {
		t.Errorf("limit 应回显 20，got %v", out["limit"])
	}

	// 留空走默认 20：同样要报截断（默认值不上账就是「最容易被当成全部」的那一档）。
	out = callRuns(t, r, "?workflow_id=7")
	if out["limit"] != float64(oper.RunListDefault) {
		t.Errorf("留空应回显默认 %d，got %v", oper.RunListDefault, out["limit"])
	}
	if out["truncated"] != true {
		t.Errorf("默认 20 条装不下 25 条，应报截断，got %v", out["truncated"])
	}

	// 超上限请求：取上限而不是退回默认，且一次拿全时不许报截断。
	out = callRuns(t, r, "?workflow_id=7&limit=999999")
	if out["limit"] != float64(oper.RunListLimit) {
		t.Errorf("超上限应回显生效上限 %d，got %v", oper.RunListLimit, out["limit"])
	}
	if out["truncated"] != false {
		t.Errorf("25 条一次拿全不该报截断，got %v", out["truncated"])
	}

	// 真库层：Count 必须与 List 同按 workflow_id 过滤，否则 A 工作流的历史
	// 会被 B 工作流的运行数顶成「还有更多」。
	r2, _ := setupRunList(t, map[uint]int{7: 25, 8: 3})
	out = callRuns(t, r2, "?workflow_id=8&limit=20")
	if out["total"] != float64(3) {
		t.Errorf("工作流 8 的总数应为 3（不是 28），got %v", out["total"])
	}
	if out["truncated"] != false {
		t.Errorf("3 条一次拿全不该报截断，got %v", out["truncated"])
	}

	out = callRuns(t, r2, "?workflow_id=99&limit=20")
	if out["total"] != float64(0) {
		t.Errorf("没跑过的工作流 total 应为 0，got %v", out["total"])
	}
	if out["truncated"] != false {
		t.Errorf("空历史不是截断，truncated 应为假，got %v", out["truncated"])
	}
}
