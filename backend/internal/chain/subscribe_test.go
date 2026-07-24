package chain

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/media"
)

type subscriptionPosterSource struct {
	name   string
	items  []media.MediaInfo
	detail *media.MediaInfo
}

func (f subscriptionPosterSource) Name() string { return f.name }
func (f subscriptionPosterSource) Search(_ context.Context, _ string, _ int) ([]media.MediaInfo, error) {
	return f.items, nil
}
func (f subscriptionPosterSource) Detail(_ context.Context, _ string) (*media.MediaInfo, error) {
	return f.detail, nil
}

// TestRatingGate 按（已回退全局订阅默认的）有效评分保护开关 + 评分高低决定是否拦截本轮推送。
// 全局 MediaQualityConfig 默认 Enabled=false，故仅 enableGuard=true 时触发。
func TestRatingGate(t *testing.T) {
	// 低分源：聚合应不通过。
	lowCh := newRatingChain(ratingFakeSrc{name: "tmdb", items: []media.MediaInfo{
		{Source: "tmdb", Title: "Bad", Rating: 4.0, VoteCount: 500, ReleaseDate: "2010-01-01"},
	}})
	subCh := &SubscribeChain{mediaCh: lowCh}

	// 订阅开启评分闸门 + 低分 → 拦截。
	held, reason := subCh.ratingGate(context.Background(), &models.Subscription{Title: "Bad"}, true)
	if !held || reason == "" {
		t.Fatalf("expected held with reason, got held=%v reason=%q", held, reason)
	}

	// 订阅未开启评分闸门 → 放行（全局默认关）。
	held, _ = subCh.ratingGate(context.Background(), &models.Subscription{Title: "Bad"}, false)
	if held {
		t.Fatal("expected not held when guard disabled")
	}

	// 高分源：聚合通过 → 不拦截。
	highCh := newRatingChain(ratingFakeSrc{name: "tmdb", items: []media.MediaInfo{
		{Source: "tmdb", Title: "Good", Rating: 8.5, VoteCount: 5000, ReleaseDate: "2010-01-01"},
	}})
	highSub := &SubscribeChain{mediaCh: highCh}
	held, _ = highSub.ratingGate(context.Background(), &models.Subscription{Title: "Good"}, true)
	if held {
		t.Fatal("expected not held for high-rated media")
	}

	// 未装配 mediaCh → 始终放行（fail-open / 置灰）。
	noneSub := &SubscribeChain{mediaCh: nil}
	held, _ = noneSub.ratingGate(context.Background(), &models.Subscription{Title: "X"}, true)
	if held {
		t.Fatal("expected not held when mediaCh is nil")
	}
}

func TestSubscriptionPoster(t *testing.T) {
	t.Run("prefers exact source detail", func(t *testing.T) {
		mediaCh := newRatingChain(
			subscriptionPosterSource{
				name:   "douban",
				detail: &media.MediaInfo{PosterURL: "https://img.example/exact.jpg"},
				items:  []media.MediaInfo{{PosterURL: "https://img.example/search.jpg"}},
			},
		)
		c := &SubscribeChain{mediaCh: mediaCh}
		got := c.subscriptionPoster(context.Background(), &models.Subscription{Source: "douban", SourceID: "123", Title: "测试"})
		if got != "https://img.example/exact.jpg" {
			t.Fatalf("poster = %q", got)
		}
	})

	t.Run("falls back to title search", func(t *testing.T) {
		mediaCh := newRatingChain(
			subscriptionPosterSource{name: "douban", items: []media.MediaInfo{{PosterURL: "https://img.example/search.jpg"}}},
		)
		c := &SubscribeChain{mediaCh: mediaCh}
		got := c.subscriptionPoster(context.Background(), &models.Subscription{Source: "unknown", Title: "测试", Year: 2026})
		if got != "https://img.example/search.jpg" {
			t.Fatalf("poster = %q", got)
		}
	})
}

// lockCheck per-subscription 互斥：同一 subID 第二次加锁须等第一次解锁后才能获得（缺陷 5 修复）。
// 防 Create 建后异步 Check 与 cron CheckAll 对同一订阅并发重复推送。
func TestLockCheckPerSubscriptionMutex(t *testing.T) {
	c := &SubscribeChain{}
	unlock := c.lockCheck(42)

	// 已持锁：另起 goroutine 再锁同一 subID，应阻塞直到我们解锁。
	acquired := make(chan struct{})
	go func() {
		release := c.lockCheck(42)
		close(acquired)
		release()
	}()

	select {
	case <-acquired:
		t.Fatal("second lockCheck should block while first holds the lock")
	case <-time.After(50 * time.Millisecond):
		// 预期阻塞：OK
	}

	unlock() // 释放第一次锁

	select {
	case <-acquired:
		// 第二次获得锁：OK
	case <-time.After(time.Second):
		t.Fatal("second lockCheck did not acquire after unlock (deadlock)")
	}

	// 不同 subID 互不阻塞：另起一个 subID 应立即获得。
	other := make(chan struct{})
	go func() {
		r := c.lockCheck(99)
		close(other)
		r()
	}()
	select {
	case <-other:
	case <-time.After(time.Second):
		t.Fatal("different subID should not block")
	}
}

// Check 走 per-sub 互斥：并发两次 Check 同一订阅应串行（不重叠）。
// 用 inFlight 计数验证：若真并发，inFlight 会 >1。
func TestCheckSerializedPerSubscription(t *testing.T) {
	c := &SubscribeChain{} // 空 chain：Check 内部 lockCheck 后 checkWithDefaults 会因无下载器报错，但锁已验证

	var inFlight, maxInFlight int32
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			unlock := c.lockCheck(7)
			cur := atomic.AddInt32(&inFlight, 1)
			// 记录峰值
			for {
				m := atomic.LoadInt32(&maxInFlight)
				if cur <= m || atomic.CompareAndSwapInt32(&maxInFlight, m, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond) // 模拟 Check 临界区工作
			atomic.AddInt32(&inFlight, -1)
			unlock()
			done <- nil
		}()
	}
	<-done
	<-done

	if maxInFlight > 1 {
		t.Errorf("expected serialized Check (max in-flight 1), got %d", maxInFlight)
	}
}
