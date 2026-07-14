package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/workflow"
	"github.com/robfig/cron/v3"
)

// runTimeout 单次工作流执行整体超时（兜底，防节点卡死）。
const runTimeout = 10 * time.Minute

// cronScheduler 抽象 scheduler.Scheduler 的工作流所需子集，便于测试注入故障实现
// （验证 Create 在 RegisterTriggers 失败时回滚）。*scheduler.Scheduler 天然实现此接口。
type cronScheduler interface {
	AddFunc(name, spec string, fn func()) error
	Remove(name string)
}

// WorkflowChain 编排工作流：CRUD + 执行 + 触发器注册（timer cron / event 总线）。
type WorkflowChain struct {
	Base
	op     *oper.WorkflowOper
	runOp  *oper.WorkflowRunOper
	reg    *workflow.Registry
	exec   *workflow.Executor
	sched  cronScheduler
	bus    *event.Bus
	notice *notice.Service

	mu              sync.Mutex     // 重入守卫 + 事件订阅登记
	eventSubscribed map[string]int // 已注册事件监听 key="wfID:eventType" → bus.Subscribe 返回的 handle（供 UnregisterTriggers 精确退订）
}

// NewWorkflowChain 构造。
func NewWorkflowChain(b Base, op *oper.WorkflowOper, runOp *oper.WorkflowRunOper,
	reg *workflow.Registry, sched cronScheduler, bus *event.Bus, ns *notice.Service) *WorkflowChain {
	return &WorkflowChain{
		Base: b, op: op, runOp: runOp, reg: reg, exec: workflow.NewExecutor(reg),
		sched: sched, bus: bus, notice: ns, eventSubscribed: make(map[string]int),
	}
}

// Catalog 返回节点目录（/workflows/actions）。
func (c *WorkflowChain) Catalog() []workflow.NodeInfo { return c.reg.List() }

// Templates 返回内置工作流模板（/workflows/templates）。
func (c *WorkflowChain) Templates() []workflow.Template { return workflow.Templates() }

// WorkflowStats 运行统计看板聚合（跨工作流）。
type WorkflowStats struct {
	Total          int                  `json:"total"`
	Enabled        int                  `json:"enabled"`
	RunsTotal      int                  `json:"runs_total"`
	RunsSuccess    int                  `json:"runs_success"`
	RunsFailed     int                  `json:"runs_failed"`
	AvgDurationMS  int64                `json:"avg_duration_ms"`
	RecentFailures []models.WorkflowRun `json:"recent_failures"`
	NodeTiming     []NodeTiming         `json:"node_timing"`
}

// NodeTiming 节点耗时排行（按 node_type 聚合）。
type NodeTiming struct {
	NodeType string `json:"node_type"`
	NodeName string `json:"node_name"`
	Count    int    `json:"count"`
	TotalMS  int64  `json:"total_ms"`
	AvgMS    int64  `json:"avg_ms"`
	Failed   int    `json:"failed"`
}

// Stats 计算运行统计：取最近 500 次运行，聚合成功/失败/平均耗时 + per-node 耗时 + 最近失败。
func (c *WorkflowChain) Stats() (*WorkflowStats, error) {
	wfs, err := c.op.List()
	if err != nil {
		return nil, err
	}
	enabled := 0
	for _, w := range wfs {
		if w.Enabled {
			enabled++
		}
	}
	runs, err := c.runOp.ListRecent(500)
	if err != nil {
		return nil, err
	}
	st := &WorkflowStats{Total: len(wfs), Enabled: enabled, RunsTotal: len(runs), NodeTiming: []NodeTiming{}, RecentFailures: []models.WorkflowRun{}}
	var totalDur int64
	timing := map[string]*NodeTiming{} // key node_type
	failures := []models.WorkflowRun{}
	for _, r := range runs {
		totalDur += r.DurationMS
		if r.State == "success" {
			st.RunsSuccess++
		} else if r.State == "failed" {
			st.RunsFailed++
			if len(failures) < 10 {
				failures = append(failures, r)
			}
		}
		var entries []workflow.LogEntry
		if r.NodeExecutions != "" {
			_ = json.Unmarshal([]byte(r.NodeExecutions), &entries)
		}
		for _, le := range entries {
			k := le.NodeType
			t, ok := timing[k]
			if !ok {
				t = &NodeTiming{NodeType: le.NodeType, NodeName: le.NodeName}
				timing[k] = t
			}
			t.Count++
			t.TotalMS += le.DurationMS
			if le.Status == "failed" {
				t.Failed++
			}
			if le.NodeName != "" {
				t.NodeName = le.NodeName
			}
		}
	}
	if st.RunsTotal > 0 {
		st.AvgDurationMS = totalDur / int64(st.RunsTotal)
	}
	for _, t := range timing {
		if t.Count > 0 {
			t.AvgMS = t.TotalMS / int64(t.Count)
		}
		st.NodeTiming = append(st.NodeTiming, *t)
	}
	// 按总耗时倒序
	sort.Slice(st.NodeTiming, func(i, j int) bool { return st.NodeTiming[i].TotalMS > st.NodeTiming[j].TotalMS })
	st.RecentFailures = failures
	return st, nil
}

// List 全部工作流。
func (c *WorkflowChain) List() ([]models.Workflow, error) { return c.op.List() }

// Get 单条。
func (c *WorkflowChain) Get(id uint) (*models.Workflow, error) { return c.op.Get(id) }

// Create 新建（校验图 → 落库 → 注册触发器）。
func (c *WorkflowChain) Create(wf *models.Workflow) (*models.Workflow, error) {
	if wf.TriggerType == "" {
		wf.TriggerType = "manual"
	}
	if err := c.validateGraph(wf); err != nil {
		return nil, err
	}
	if wf.Enabled {
		if err := validateWorkflowTrigger(wf); err != nil {
			return nil, err
		}
	}
	if wf.State == "" {
		wf.State = "idle"
	}
	if err := c.op.Create(wf); err != nil {
		return nil, err
	}
	if wf.Enabled {
		if err := c.RegisterTriggers(wf); err != nil {
			_ = c.op.Delete(wf.ID)
			return nil, err
		}
	}
	return wf, nil
}

// Update 编辑（注销旧触发器 → 保存 → 注册新触发器）。
func (c *WorkflowChain) Update(wf *models.Workflow) error {
	if err := c.validateGraph(wf); err != nil {
		return err
	}
	if wf.Enabled {
		if err := validateWorkflowTrigger(wf); err != nil {
			return err
		}
	}
	old, err := c.op.Get(wf.ID)
	if err != nil {
		// DB 读失败不能吞：old=nil 会跳过 UnregisterTriggers → 旧定时器/webhook 残留与新触发器并存→重复触发。中止让调用方重试。
		slog.Warn("workflow get old failed, abort update to avoid trigger leak", "id", wf.ID, "err", err)
		return err
	}
	if old != nil {
		c.UnregisterTriggers(old)
	}
	if err := c.op.Update(wf); err != nil {
		return err
	}
	if wf.Enabled {
		if err := c.RegisterTriggers(wf); err != nil {
			_ = c.op.Update(old)
			if old.Enabled {
				_ = c.RegisterTriggers(old)
			}
			return err
		}
	}
	return nil
}

// Delete 删除（注销触发器 → 删库）。
func (c *WorkflowChain) Delete(id uint) error {
	old, err := c.op.Get(id)
	if err != nil {
		// 同 Update：DB 读失败须中止，避免 ghost 触发器指向已删工作流（下次触发 nil 指针/panic）。
		slog.Warn("workflow get old failed, abort delete to avoid trigger leak", "id", id, "err", err)
		return err
	}
	if old != nil {
		c.UnregisterTriggers(old)
	}
	return c.op.Delete(id)
}

// SetEnabled 启用/禁用（重注册触发器）。
func (c *WorkflowChain) SetEnabled(id uint, enabled bool) error {
	wf, err := c.op.Get(id)
	if err != nil {
		return err
	}
	wf.Enabled = enabled
	if enabled {
		if err := validateWorkflowTrigger(wf); err != nil {
			return err
		}
	}
	if err := c.op.Update(wf); err != nil {
		return err
	}
	if enabled {
		if err := c.RegisterTriggers(wf); err != nil {
			wf.Enabled = false
			_ = c.op.Update(wf)
			return err
		}
		return nil
	}
	c.UnregisterTriggers(wf)
	return nil
}

// Reset 重置状态与运行数。
func (c *WorkflowChain) Reset(id uint) error {
	return c.op.ResetState(id)
}

// Run 执行工作流。trigger ∈ manual|timer|event；eventData 为事件触发时的负载（种子进 Variables）。
// 重入守卫：State==running 时拒绝。执行期间不持锁。
func (c *WorkflowChain) Run(ctx context.Context, id uint, trigger string, eventData map[string]any) (*models.WorkflowRun, error) {
	// 重入守卫：检查并置 running（临界区内完成，不持有锁执行）。
	c.mu.Lock()
	wf, err := c.op.Get(id)
	if err != nil || wf == nil {
		c.mu.Unlock()
		return nil, fmt.Errorf("工作流不存在")
	}
	if wf.State == "running" {
		c.mu.Unlock()
		return nil, fmt.Errorf("工作流正在执行中，请稍后")
	}
	if err := c.op.SetState(id, "running"); err != nil {
		// 状态写失败不阻断执行，但必须可见——否则工作流会卡在 running 无法被重新触发。
		slog.Error("workflow mark running failed", "id", id, "err", err)
	}
	c.mu.Unlock()

	rctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	run := &models.WorkflowRun{WorkflowID: id, State: "running", Trigger: trigger, StartedAt: time.Now()}
	if err := c.runOp.Create(run); err != nil {
		// 运行记录创建失败不阻断执行，但 run.ID 缺失会让后续 Update 落到零值。
		slog.Error("workflow run record create failed", "id", id, "err", err)
	}

	// 解析图
	var gnodes []workflow.GraphNode
	var edges []workflow.GraphEdge
	_ = json.Unmarshal([]byte(wf.Nodes), &gnodes)
	_ = json.Unmarshal([]byte(wf.Edges), &edges)

	ecx := &workflow.Context{Variables: map[string]any{"trigger": trigger}}
	for k, v := range eventData {
		ecx.Variables[k] = v
	}

	logEntries, runErr := c.exec.Run(rctx, gnodes, edges, ecx)

	state := "success"
	result := strings.Join(ecx.Messages, "\n")
	if runErr != nil {
		state = "failed"
		result = runErr.Error()
	}

	run.FinishedAt = ptrTime(time.Now())
	run.DurationMS = time.Since(run.StartedAt).Milliseconds()
	run.State = state
	run.Result = result
	if blob, err := json.Marshal(ecx.Log); err == nil {
		run.NodeExecutions = string(blob)
	} else if logEntries != nil { // 兜底用返回的日志
		if b, err := json.Marshal(logEntries); err == nil {
			run.NodeExecutions = string(b)
		}
	}
	if err := c.runOp.Update(run); err != nil {
		slog.Error("workflow run record update failed", "id", id, "run_id", run.ID, "err", err)
	}
	if err := c.op.MarkRun(id, state, firstLine(result)); err != nil {
		// 终态写失败会让工作流永久停在 running，下次触发会被「正在执行中」挡住——必须可见。
		slog.Error("workflow mark final state failed", "id", id, "state", state, "err", err)
	}

	if runErr != nil && c.notice != nil {
		nc, ncancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = c.notice.Notice(nc, notice.Input{
			Source: notice.SourceWorkflow, Title: "工作流执行失败：" + wf.Name, Body: result, Level: "warning",
		}, false)
		ncancel()
	}
	slog.Info("workflow run done", "id", id, "name", wf.Name, "state", state, "duration_ms", run.DurationMS)
	return run, runErr
}

// ListRuns 运行历史。
func (c *WorkflowChain) ListRuns(workflowID uint, limit int) ([]models.WorkflowRun, error) {
	return c.runOp.ListByWorkflow(workflowID, limit)
}

// GetRun 单次运行详情。
func (c *WorkflowChain) GetRun(runID uint) (*models.WorkflowRun, error) { return c.runOp.Get(runID) }

// RegisterTriggers 按触发类型注册（timer→cron；event→总线监听）。幂等。
func (c *WorkflowChain) RegisterTriggers(wf *models.Workflow) error {
	if wf == nil || !wf.Enabled {
		return nil
	}
	if err := validateWorkflowTrigger(wf); err != nil {
		return err
	}
	switch wf.TriggerType {
	case "timer":
		if strings.TrimSpace(wf.Timer) == "" {
			return fmt.Errorf("定时工作流缺少 cron 表达式")
		}
		name := fmt.Sprintf("wf-timer-%d", wf.ID)
		wfID := wf.ID
		return c.sched.AddFunc(name, wf.Timer, func() {
			bg, cancel := context.WithTimeout(context.Background(), runTimeout)
			defer cancel()
			if _, err := c.Run(bg, wfID, "timer", nil); err != nil {
				slog.Warn("workflow timer run failed", "id", wfID, "err", err)
			}
		})
	case "event":
		if strings.TrimSpace(wf.EventType) == "" {
			return fmt.Errorf("事件工作流缺少事件类型")
		}
		c.registerEventHandler(wf)
	}
	return nil
}

func validateWorkflowTrigger(wf *models.Workflow) error {
	if wf == nil {
		return fmt.Errorf("workflow is nil")
	}
	switch strings.TrimSpace(wf.TriggerType) {
	case "manual":
		return nil
	case "timer":
		if strings.TrimSpace(wf.Timer) == "" {
			return fmt.Errorf("timer workflow requires a cron expression")
		}
		if _, err := cron.ParseStandard(wf.Timer); err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
		return nil
	case "event":
		if strings.TrimSpace(wf.EventType) == "" {
			return fmt.Errorf("event workflow requires an event type")
		}
		return validateEventConditions(wf.EventConditions)
	default:
		return fmt.Errorf("unsupported trigger type: %s", wf.TriggerType)
	}
}

func validateEventConditions(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var conds []workflow.EventCondition
	if err := json.Unmarshal([]byte(raw), &conds); err != nil {
		return fmt.Errorf("invalid event conditions JSON: %w", err)
	}
	for _, condition := range conds {
		if strings.TrimSpace(condition.Field) == "" {
			return fmt.Errorf("event condition field is required")
		}
		switch condition.Operator {
		case "eq", "ne", "contains":
		case "regex":
			if _, err := regexp.Compile(condition.Value); err != nil {
				return fmt.Errorf("invalid event condition regex: %w", err)
			}
		default:
			return fmt.Errorf("unsupported event condition operator: %s", condition.Operator)
		}
	}
	return nil
}

// UnregisterTriggers 注销触发器。timer→sched.Remove；event→bus.Unsubscribe 按 handle 精确退订。
func (c *WorkflowChain) UnregisterTriggers(wf *models.Workflow) {
	if wf == nil {
		return
	}
	if wf.TriggerType == "timer" {
		c.sched.Remove(fmt.Sprintf("wf-timer-%d", wf.ID))
	} else if wf.TriggerType == "event" {
		key := fmt.Sprintf("%d:%s", wf.ID, wf.EventType)
		c.mu.Lock()
		handle, ok := c.eventSubscribed[key]
		if ok {
			delete(c.eventSubscribed, key)
		}
		c.mu.Unlock()
		if ok {
			c.bus.Unsubscribe(wf.EventType, handle) // 真正退订，释放 handler 闭包（治本，非惰性自废）
		}
	}
}

// RegisterAllTriggers 启动时遍历启用的工作流注册触发器（cron 任务随进程重启丢失，需重建）。
func (c *WorkflowChain) RegisterAllTriggers(ctx context.Context) error {
	wfs, err := c.op.ListEnabled()
	if err != nil {
		return err
	}
	for i := range wfs {
		if err := c.RegisterTriggers(&wfs[i]); err != nil {
			slog.Warn("workflow register triggers on startup failed", "id", wfs[i].ID, "err", err)
		}
	}
	return nil
}

// registerEventHandler 注册一次事件监听（按 "wfID:eventType" 去重），并记录 bus.Subscribe 返回的 handle，
// 供 UnregisterTriggers 调 bus.Unsubscribe 精确退订。去重键含 EventType——改事件类型时新类型生成新 key
// 重新订阅；handler 仍保留 EventType 自废校验作双保险（防任何路径漏退订）。
func (c *WorkflowChain) registerEventHandler(wf *models.Workflow) {
	key := fmt.Sprintf("%d:%s", wf.ID, wf.EventType)
	c.mu.Lock()
	if _, ok := c.eventSubscribed[key]; ok {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	wfID := wf.ID
	eventType := wf.EventType
	handle := c.bus.Subscribe(eventType, 100, func(ctx context.Context, e event.Event) {
		cur, err := c.op.Get(wfID)
		if err != nil || cur == nil || !cur.Enabled || cur.TriggerType != "event" || cur.EventType != eventType {
			return // 已删除/禁用/改型/改事件类型 → 自废（双保险，正常路径已由 Unsubscribe 退订）
		}
		if !evaluateEventConditions(cur.EventConditions, e.Data) {
			return
		}
		bg, cancel := context.WithTimeout(context.Background(), runTimeout)
		defer cancel()
		if _, err := c.Run(bg, wfID, "event", e.Data); err != nil {
			slog.Warn("workflow event run failed", "id", wfID, "err", err)
		}
	})
	c.mu.Lock()
	c.eventSubscribed[key] = handle
	c.mu.Unlock()
}

// validateGraph 解析并校验节点图。
func (c *WorkflowChain) validateGraph(wf *models.Workflow) error {
	var gnodes []workflow.GraphNode
	var edges []workflow.GraphEdge
	if wf.Nodes != "" {
		if err := json.Unmarshal([]byte(wf.Nodes), &gnodes); err != nil {
			return fmt.Errorf("节点 JSON 无效: %w", err)
		}
	}
	if wf.Edges != "" {
		if err := json.Unmarshal([]byte(wf.Edges), &edges); err != nil {
			return fmt.Errorf("连线 JSON 无效: %w", err)
		}
	}
	return c.exec.Validate(gnodes, edges)
}

// evaluateEventConditions 校验事件触发条件（全部命中才运行）。空条件=放行。
func evaluateEventConditions(condsJSON string, data map[string]any) bool {
	condsJSON = strings.TrimSpace(condsJSON)
	if condsJSON == "" {
		return true
	}
	var conds []workflow.EventCondition
	if err := json.Unmarshal([]byte(condsJSON), &conds); err != nil {
		return false
	}
	for _, c := range conds {
		raw, ok := data[c.Field]
		if !ok {
			return false
		}
		val := fmt.Sprintf("%v", raw)
		switch c.Operator {
		case "eq":
			if val != c.Value {
				return false
			}
		case "ne":
			if val == c.Value {
				return false
			}
		case "contains":
			if !strings.Contains(val, c.Value) {
				return false
			}
		case "regex":
			matched, err := regexp.MatchString(c.Value, val)
			if err != nil || !matched {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func ptrTime(t time.Time) *time.Time { return &t }

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
