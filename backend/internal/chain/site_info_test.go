package chain

import (
	"context"
	"strings"
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

// fakeFLAdapter 在 fakeAdapter 基础上加 FreeleechCapable；阻塞 + atomic 计数，验证 singleflight。
type fakeFLAdapter struct {
	*fakeAdapter
	notices []site.SiteFreeleech
	calls   int32
	proceed chan struct{}
}

func (f *fakeFLAdapter) FetchFreeleech(context.Context, *site.SiteMetadata, string, string) ([]site.SiteFreeleech, error) {
	atomic.AddInt32(&f.calls, 1)
	<-f.proceed
	return f.notices, nil
}

// TestFreeleechAllConcurrentDedup 验证 singleflight：N 个 FreeleechAll 并发（同 siteID），
// FetchFreeleech 只被调用一次。sem 移入 fn 后 waiter 不占并发槽。
func TestFreeleechAllConcurrentDedup(t *testing.T) {
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	s, _ := siteOp.Create(oper.SiteInput{Name: "t", Platform: "nexusphp", BaseURL: "https://x", IsActive: true})
	src := &fakeFLAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		notices:     []site.SiteFreeleech{{Title: "FREE"}},
		proceed:     make(chan struct{}),
	}
	mm := module.New()
	mm.Register(src)
	ch := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())

	const n = 50
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([][]FreeleechResult, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = ch.FreeleechAll(context.Background(), false)
		}(i)
	}
	close(start)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&src.calls) >= 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Fatalf("expected exactly 1 in-flight FetchFreeleech (singleflight coalesce), got %d", got)
	}
	close(src.proceed)
	wg.Wait()

	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Errorf("after completion: expected 1 call, got %d", got)
	}
	for i, r := range results {
		if len(r) != 1 || r[0].SiteID != s.ID || len(r[0].Notices) != 1 || r[0].Notices[0].Title != "FREE" {
			t.Errorf("result[%d] = %+v, want one site with FREE", i, r)
		}
	}

	// 缓存已写：再次调用应命中，不触发抓取。
	ch.FreeleechAll(context.Background(), false)
	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Errorf("cached call should not re-fetch, calls=%d", got)
	}
}

// fakeAnnAdapter 在 fakeAdapter 基础上加 AnnouncementCapable。
type fakeAnnAdapter struct {
	*fakeAdapter
	ann     *site.SiteAnnouncement
	calls   int32
	proceed chan struct{}
}

func (f *fakeAnnAdapter) FetchAnnouncement(context.Context, *site.SiteMetadata, string, string) (*site.SiteAnnouncement, error) {
	atomic.AddInt32(&f.calls, 1)
	<-f.proceed
	return f.ann, nil
}

// TestAnnouncementsAllConcurrentDedup 验证 singleflight：N 个 AnnouncementsAll 并发（同 siteID），
// FetchAnnouncement 只被调用一次。公告 60s TTL 最易击穿。
func TestAnnouncementsAllConcurrentDedup(t *testing.T) {
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	s, _ := siteOp.Create(oper.SiteInput{Name: "t", Platform: "nexusphp", BaseURL: "https://x", IsActive: true})
	src := &fakeAnnAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp"},
		ann:         &site.SiteAnnouncement{Items: []site.AnnouncementItem{{Title: "ANN"}}},
		proceed:     make(chan struct{}),
	}
	mm := module.New()
	mm.Register(src)
	ch := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())

	const n = 50
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([][]AnnouncementResult, n)
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = ch.AnnouncementsAll(context.Background(), false)
		}(i)
	}
	close(start)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&src.calls) >= 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Fatalf("expected exactly 1 in-flight FetchAnnouncement (singleflight coalesce), got %d", got)
	}
	close(src.proceed)
	wg.Wait()

	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Errorf("after completion: expected 1 call, got %d", got)
	}
	for i, r := range results {
		if len(r) != 1 || r[0].SiteID != s.ID || r[0].Error != "" {
			t.Errorf("result[%d] = %+v, want one site no error", i, r)
		}
	}

	ch.AnnouncementsAll(context.Background(), false)
	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Errorf("cached call should not re-fetch, calls=%d", got)
	}
}

// TestAnnouncementsAll_LoggedOutSurfaced 防回归：cookie 过期(ErrNotLoggedIn) 必须作为 Error
// 暴露给前端，否则公告页会把「登录失效」误显为「暂无公告」，用户无从知晓该站需刷新登录
// （与站免路径 FreeleechAll 行为一致）。前端 friendlySiteError 据错误串中的 "login" 关键词
// 把 "not logged in" 译为「站点登录已失效，请在「站点」页重新配置 Cookie」。
func TestAnnouncementsAll_LoggedOutSurfaced(t *testing.T) {
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	s, _ := siteOp.Create(oper.SiteInput{Name: "t", Platform: "nexusphp", BaseURL: "https://x", IsActive: true})
	src := &loggedOutAnnAdapter{fakeAdapter: &fakeAdapter{schema: "nexusphp"}}
	mm := module.New()
	mm.Register(src)
	ch := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())

	results := ch.AnnouncementsAll(context.Background(), true)
	if len(results) != 1 || results[0].SiteID != s.ID {
		t.Fatalf("results: got %+v want one result for site %d", results, s.ID)
	}
	if results[0].Error == "" {
		t.Fatalf("expected Error surfaced for ErrNotLoggedIn, got empty (silent \"暂无公告\")")
	}
	if !strings.Contains(strings.ToLower(results[0].Error), "cookie") {
		t.Errorf("Error=%q want contains \"cookie\" (前端 friendlySiteError 据此译为「站点登录已失效」)", results[0].Error)
	}
}

// loggedOutAnnAdapter 模拟 cookie 过期：FetchAnnouncement 恒返回 ErrNotLoggedIn。
type loggedOutAnnAdapter struct {
	*fakeAdapter
}

func (f *loggedOutAnnAdapter) FetchAnnouncement(context.Context, *site.SiteMetadata, string, string) (*site.SiteAnnouncement, error) {
	return nil, site.ErrNotLoggedIn
}
