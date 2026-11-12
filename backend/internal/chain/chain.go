// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

// Package chain 是业务编排层基类，编排层基类。
// Base 持有模块管理器与事件总线；各业务 Chain（SiteChain 等）内嵌 Base，
// 通过 RunModule 按 priority 遍历模块并聚合结果。
package chain

import (
	"context"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
)

// Base 是所有 Chain 的基类。
type Base struct {
	MM  *module.Manager
	Bus *event.Bus
}

// NewBase 构造 Base。
func NewBase(mm *module.Manager, bus *event.Bus) Base {
	return Base{MM: mm, Bus: bus}
}

// RunModule 是 Chain 调度原语：按优先级遍历模块，fn 返回 (值, 控制信号)，
// 依 Skip/Continue/Stop 聚合。详见 module.Run。
func RunModule[T any](ctx context.Context, b *Base, fn func(module.Module) (T, module.Control)) []T {
	return module.Run(ctx, b.MM, fn)
}

// Emit 广播一个事件。
func (b *Base) Emit(ctx context.Context, eventType string, data map[string]any) {
	if b.Bus == nil {
		return
	}
	b.Bus.Publish(ctx, eventType, data)
}
