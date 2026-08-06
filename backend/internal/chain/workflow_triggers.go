package chain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/workflow"
	"github.com/robfig/cron/v3"
)

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
			// 轻量启发式：限制 regex 长度上限，拒绝明显异常的拼凑巨型 pattern（可能拖慢编译/匹配）。
			// Go regexp 基于 RE2 保证匹配线性、无 ReDoS 回溯爆炸，此处仅作防御性拦截。
			if len(condition.Value) > 256 {
				return fmt.Errorf("event condition regex 过长（>%d 字符），已拒绝", 256)
			}
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
	wfID := wf.ID
	eventType := wf.EventType
	// check-then-act 原子化：去重检查、bus.Subscribe、写 map 全程持 c.mu，消除原先「释放锁再订阅再抢锁」
	// 的 TOCTOU 窗口（两并发调用会双双通过去重检查 → 重复订阅，泄漏一个无法退订的 handle）。
	// bus.Subscribe 内部自带 bus.mu，锁序恒为 c.mu→bus.mu；Publish 在 copy handlers 后即释放 bus.mu 再回调，
	// 回调内经 c.Run 取 c.mu 不构成反向嵌套，无死锁风险。
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.eventSubscribed[key]; ok {
		return
	}
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
	c.eventSubscribed[key] = handle
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
			// 安全说明：Go regexp（RE2）匹配为线性时间，不存在 PCRE 风格的灾难性回溯（ReDoS）；
			// 且 validateEventConditions 已对 pattern 长度做上限拦截，val 来自单个事件字段通常很短，
			// 故此处无需额外超时机制（Go regexp 也不支持 MatchString 传 context/超时）。
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
