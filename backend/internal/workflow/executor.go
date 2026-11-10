// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package workflow

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// nodeTimeout 单节点执行超时（防止慢节点卡死整条流）。
const nodeTimeout = 60 * time.Second

// Executor 在单 goroutine 内按拓扑顺序执行节点图。
//
// 分支语义：条件节点在 Execute 内设置 ecx.Next（选中的分支标签）。Executor 执行完任一节点后，
// 裁剪其 SourcePort 非空且不在 ecx.Next 中的出边（仅条件节点的出边带 SourcePort，故只影响分支）。
// 可达性：从结构根（无入边的节点）沿「活动边」BFS；不可达节点（未选中的分支及其下游）永不执行。
// 可运行：节点可达 且 其所有「来自可达源的活动入边」的源节点均已执行（合并节点等齐多前驱，AND 语义）。
type Executor struct {
	reg      *Registry
	failFast bool
}

// NewExecutor 构造（failFast=true：首个失败节点立即终止）。
func NewExecutor(reg *Registry) *Executor { return &Executor{reg: reg, failFast: true} }

// WithContinueOnError 切换为容错模式：节点失败时仅记录日志并继续后续节点，而非 fail-fast 终止。
// 适用于「某节点失败不应阻断整条流」的场景（如非关键通知）。默认仍 fail-fast。
// 同时让原本不可达的容错分支可被测试覆盖。
func (e *Executor) WithContinueOnError() *Executor { e.failFast = false; return e }

// Run 执行整张图，返回累积日志与首个错误（fail-fast 时）。
func (e *Executor) Run(ctx context.Context, gnodes []GraphNode, edges []GraphEdge, ecx *Context) ([]LogEntry, error) {
	if ecx.Variables == nil {
		ecx.Variables = map[string]any{}
	}
	nodeByID := make(map[string]*GraphNode, len(gnodes))
	for i := range gnodes {
		nodeByID[gnodes[i].ID] = &gnodes[i]
	}
	// 结构入度（含全部边），用于判定根节点。
	inDegTotal := make(map[string]int, len(gnodes))
	for _, e := range edges {
		inDegTotal[e.Target]++
	}

	executed := make(map[string]bool, len(gnodes))
	// active[edgeIndex]bool：活动边集合（条件分支裁剪时移除）。
	active := make([]bool, len(edges))
	for i := range edges {
		active[i] = true
	}

	// reachable 沿活动边从结构根 BFS。
	reachable := func() map[string]bool {
		r := make(map[string]bool, len(gnodes))
		queue := make([]string, 0)
		for _, n := range gnodes {
			if inDegTotal[n.ID] == 0 { // 结构根
				r[n.ID] = true
				queue = append(queue, n.ID)
			}
		}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for i, ed := range edges {
				if active[i] && ed.Source == cur && !r[ed.Target] {
					r[ed.Target] = true
					queue = append(queue, ed.Target)
				}
			}
		}
		return r
	}

	// pendingActive[node] = 来自「可达源」的活动入边中尚未执行的源数。
	pendingActive := func(reach map[string]bool) map[string]int {
		pa := make(map[string]int, len(gnodes))
		for i, ed := range edges {
			if !active[i] {
				continue
			}
			if !reach[ed.Source] {
				continue // 源不可达（未选中分支上的节点）→ 不计入依赖
			}
			if executed[ed.Source] {
				continue // 源已执行
			}
			pa[ed.Target]++
		}
		return pa
	}

	for {
		// 每轮重算可达性与待执行入度，选出下一个可运行节点（可达 & 无未执行的可达前驱）。
		// 拓扑序自然由「源先执行」保证；同批多可运行节点按 gnodes 原始顺序稳定选取。
		reach := reachable()
		pa := pendingActive(reach)
		var next string
		found := false
		for i := range gnodes {
			id := gnodes[i].ID
			if executed[id] || !reach[id] {
				continue
			}
			if pa[id] == 0 {
				next = id
				found = true
				break
			}
		}
		if !found {
			break // 无可运行节点（剩余均为不可达的未选中分支，正常结束）
		}

		gnode := nodeByID[next]
		node, ok := e.reg.Get(gnode.Type)
		ecx.Next = nil // 清空上一节点的分支选择，防泄漏
		if !ok {
			ecx.Log = append(ecx.Log, LogEntry{
				NodeID: next, NodeType: gnode.Type, NodeName: gnode.Name,
				Status: "skipped", Message: "未知节点类型：" + gnode.Type, At: nowRFC3339(),
			})
			executed[next] = true
			if e.failFast {
				return ecx.Log, fmt.Errorf("未知节点类型：%s", gnode.Type)
			}
			continue
		}

		// 每节点循环（参考 MoviePilot loop/loop_interval）：loop=true 时按 loop_times 重试，成功即停。
		runOnce := func() (runErr error) {
			octx, ocancel := context.WithTimeout(ctx, nodeExecutionTimeout(gnode.Params))
			defer ocancel()
			// 节点 Execute 涉及 HTTP/JSON/类型断言等多层，panic 不应炸垮整个工作流执行器
			// （会冒泡到 webhook/agent 等后台 goroutine，进程级 goroutine 崩溃）。
			defer func() {
				if r := recover(); r != nil {
					runErr = fmt.Errorf("节点 %s 执行 panic: %v", gnode.Type, r)
				}
			}()
			return node.Execute(octx, gnode.Params, ecx)
		}
		start := time.Now()
		var err error
		attempts := 1 // 重试次数（loop 模式下 >1），用于失败日志标注「重试 N 次后仍失败」
		if ParamBool(gnode.Params, "loop") {
			times := ParamInt(gnode.Params, "loop_times")
			if times <= 0 {
				times = 3
			}
			if times > 20 {
				times = 20 // 上限防爆
			}
			interval := ParamInt(gnode.Params, "loop_interval_sec")
			if interval > 60 {
				interval = 60
			}
			for i := 0; i < times; i++ {
				attempts = i + 1
				err = runOnce()
				if err == nil {
					break // 成功即停（重试语义）
				}
				if i < times-1 && interval > 0 {
					select {
					case <-time.After(time.Duration(interval) * time.Second):
					case <-ctx.Done():
						err = ctx.Err()
					}
					if err == context.Canceled || ctx.Err() != nil {
						break
					}
				}
			}
		} else {
			err = runOnce()
		}
		dur := time.Since(start).Milliseconds()

		if err != nil {
			msg := err.Error()
			if attempts > 1 {
				msg = fmt.Sprintf("重试 %d 次后仍失败：%s", attempts, msg)
			}
			ecx.Log = append(ecx.Log, LogEntry{
				NodeID: next, NodeType: gnode.Type, NodeName: gnode.Name,
				Status: "failed", Message: msg, DurationMS: dur, At: nowRFC3339(),
			})
			executed[next] = true
			if e.failFast {
				return ecx.Log, fmt.Errorf("节点 %s(%s) 执行失败: %s", gnode.Name, gnode.Type, msg)
			}
		} else {
			ecx.Log = append(ecx.Log, LogEntry{
				NodeID: next, NodeType: gnode.Type, NodeName: gnode.Name,
				Status: "success", Message: "", DurationMS: dur, At: nowRFC3339(),
			})
		}

		// 分支裁剪：移除本节点 SourcePort 非空且不在 ecx.Next 中的出边。
		if len(ecx.Next) > 0 {
			chosen := make(map[string]bool, len(ecx.Next))
			for _, b := range ecx.Next {
				chosen[b] = true
			}
			for i, ed := range edges {
				if active[i] && ed.Source == next && ed.SourcePort != "" && !chosen[ed.SourcePort] {
					active[i] = false
				}
			}
		}

		executed[next] = true
		if ecx.Stop {
			break
		}
	}
	return ecx.Log, nil
}

// nowRFC3339 当前时间 RFC3339（节点日志时间戳）。
func nowRFC3339() string { return time.Now().Format(time.RFC3339) }

func nodeExecutionTimeout(params map[string]any) time.Duration {
	sec := ParamInt(params, "timeout_sec")
	if sec <= 0 {
		return nodeTimeout
	}
	if sec < 5 {
		sec = 5
	}
	if sec > 600 {
		sec = 600
	}
	return time.Duration(sec) * time.Second
}

// Validate 保存/运行前校验图结构：非空、至少一个根节点、无环（Kahn 全图检测）、
// 节点类型存在于 Registry、边端点存在、必填参数齐全。
func (e *Executor) Validate(gnodes []GraphNode, edges []GraphEdge) error {
	if len(gnodes) == 0 {
		return errors.New("工作流不能为空")
	}
	nodeByID := make(map[string]*GraphNode, len(gnodes))
	for i := range gnodes {
		if gnodes[i].ID == "" {
			return errors.New("存在无 id 的节点")
		}
		if _, dup := nodeByID[gnodes[i].ID]; dup {
			return fmt.Errorf("节点 id 重复：%s", gnodes[i].ID)
		}
		nodeByID[gnodes[i].ID] = &gnodes[i]
	}
	// 类型存在 + 必填参数
	for i := range gnodes {
		gn := &gnodes[i]
		node, ok := e.reg.Get(gn.Type)
		if !ok {
			return fmt.Errorf("未知节点类型：%s", gn.Type)
		}
		for _, f := range node.ParamSchema() {
			if !f.Required {
				continue
			}
			v := ParamString(gn.Params, f.Name)
			if v == "" {
				return fmt.Errorf("节点 %s 缺少必填参数：%s", gn.Name, f.Label)
			}
		}
	}
	// 边端点存在
	for _, ed := range edges {
		if _, ok := nodeByID[ed.Source]; !ok {
			return fmt.Errorf("边的源节点不存在：%s", ed.Source)
		}
		if _, ok := nodeByID[ed.Target]; !ok {
			return fmt.Errorf("边的目标节点不存在：%s", ed.Target)
		}
	}
	// 环检测：Kahn 全图
	inDeg := make(map[string]int, len(gnodes))
	for _, ed := range edges {
		inDeg[ed.Target]++
	}
	queue := make([]string, 0)
	for _, n := range gnodes {
		if inDeg[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}
	processed := 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		processed++
		for _, ed := range edges {
			if ed.Source == cur {
				inDeg[ed.Target]--
				if inDeg[ed.Target] == 0 {
					queue = append(queue, ed.Target)
				}
			}
		}
	}
	if processed != len(gnodes) {
		return errors.New("工作流存在环路，无法确定执行顺序")
	}
	return nil
}
