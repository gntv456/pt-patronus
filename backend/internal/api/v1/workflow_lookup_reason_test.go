package v1

// #343 工作流「按 id 取」失败的三处出口手抄归类，其中一处还用通用码（#342 的同族）。
//
// workflow.go 里同一个形状写出了四个版本：
//
//	:126 Get        → Respond(WorkflowNotFound)        手抄
//	:175 Update     → Respond(WorkflowNotFound)        手抄
//	:216 Delete     → respondRecordError(WorkflowNotFound, …)  ← 统一判据
//	:269 Enable/…   → respondRecordError(WorkflowNotFound, …)  ← 统一判据
//	:313 RunDetail  → Respond(ResourceNotFound)        手抄 + 通用码
//
// 两处问题：
//  1. 手抄的三处只判「不存在」，不判 DB 真故障：WorkflowOper.Get / WorkflowRunOper.Get
//     底层都是 db.First，磁盘满/连接断时也被说成「工作流不存在」/「资源不存在」。
//  2. RunDetail 用的是全仓通用 ResourceNotFound（「资源不存在」）——同域的
//     WorkflowNotFound（「工作流不存在」）就在同文件上方，说清了对象，RunDetail 没用。
//
// 修法：这三处改走既有的 respondRecordError（它已经处理了 404/400/500 三段），
// 不新造判据——这正是本仓反复出现的「同族问题每处自己抄一份」。

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

// TestWorkflowLookupOutletsShareClassifier 三处出口必须走统一判据。
func TestWorkflowLookupOutletsShareClassifier(t *testing.T) {
	src, err := os.ReadFile("workflow.go")
	if err != nil {
		t.Fatalf("读不到 workflow.go：%v", err)
	}
	code := stripCommentsForTest(string(src))

	// 1. 不许再手抄裸 Respond(WorkflowNotFound) —— 只判不存在、不判 DB 故障。
	if n := strings.Count(code, "errcode.WorkflowNotFound)"); n != 0 {
		t.Errorf("还有 %d 处裸 Respond(WorkflowNotFound)：只判「不存在」，DB 真故障会被说成工作流没了", n)
	}
	// 2. RunDetail 不许用通用 ResourceNotFound（它连对象都没说）。
	if strings.Contains(code, "errcode.ResourceNotFound") {
		t.Error("RunDetail 不许用通用 ResourceNotFound（旧形状：只说「资源不存在」，用户不知道是运行记录）")
	}
	// 3. 五个按 id 取的出口都该走 respondRecordError（统一判据）。
	if n := strings.Count(code, "respondRecordError(c, err,"); n < 5 {
		t.Errorf("走统一判据的出口 = %d, want ≥5（Get/Update/Delete/Enable/Run/RunDetail）", n)
	}
}

// TestWorkflowLookupKeeps404ForMissing 反向对照：真·不存在仍要 404 + workflow_not_found。
func TestWorkflowLookupKeeps404ForMissing(t *testing.T) {
	r, _ := setupWfHandler(t)

	rec := doReq(t, r, http.MethodGet, "/workflows/999999", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("工作流不存在应 404，got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "workflow_not_found") {
		t.Errorf("应回本域专属码 got %s", rec.Body.String())
	}

	rec = doReq(t, r, http.MethodGet, "/workflow-runs/999999", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("运行记录不存在应 404，got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "workflow_not_found") {
		t.Errorf("运行记录也应回本域专属码（不该再是 resource_not_found）got %s", rec.Body.String())
	}
}
