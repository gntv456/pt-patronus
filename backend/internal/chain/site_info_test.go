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
// panicHost 非空时，baseURL 命中的站在 FetchFreeleech 现场 panic（per-site 隔离用例）。
type fakeFLAdapter struct {
	*fakeAdapter
	notices   []site.SiteFreeleech
	err       error
	calls     int32
	proceed   chan struct{}
	panicHost string
}

func (f *fakeFLAdapter) FetchFreeleech(_ context.Context, _ *site.SiteMetadata, baseURL, _ string) ([]site.SiteFreeleech, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.panicHost != "" && strings.Contains(baseURL, f.panicHost) {
		panic("nil pointer dereference on malformed freeleech page")
	}
	if f.proceed != nil {
		<-f.proceed
	}
	return f.notices, f.err
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
			results[i], _ = ch.FreeleechAll(context.Background(), false)
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
	if _, err := ch.FreeleechAll(context.Background(), false); err != nil {
		t.Fatalf("缓存命中调用不应报错: %v", err)
	}
	if got := atomic.LoadInt32(&src.calls); got != 1 {
		t.Errorf("cached call should not re-fetch, calls=%d", got)
	}
}

// fakeAnnAdapter 在 fakeAdapter 基础上加 AnnouncementCapable。
// panicHost 非空时，baseURL 命中的站在 FetchAnnouncement 现场 panic（per-site 隔离用例）。
type fakeAnnAdapter struct {
	*fakeAdapter
	ann       *site.SiteAnnouncement
	err       error
	calls     int32
	proceed   chan struct{}
	panicHost string
}

func (f *fakeAnnAdapter) FetchAnnouncement(_ context.Context, _ *site.SiteMetadata, baseURL, _ string) (*site.SiteAnnouncement, error) {
	atomic.AddInt32(&f.calls, 1)
	if f.panicHost != "" && strings.Contains(baseURL, f.panicHost) {
		panic("nil pointer dereference on malformed announcement page")
	}
	if f.proceed != nil {
		<-f.proceed
	}
	return f.ann, f.err
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
			results[i], _ = ch.AnnouncementsAll(context.Background(), false)
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

	if _, err := ch.AnnouncementsAll(context.Background(), false); err != nil {
		t.Fatalf("缓存命中调用不应报错: %v", err)
	}
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

	results, _ := ch.AnnouncementsAll(context.Background(), true)
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

// TestSiteInfoListFailurePropagates 验证站点清单读失败必须交回错误（JSON errcode / NDJSON 流末尾
// done 行 list_error），不能渲染成「全部站无站免/无公告」假绿。
func TestSiteInfoListFailurePropagates(t *testing.T) {
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if _, e := siteOp.ListActive(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 ListActive 仍成功")
	}
	ch := NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry())

	if _, err := ch.FreeleechAll(context.Background(), false); err == nil {
		t.Error("FreeleechAll：ListActive 失败应交回错误，而不是 nil+nil")
	}
	if _, err := ch.AnnouncementsAll(context.Background(), false); err == nil {
		t.Error("AnnouncementsAll：ListActive 失败应交回错误，而不是 nil+nil")
	}
	flEmitted, annEmitted := 0, 0
	if listErr := ch.FreeleechStream(context.Background(), false, func(FreeleechResult) { flEmitted++ }); listErr == "" {
		t.Error("FreeleechStream：ListActive 失败应返回 listErr，供 done 行带出")
	}
	if listErr := ch.AnnouncementsStream(context.Background(), false, func(AnnouncementResult) { annEmitted++ }); listErr == "" {
		t.Error("AnnouncementsStream：ListActive 失败应返回 listErr，供 done 行带出")
	}
	if flEmitted != 0 || annEmitted != 0 {
		t.Errorf("失败时不应 emit 任何帧，freeleech=%d announcements=%d", flEmitted, annEmitted)
	}
}
