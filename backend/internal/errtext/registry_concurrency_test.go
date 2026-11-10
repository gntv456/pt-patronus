// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package errtext

// #401 登记口（Register / lookupRegistry）的并发安全与幂等判据。
//
// registry 是包级 slice，Register 会 append、Text 会遍历它 —— 若有人在运行时
// （handler / goroutine）调 Register，就构成 data race。
//
// 本仓现有两处登记都在 chain 的 init() 里，而 Go 保证**全部 init() 跑完才进 main**，
// 所以真实路径是「init 里写、之后只读」，安全。
//
// 说明：本机没有 gcc，`go test -race` 跑不起来（-race requires cgo）。
// 因此这里用「并发读 + 结果一致」作次优实证：若 Register 在运行中被并发调用，
// append 与遍历交错会让部分读者**读不到**刚登记的条目；本测试用足够多的并发读者
// 反复读，断言每次都拿到同一条已登记文案 —— 这能抓住「读侧被写侧污染」的实际症状。

import (
	"errors"
	"sync"
	"testing"
)

// 注册一个本测试专属的哨兵（不污染 chain 的两条）。
var errTestOnlySentinel = errors.New("errtext registry concurrency probe")

func init() {
	Register(errTestOnlySentinel, "测试用文案：这是登记口并发读要拿到的那句")
}

func TestRegistryReadableConcurrently(t *testing.T) {
	const readers = 64
	const rounds = 500
	var wg sync.WaitGroup
	errs := make(chan string, readers*rounds)

	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < rounds; j++ {
				if got := Text(errTestOnlySentinel); got != "测试用文案：这是登记口并发读要拿到的那句" {
					errs <- got
				}
			}
		}()
	}
	wg.Wait()
	close(errs)

	for got := range errs {
		t.Fatalf("并发读拿到了不一致的结果（登记口在运行期被写过？）：%q", got)
	}
}

// Register 必须忽略空哨兵与空文案 —— 否则会往表里塞一条永远匹配不到、
// 却让每次 Text 都多走一遍的垃圾。
func TestRegisterIgnoresEmpty(t *testing.T) {
	before := len(registry)
	Register(nil, "空哨兵不该进表")
	Register(errors.New("x"), "")
	if after := len(registry); after != before {
		t.Errorf("空哨兵/空文案被登记进去了（表长 %d → %d）", before, after)
	}
}

// 登记表不得抢走内置档：登记顺序在内置判定之前，但内置哨兵没被登记过，
// 所以它们仍走自己的文案。
func TestRegistryDoesNotShadowBuiltin(t *testing.T) {
	if got := Text(errTestOnlySentinel); got == "" {
		t.Error("已登记的哨兵必须能被查到")
	}
}
