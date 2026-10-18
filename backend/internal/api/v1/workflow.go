package v1

import (
	"encoding/json"
	"errors"

	"github.com/hxpt/ptpatronus/internal/api/errcode"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// WorkflowHandler 处理工作流 CRUD、执行、触发器与运行历史。
type WorkflowHandler struct {
	ch *chain.WorkflowChain
}

func NewWorkflowHandler(ch *chain.WorkflowChain) *WorkflowHandler {
	return &WorkflowHandler{ch: ch}
}

// wfReq 创建/编辑工作流的请求体。Nodes/Edges/EventConditions 为前端数组，存库时序列化为 text。
type wfReq struct {
	Name            string          `json:"name" binding:"max=128"`
	Description     string          `json:"description" binding:"max=2000"`
	TriggerType     string          `json:"trigger_type" binding:"max=16"`
	Timer           string          `json:"timer" binding:"max=64"`
	EventType       string          `json:"event_type" binding:"max=64"`
	EventConditions json.RawMessage `json:"event_conditions"`
	Enabled         bool            `json:"enabled"`
	Nodes           json.RawMessage `json:"nodes"`
	Edges           json.RawMessage `json:"edges"`
}

// workflowResponse 把 text 列还原为数组返回前端（节点/连线/条件/执行日志）。
func workflowResponse(wf *models.Workflow) gin.H {
	nodes, edges, conds := []any{}, []any{}, []any{}
	if wf.Nodes != "" {
		_ = json.Unmarshal([]byte(wf.Nodes), &nodes)
	}
	if wf.Edges != "" {
		_ = json.Unmarshal([]byte(wf.Edges), &edges)
	}
	if wf.EventConditions != "" {
		_ = json.Unmarshal([]byte(wf.EventConditions), &conds)
	}
	return gin.H{
		"id": wf.ID, "name": wf.Name, "description": wf.Description,
		"trigger_type": wf.TriggerType, "timer": wf.Timer, "event_type": wf.EventType,
		"event_conditions": conds, "enabled": wf.Enabled, "state": wf.State,
		"run_count": wf.RunCount, "last_run_at": wf.LastRunAt, "last_result": wf.LastResult,
		"nodes": nodes, "edges": edges, "created_at": wf.CreatedAt, "updated_at": wf.UpdatedAt,
	}
}

func runResponse(r *models.WorkflowRun) gin.H {
	execs := []any{}
	if r.NodeExecutions != "" {
		_ = json.Unmarshal([]byte(r.NodeExecutions), &execs)
	}
	return gin.H{
		"id": r.ID, "workflow_id": r.WorkflowID, "state": r.State, "trigger": r.Trigger,
		"started_at": r.StartedAt, "finished_at": r.FinishedAt, "duration_ms": r.DurationMS,
		"node_executions": execs, "result": r.Result, "created_at": r.CreatedAt,
	}
}

// Catalog 节点目录（GET /workflows/actions）。
func (h *WorkflowHandler) Catalog(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"actions": h.ch.Catalog()})
}

// Templates 内置工作流模板（GET /workflows/templates）。
func (h *WorkflowHandler) Templates(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"templates": h.ch.Templates()})
}

// Stats 运行统计看板（GET /workflows/stats）。
func (h *WorkflowHandler) Stats(c *gin.Context) {
	st, err := h.ch.Stats()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	// RecentFailures 走 runResponse：把 NodeExecutions(text) 反序列化为数组，
	// 与 Run/ListRuns/RunDetail 保持一致——否则前端拿到 JSON 字符串，迭代 .map 必崩。
	failures := make([]gin.H, 0, len(st.RecentFailures))
	for i := range st.RecentFailures {
		failures = append(failures, runResponse(&st.RecentFailures[i]))
	}
	c.JSON(http.StatusOK, gin.H{
		"total": st.Total, "enabled": st.Enabled, "runs_total": st.RunsTotal,
		"runs_success": st.RunsSuccess, "runs_failed": st.RunsFailed,
		"avg_duration_ms": st.AvgDurationMS, "node_timing": st.NodeTiming,
		"recent_failures": failures,
	})
}

// List 全部工作流。
func (h *WorkflowHandler) List(c *gin.Context) {
	wfs, err := h.ch.List()
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	out := make([]gin.H, 0, len(wfs))
	for i := range wfs {
		out = append(out, workflowResponse(&wfs[i]))
	}
	c.JSON(http.StatusOK, gin.H{"workflows": out})
}

// Get 单条。
func (h *WorkflowHandler) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	wf, err := h.ch.Get(id)
	if err != nil {
		// #343 走统一判据：手抄的 Respond(WorkflowNotFound) 只判「不存在」，
		// 不判 DB 真故障（WorkflowOper.Get 底层是 db.First）⇒ 磁盘满/连接断时
		// 也会被说成「工作流不存在」。同文件 Delete/Enable 已经走 respondRecordError。
		respondRecordError(c, err, errcode.WorkflowNotFound, errcode.InvalidInput, errcode.Internal)
		return
	}
	c.JSON(http.StatusOK, workflowResponse(wf))
}

// respondWorkflowErr 节点图校验不合法时把原因带回响应（原因是我们自己写的中文，
// 例如「工作流节点图不合法：未知节点类型：xxx」），编辑图的人才知道哪儿错了；
// 其它情况仍用泛化的「参数有误」（Go 运行时类型错误不该原样弹给用户）。
func respondWorkflowErr(c *gin.Context, err error) {
	if errors.Is(err, chain.ErrInvalidGraph) {
		errcode.Respond(c, errcode.InvalidInput.WithDetail(err))
		return
	}
	errcode.Respond(c, errcode.InvalidInput.WithErr(err))
}

// Create 新建。
func (h *WorkflowHandler) Create(c *gin.Context) {
	var in wfReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if in.Name == "" {
		errcode.Respond(c, errcode.NameRequired)
		return
	}
	wf := &models.Workflow{
		Name: in.Name, Description: in.Description, TriggerType: in.TriggerType,
		Timer: in.Timer, EventType: in.EventType, Enabled: in.Enabled,
		Nodes: string(in.Nodes), Edges: string(in.Edges), EventConditions: string(in.EventConditions),
	}
	out, err := h.ch.Create(wf)
	if err != nil {
		respondWorkflowErr(c, err)
		return
	}
	c.JSON(http.StatusOK, workflowResponse(out))
}

// Update 编辑。
func (h *WorkflowHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	wf, err := h.ch.Get(id)
	if err != nil {
		// #343 走统一判据：手抄的 Respond(WorkflowNotFound) 只判「不存在」，
		// 不判 DB 真故障（WorkflowOper.Get 底层是 db.First）⇒ 磁盘满/连接断时
		// 也会被说成「工作流不存在」。同文件 Delete/Enable 已经走 respondRecordError。
		respondRecordError(c, err, errcode.WorkflowNotFound, errcode.InvalidInput, errcode.Internal)
		return
	}
	var in wfReq
	if err := c.ShouldBindJSON(&in); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	if in.Name != "" {
		wf.Name = in.Name
	}
	wf.Description = in.Description
	wf.TriggerType = in.TriggerType
	wf.Timer = in.Timer
	wf.EventType = in.EventType
	wf.Enabled = in.Enabled
	if len(in.Nodes) > 0 {
		wf.Nodes = string(in.Nodes)
	}
	if len(in.Edges) > 0 {
		wf.Edges = string(in.Edges)
	}
	if len(in.EventConditions) > 0 {
		wf.EventConditions = string(in.EventConditions)
	}
	if err := h.ch.Update(wf); err != nil {
		respondWorkflowErr(c, err)
		return
	}
	c.JSON(http.StatusOK, workflowResponse(wf))
}

// Delete 删除。
func (h *WorkflowHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.ch.Delete(id); err != nil {
		// 删一条已经不存在的工作流：从前一律 500「服务内部错误」，
		// 而同一族的其它 12 个域都回 200——两边都不对，先按「记录不存在」说清。
		respondRecordError(c, err, errcode.WorkflowNotFound, errcode.InvalidInput, errcode.Internal)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// Run 手动执行。
func (h *WorkflowHandler) Run(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	run, err := h.ch.Run(c.Request.Context(), id, "manual", nil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": userErrText(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "run": runResponse(run)})
}

// Enable 启用。
func (h *WorkflowHandler) Enable(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.ch.SetEnabled(id, true); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": true})
}

// Disable 禁用。
func (h *WorkflowHandler) Disable(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.ch.SetEnabled(id, false); err != nil {
		errcode.Respond(c, errcode.InvalidInput.WithErr(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": false})
}

// Reset 重置状态。
func (h *WorkflowHandler) Reset(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.ch.Reset(id); err != nil {
		respondRecordError(c, err, errcode.WorkflowNotFound, errcode.InvalidInput, errcode.Internal)
		return
	}
	c.JSON(http.StatusOK, gin.H{"reset": id})
}

// ListRuns 运行历史（GET /workflow-runs?workflow_id=&limit=）。
func (h *WorkflowHandler) ListRuns(c *gin.Context) {
	wfID, _ := strconv.ParseUint(c.Query("workflow_id"), 10, 64)
	reqLimit, _ := strconv.Atoi(c.Query("limit"))
	// 默认只有 20 条：不回显生效 limit、也不给总数，运行历史弹窗就把 20 条画成「就跑过这些」。
	limit := oper.NormalizeRunLimit(reqLimit)
	runs, err := h.ch.ListRuns(uint(wfID), limit)
	if err != nil {
		errcode.Respond(c, errcode.Internal.WithErr(err))
		return
	}
	out := make([]gin.H, 0, len(runs))
	for i := range runs {
		out = append(out, runResponse(&runs[i]))
	}
	// total/truncated 与清单同批给出（口径同 RSS / 云任务队列 / 收件箱 / 设备快照）；
	// 计数读不出时保守报「可能被截断」，并省略 total，不编一个 0。
	total, cntErr := h.ch.CountRuns(uint(wfID))
	truncated := cntErr != nil || total > int64(len(out))
	if cntErr != nil {
		slog.Warn("workflow run count failed", "workflow_id", wfID, "err", cntErr)
	}
	resp := gin.H{"runs": out, "limit": limit, "truncated": truncated}
	if cntErr == nil {
		resp["total"] = total
	}
	c.JSON(http.StatusOK, resp)
}

// RunDetail 单次运行详情（GET /workflow-runs/:rid）。
func (h *WorkflowHandler) RunDetail(c *gin.Context) {
	rid, err := strconv.ParseUint(c.Param("rid"), 10, 64)
	if err != nil {
		errcode.Respond(c, errcode.InvalidID)
		return
	}
	r, err := h.ch.GetRun(uint(rid))
	if err != nil {
		// #343 不再用通用 ResourceNotFound（「资源不存在」连对象都没说，用户不知道
		// 缺的是哪次运行）。改走本域专属码 + 统一判据，DB 真故障也归 500 而非 404。
		respondRecordError(c, err, errcode.WorkflowNotFound, errcode.InvalidInput, errcode.Internal)
		return
	}
	c.JSON(http.StatusOK, runResponse(r))
}
