// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"fmt"

	"golang.org/x/sync/singleflight"
)

// sfDo 包装 singleflight.Group.Do，消除调用处 interface{} 类型断言样板，做 read-coalescing
// （同 key 并发请求只回源一次，其余共享结果，防缓存击穿）。
//
// g 零值可用。fn 开头必须 double-check 缓存：sf entry 在 leader 返回后即移除，紧随其后的
// 新 caller 会成为新 leader 重跑 fn，double-check 命中刚写入的缓存可直接返回，避免冗余回源。
//
// fn 内禁止持有外层缓存 mu（回源期间），否则 waiter 死锁——本包各处 fetch 均在 mu Unlock 之后。
func sfDo[V any](g *singleflight.Group, key string, fn func() (V, error)) (V, error) {
	v, err, _ := g.Do(key, func() (interface{}, error) { return fn() })
	if err != nil {
		var zero V
		return zero, err
	}
	if vv, ok := v.(V); ok {
		return vv, nil
	}
	return *new(V), fmt.Errorf("singleflight: unexpected type %T", v)
}

// sfDoContext coalesces reads while allowing a waiting caller to honor its own deadline.
func sfDoContext[V any](ctx context.Context, g *singleflight.Group, key string, fn func() (V, error)) (V, error) {
	result := g.DoChan(key, func() (interface{}, error) { return fn() })
	select {
	case r := <-result:
		if r.Err != nil {
			var zero V
			return zero, r.Err
		}
		if value, ok := r.Val.(V); ok {
			return value, nil
		}
		return *new(V), fmt.Errorf("singleflight: unexpected type %T", r.Val)
	case <-ctx.Done():
		var zero V
		return zero, ctx.Err()
	}
}
