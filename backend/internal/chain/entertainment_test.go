package chain

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/entertainment"
	"github.com/hxpt/ptpatronus/internal/httpclient"
)

// —— spendGuard 单元测试 ——

func TestSpendGuardInflightDedup(t *testing.T) {
	g := newSpendGuard()
	if !g.acquire("k") {
		t.Fatal("first acquire should succeed")
	}
	if g.acquire("k") {
		t.Fatal("second acquire while in-flight should fail")
	}
	g.release("k")
	if !g.acquire("k") {
		t.Fatal("acquire after release should succeed")
	}
	g.release("k")
	// 不同键互不影响
	if !g.acquire("other") {
		t.Fatal("different key should acquire independently")
	}
	g.release("other")
}

// TestSpendGuardConcurrentExactlyOneWinner：50 个 goroutine 同时 acquire 同一键，
// 在胜者持锁期间其余 49 个必须全部失败 → 精确 1 个胜者（证明并发去重无遗漏）。
func TestSpendGuardConcurrentExactlyOneWinner(t *testing.T) {
	g := newSpendGuard()
	const n = 50
	var winners, attempted int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			<-start
			got := g.acquire("same-key")
			atomic.AddInt32(&attempted, 1)
			if got {
				atomic.AddInt32(&winners, 1)
				for atomic.LoadInt32(&attempted) < n {
					runtime.Gosched() // 持锁直到所有 goroutine 都尝试过
				}
				g.release("same-key")
			}
		}()
	}
	close(start)
	wg.Wait()
	if winners != 1 {
		t.Errorf("expected exactly 1 winner under concurrency, got %d", winners)
	}
}

// —— chain 级端到端：并发双击同一花钱操作只触发一次外部调用 ——

// blockGame 是测试用 LotteryGame：Play 阻塞在 proceed 上模拟慢外部调用，并计数实际执行次数。
type blockGame struct {
	id      string
	proceed chan struct{} // 关闭它 Play 才返回
	calls   int32         // Play 实际进入次数（应为 1：被拦的那个不会进入）
}

func (g *blockGame) ID() string   { return g.id }
func (g *blockGame) Name() string { return "block" }
func (g *blockGame) Play(_ context.Context, _ *httpclient.Client, _ string, _ entertainment.Creds) (*entertainment.PlayResult, error) {
	atomic.AddInt32(&g.calls, 1)
	<-g.proceed
	return &entertainment.PlayResult{Success: true, Message: "ok"}, nil
}

// TestEntertainmentPlayConcurrentDedup：两个并发 Play 同一站点+游戏：
// 先到的占住 guard 进入（阻塞的）外部调用，后到的立刻被 ErrDuplicateSpend 拒绝；
// 最终外部调用只执行 1 次（未被重复扣币）。
func TestEntertainmentPlayConcurrentDedup(t *testing.T) {
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	s, _ := siteOp.Create(oper.SiteInput{Name: "t", Platform: "nexusphp", BaseURL: "https://game.test"})

	reg := entertainment.NewRegistry()
	game := &blockGame{id: "g1", proceed: make(chan struct{})}
	reg.Register("game.test", game)

	ch := NewEntertainmentChain(NewBase(module.New(), event.New()), reg, nil, siteOp)

	// 先发的请求：应拿到 guard 并进入 Play（阻塞在 proceed）
	type result struct {
		res *entertainment.PlayResult
		err error
	}
	firstRes := make(chan result, 1)
	go func() {
		r, e := ch.Play(context.Background(), s.ID, "g1")
		firstRes <- result{r, e}
	}()

	// 等待先发请求确实进入了 Play（calls==1 ⇒ 已占住 guard）。轮询最多 2s；
	// 若先发请求在进入前就出错返回（resolve 失败等），捕获错误以便诊断。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&game.calls) != 1 {
		select {
		case fr := <-firstRes:
			if fr.err != nil {
				t.Fatalf("first Play errored before entering fake game: %v", fr.err)
			}
			t.Fatalf("first Play returned without entering fake game (res=%v)", fr.res)
		default:
		}
		time.Sleep(time.Millisecond)
	}
	if atomic.LoadInt32(&game.calls) != 1 {
		t.Fatal("first Play never entered (guard not acquired) within timeout")
	}

	// 第二个并发请求：同一 site+game 在 in-flight 期间必须被拒，且不进入 Play（calls 仍为 1）
	_, err := ch.Play(context.Background(), s.ID, "g1")
	if !errors.Is(err, ErrDuplicateSpend) {
		t.Fatalf("expected ErrDuplicateSpend for concurrent duplicate, got %v", err)
	}
	if got := atomic.LoadInt32(&game.calls); got != 1 {
		t.Errorf("external Play should run exactly once, got %d calls", got)
	}

	// 放行先发请求 → 成功返回
	close(game.proceed)
	fr := <-firstRes
	if fr.err != nil || fr.res == nil || !fr.res.Success {
		t.Fatalf("first Play should succeed, got res=%v err=%v", fr.res, fr.err)
	}
}

// TestEntertainmentSitesGameCount：Sites() 返回每站已适配娱乐项目数，计数按 base_url
// 的 host 匹配 registry（含 www 子域放宽），未注册游戏的站点为 0——对应娱乐页筛选徽标。
func TestEntertainmentSitesGameCount(t *testing.T) {
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	// rich：base_url 带 www，registry 用裸域 → 验证子域放宽后仍计 3 个。
	_, _ = siteOp.Create(oper.SiteInput{Name: "rich", Platform: "nexusphp", BaseURL: "https://www.hxpt.org"})
	_, _ = siteOp.Create(oper.SiteInput{Name: "single", Platform: "nexusphp", BaseURL: "https://cspt.fun"})
	_, _ = siteOp.Create(oper.SiteInput{Name: "none", Platform: "nexusphp", BaseURL: "https://bare.test"})

	reg := entertainment.NewRegistry()
	reg.Register("hxpt.org", &blockGame{id: "g1", proceed: make(chan struct{})})
	reg.Register("hxpt.org", &blockGame{id: "g2", proceed: make(chan struct{})})
	reg.Register("hxpt.org", &blockGame{id: "g3", proceed: make(chan struct{})})
	reg.Register("cspt.fun", &blockGame{id: "g1", proceed: make(chan struct{})})

	ch := NewEntertainmentChain(NewBase(module.New(), event.New()), reg, nil, siteOp)

	got, err := ch.Sites()
	if err != nil {
		t.Fatalf("Sites() error: %v", err)
	}
	byName := map[string]int{}
	for _, s := range got {
		byName[s.Name] = s.GameCount
	}
	if byName["rich"] != 3 {
		t.Errorf("rich (www.hxpt.org→hxpt.org) want 3 games, got %d", byName["rich"])
	}
	if byName["single"] != 1 {
		t.Errorf("single (cspt.fun) want 1 game, got %d", byName["single"])
	}
	if byName["none"] != 0 {
		t.Errorf("none (bare.test) want 0 games, got %d", byName["none"])
	}
}
