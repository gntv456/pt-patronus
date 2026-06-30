// Package module 实现可插拔模块注册表与 Run 调度原语（借鉴 MoviePilot 的
// ModuleManager + run_module）。Chain 层通过 Run 按 priority 遍历模块，
// 依据返回的控制信号聚合结果。
package module

import (
	"context"
	"sort"
	"sync"
)

// Module 是可注册的模块。Priority 数值越小优先级越高（越先被遍历）。
type Module interface {
	Name() string
	Priority() int
}

// Control 控制 Run 的聚合行为：
//   - Skip：本模块不处理（None 语义），继续下一个
//   - Continue：本模块已处理，收集结果，继续下一个（列表/扇出语义）
//   - Stop：本模块已处理，收集结果并停止（单值/命中语义）
type Control int

const (
	Skip Control = iota
	Continue
	Stop
)

// Manager 是模块注册表。
type Manager struct {
	mu     sync.RWMutex
	mods   []Module
	sorted bool
}

// New 构造一个空 Manager。
func New() *Manager { return &Manager{} }

// Register 注册若干模块。
func (m *Manager) Register(mods ...Module) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mods = append(m.mods, mods...)
	m.sorted = false
}

// List 返回按 priority 升序排列的模块快照。
func (m *Manager) List() []Module {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.sorted {
		sort.SliceStable(m.mods, func(i, j int) bool {
			return m.mods[i].Priority() < m.mods[j].Priority()
		})
		m.sorted = true
	}
	out := make([]Module, len(m.mods))
	copy(out, m.mods)
	return out
}

// Run 是调度原语：按优先级遍历模块，对每个调用 fn。
// fn 返回 (值, 控制信号)；依信号收集或停止，返回聚合切片。
// 遍历过程响应 ctx 取消。
func Run[T any](ctx context.Context, m *Manager, fn func(Module) (T, Control)) []T {
	var out []T
	for _, mod := range m.List() {
		select {
		case <-ctx.Done():
			return out
		default:
		}
		v, c := fn(mod)
		switch c {
		case Skip:
			continue
		case Continue:
			out = append(out, v)
		case Stop:
			out = append(out, v)
			return out
		}
	}
	return out
}
