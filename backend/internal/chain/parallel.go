// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"

	"github.com/hxpt/ptpatronus/internal/errtext"
)

// acquireSemaphore waits for a shared concurrency slot without ignoring cancellation.
func acquireSemaphore(ctx context.Context, sem chan struct{}) error {
	if sem == nil {
		return nil
	}
	select {
	case sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ctxErrText 把「聚合限时截断」的错误转成用户可读文案。非流式聚合端点走
// aggregateRequestContext（15s），到点后被截断站点的错误是 context.DeadlineExceeded/
// Canceled（等待 sem 槽位/单飞共享）或底层包裹的 "context canceled"（HTTP 中断）。
// 开发者术语直出卡片/药单导出会让用户无从判断该做什么；此时统一给「稍后重试」语义。
// ctx.Err() 非空即说明本次调用确系外层限时截断；否则按真实错误原样透出。
func ctxErrText(ctx context.Context, err error) string {
	if ctx.Err() == nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		// 逐条结果行的兜底必须走 errtext：从前这里直接 err.Error()，
		// 于是后宫/求种/公告等 11 个聚合面把 "ssrf: blocked private/internal address: 127.0.0.1"
		// 这类英文原文端给用户（#112 的单一文案源漏了这一处）。
		return errtext.Text(err)
	}
	return "抓取超时（未在聚合时限内完成），请稍后重试"
}
