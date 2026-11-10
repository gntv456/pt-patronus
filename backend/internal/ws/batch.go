// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package ws

import (
	"os"
	"sync"
)

// Batch 把同一 tick 内的多条广播合并为一次 Hub 遍历（阶段 3「快慢分离」）。
// 原语义每条消息取一次 RLock；N 条消息合并后只取一次，高频推送时锁竞争显著下降。
//
// feature flag：PTP_WS_BATCH=0 时 Flush 退化为逐条 Broadcast，可秒级回退。
// 默认开启（环境变量未设置时视为开启）。
type Batch struct {
	hub      *Hub
	payloads [][]byte
	enabled  bool
}

var batchFlagOnce sync.Once
var batchFlag bool

func batchEnabled() bool {
	batchFlagOnce.Do(func() { batchFlag = os.Getenv("PTP_WS_BATCH") != "0" })
	return batchFlag
}

// NewBatch 构造一个批量广播器。
func NewBatch(h *Hub) *Batch { return &Batch{hub: h, enabled: batchEnabled()} }

// Add 追加一条待广播的已编码消息（不复制，调用方在 Flush 前不得复用底层数组）。
func (b *Batch) Add(payload []byte) {
	if len(payload) > 0 {
		b.payloads = append(b.payloads, payload)
	}
}

// Len 返回当前积压条数。
func (b *Batch) Len() int { return len(b.payloads) }

// Flush 一次持锁把所有消息按序投递给所有连接，然后清空。
// 与 Hub.Broadcast 一致：某连接发不出去则跳过该条，绝不阻塞。
func (b *Batch) Flush() {
	if len(b.payloads) == 0 {
		return
	}
	if !b.enabled {
		for _, p := range b.payloads {
			b.hub.Broadcast(p)
		}
		b.payloads = b.payloads[:0]
		return
	}
	b.hub.mu.RLock()
	defer b.hub.mu.RUnlock()
	for c := range b.hub.clients {
		for _, p := range b.payloads {
			select {
			case c.send <- p:
			default:
			}
		}
	}
	b.payloads = b.payloads[:0]
}
