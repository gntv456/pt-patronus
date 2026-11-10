// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
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

// fakeHaremAdapter 在 fakeAdapter 基础上加 HaremCapable（FetchInviteStatus + FetchInvitees）；
// 两方法各自 atomic 计数 + 共享 proceed，验证 singleflight 下后宫两步 HTTP 各只调一次。
// panicHost 非空时，baseURL 命中的站在 FetchInviteStatus 现场 panic（per-site 隔离用例）。
type fakeHaremAdapter struct {
	*fakeAdapter
	status        *site.InviteStatus
	invitees      []site.InviteeInfo
	statusCalls   int32
	inviteesCalls int32
	proceed       chan struct{}
	panicHost     string
}

func (f *fakeHaremAdapter) FetchInviteStatus(_ context.Context, _ *site.SiteMetadata, baseURL, _ string) (*site.InviteStatus, error) {
	atomic.AddInt32(&f.statusCalls, 1)
	if f.panicHost != "" && strings.Contains(baseURL, f.panicHost) {
		panic("nil pointer dereference on malformed invite page")
	}
	if f.proceed != nil {
		<-f.proceed
	}
	return f.status, nil
}
func (f *fakeHaremAdapter) FetchInvitees(context.Context, *site.SiteMetadata, string, string) ([]site.InviteeInfo, error) {
	atomic.AddInt32(&f.inviteesCalls, 1)
	if f.proceed != nil {
		<-f.proceed
	}
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
			results[i], _ = ch.HaremAll(context.Background(), false)
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

// TestHaremListFailurePropagates 站点清单读不出：以前 HaremAll 返回 nil → handler 回 200 空数组，
// 与「没配站点」在界面上同形；现在错误必须交回（JSON 报错 / SSE done 帧带 list_error）。
func TestHaremListFailurePropagates(t *testing.T) {
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("harem-list-seed"))
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	// 前提断言：清单必须真的读不出，否则下面测的是别的路径
	if _, e := siteOp.ListActive(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 ListActive 仍成功")
	}

	ch := NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry())
	if _, err := ch.HaremAll(context.Background(), false); err == nil {
		t.Fatal("清单读不出 HaremAll 要交回错误，不能返回空结果")
	}
	emitted := 0
	_, listErr := ch.HaremAllStream(context.Background(), false, func(site.PrescriptionItem) { emitted++ })
	if listErr == "" {
		t.Fatal("流式路径也要交回清单错误文本")
	}
	if emitted != 0 {
		t.Fatalf("清单读不出时不得 emit 任何站，emitted=%d", emitted)
	}
}

// TestHaremUnsupportedSiteSurfacesError 不支持后宫的站以前返回 (nil, nil)，
// 渲染成「剩余 0、权限 ✗、无错误」的假数据；现在必须带错误留痕且不进汇总。
func TestHaremUnsupportedSiteSurfacesError(t *testing.T) {
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("harem-unsup-seed"))
	s, err := siteOp.Create(oper.SiteInput{Name: "u", Platform: "nexusphp", BaseURL: "https://x", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	// fakeAdapter 未实现 HaremCapable → dispatchAdapter 回 ErrNotSupported
	mm := module.New()
	mm.Register(&fakeAdapter{schema: "nexusphp"})
	ch := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())

	if _, err := ch.HaremStatus(context.Background(), s); !errors.Is(err, ErrNotSupported) {
		t.Errorf("HaremStatus 对不支持的站要回 ErrNotSupported（不能是 nil,nil）, got %v", err)
	}

	res, err := ch.HaremAll(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Prescriptions) != 1 {
		t.Fatalf("want 1 prescription, got %+v", res)
	}
	it := res.Prescriptions[0]
	if it.Error != "该站不支持邀请页抓取" {
		t.Errorf("不支持的站要留痕人话文案, got %q", it.Error)
	}
	if res.Overview.TotalSites != 0 {
		t.Errorf("不支持的站不该计入汇总: %+v", res.Overview)
	}
}
