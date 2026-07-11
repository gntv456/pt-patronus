package chain

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// fakeHaremAdapter 在 fakeAdapter 基础上加 HaremCapable（FetchInviteStatus + FetchInvitees）；
// 两方法各自 atomic 计数 + 共享 proceed，验证 singleflight 下后宫两步 HTTP 各只调一次。
type fakeHaremAdapter struct {
	*fakeAdapter
	status        *site.InviteStatus
	invitees      []site.InviteeInfo
	statusCalls   int32
	inviteesCalls int32
	proceed       chan struct{}
}

func (f *fakeHaremAdapter) FetchInviteStatus(context.Context, *site.SiteMetadata, string, string) (*site.InviteStatus, error) {
	atomic.AddInt32(&f.statusCalls, 1)
	<-f.proceed
	return f.status, nil
}
func (f *fakeHaremAdapter) FetchInvitees(context.Context, *site.SiteMetadata, string, string) ([]site.InviteeInfo, error) {
	atomic.AddInt32(&f.inviteesCalls, 1)
	<-f.proceed
	return f.invitees, nil
}

// TestHaremAllConcurrentDedup 验证 singleflight：N 个 HaremAll 并发（同 siteID），
// 两步 HTTP（FetchInviteStatus + FetchInvitees）各只被调用一次（double-HTTP 击穿代价最高）。
func TestHaremAllConcurrentDedup(t *testing.T) {
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	s, _ := siteOp.Create(oper.SiteInput{Name: "t", Platform: "nexusphp", BaseURL: "https://x", IsActive: true})
	src := &fakeHaremAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		status:      &site.InviteStatus{},
		proceed:     make(chan struct{}),
	}
	mm := module.New()
	mm.Register(src)
	ch := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())

	const n = 50
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]*HaremResult, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = ch.HaremAll(context.Background(), false)
		}(i)
	}
	close(start)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&src.statusCalls) >= 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&src.statusCalls); got != 1 {
		t.Fatalf("expected exactly 1 in-flight FetchInviteStatus (singleflight coalesce), got %d", got)
	}
	close(src.proceed)
	wg.Wait()

	if got := atomic.LoadInt32(&src.statusCalls); got != 1 {
		t.Errorf("status calls: expected 1, got %d", got)
	}
	if got := atomic.LoadInt32(&src.inviteesCalls); got != 1 {
		t.Errorf("invitees calls: expected 1, got %d", got)
	}
	for i, r := range results {
		if r == nil || len(r.Prescriptions) != 1 || r.Prescriptions[0].SiteID != s.ID {
			t.Errorf("result[%d] = %+v, want one prescription", i, r)
		}
	}

	// 缓存已写：再次调用应命中，不触发抓取。
	ch.HaremAll(context.Background(), false)
	if got := atomic.LoadInt32(&src.statusCalls); got != 1 {
		t.Errorf("cached call should not re-fetch, statusCalls=%d", got)
	}
}
