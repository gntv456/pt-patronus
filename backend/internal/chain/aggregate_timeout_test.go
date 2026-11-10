// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/site/assessment"
)

// blockingAssessAdapter 阻塞到 ctx 结束再返回 ctx.Err()，模拟被聚合限时截断的慢站。
type blockingAssessAdapter struct {
	block bool
	err   error
	calls int32
}

func (b *blockingAssessAdapter) Platform() string { return "nexusphp" }
func (b *blockingAssessAdapter) Supported() bool  { return true }
func (b *blockingAssessAdapter) FetchAssessments(ctx context.Context, _ assessment.HTTPDoer, _ assessment.Def, _, _, _ string) ([]assessment.Task, error) {
	atomic.AddInt32(&b.calls, 1)
	if b.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return nil, b.err
}

func newAssessTimeoutChain(t *testing.T, adp *blockingAssessAdapter) *AssessmentChain {
	t.Helper()
	db := newAssessDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	for i := 0; i < 2; i++ {
		if _, err := siteOp.Create(oper.SiteInput{
			Name: fmt.Sprintf("s%d", i), Platform: "nexusphp",
			BaseURL: fmt.Sprintf("https://x%d", i), IsActive: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	reg := assessment.NewRegistry()
	reg.Register(adp)
	return NewAssessmentChain(
		NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry()),
		reg, oper.NewAssessmentOper(db), nil, nil)
}

// TestAssessAggregateTruncationFriendlyText 聚合限时截断（15s 到点）时卡片错误必须是
// 用户可读文案（此前直出 "context canceled"，用户无从判断）；真实错误不得被文案掩盖。
func TestAssessAggregateTruncationFriendlyText(t *testing.T) {
	// 截断：ctx 150ms 到点 → 卡片 UNKNOWN + 人话文案
	adp := &blockingAssessAdapter{block: true}
	ch := newAssessTimeoutChain(t, adp)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	cards, err := ch.AssessAll(ctx, false)
	if err != nil {
		t.Fatalf("聚合限时截断不该让整体报错: %v", err)
	}
	if atomic.LoadInt32(&adp.calls) == 0 {
		t.Fatal("适配器未被调用，用例失效")
	}
	if len(cards) != 2 {
		t.Fatalf("want 2 cards, got %d: %+v", len(cards), cards)
	}
	for _, c := range cards {
		if c.Status != "UNKNOWN" {
			t.Errorf("截断站点应为 UNKNOWN, got %s", c.Status)
		}
		if !strings.Contains(c.Error, "抓取超时") || strings.Contains(c.Error, "context") {
			t.Errorf("截断应给人话文案，got %q", c.Error)
		}
	}

	// 正控：ctx 存活 + 真实错误 → 原样透出（友好文案不得误伤真实错误）
	adp2 := &blockingAssessAdapter{err: errors.New("site boom")}
	ch2 := newAssessTimeoutChain(t, adp2)
	cards2, err := ch2.AssessAll(context.Background(), false)
	if err != nil {
		t.Fatalf("聚合内单站失败不该让整体报错: %v", err)
	}
	for _, c := range cards2 {
		if c.Error != "site boom" {
			t.Errorf("真实错误应原样透出, got %q", c.Error)
		}
	}
}

// blockingHaremAdapter 阻塞到 ctx 结束再返回 ctx.Err() 的站点适配器（Harem 能力）。
type blockingHaremAdapter struct {
	block bool
	calls int32
}

func (b *blockingHaremAdapter) Name() string   { return "blocking-harem" }
func (b *blockingHaremAdapter) Priority() int  { return 100 }
func (b *blockingHaremAdapter) Schema() string { return "nexusphp" }
func (b *blockingHaremAdapter) Search(context.Context, *site.SiteMetadata, string, string, string, string, int) ([]site.Torrent, error) {
	return nil, nil
}
func (b *blockingHaremAdapter) FetchUserInfo(context.Context, *site.SiteMetadata, string, string) (*site.UserInfo, error) {
	return nil, nil
}
func (b *blockingHaremAdapter) FetchInviteStatus(ctx context.Context, _ *site.SiteMetadata, _, _ string) (*site.InviteStatus, error) {
	atomic.AddInt32(&b.calls, 1)
	if b.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &site.InviteStatus{}, nil
}
func (b *blockingHaremAdapter) FetchInvitees(ctx context.Context, _ *site.SiteMetadata, _, _ string) ([]site.InviteeInfo, error) {
	if b.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return nil, nil
}

// TestHaremAggregateTruncationFriendlyText 后宫聚合截断同样给人话文案（药单导出/详情页可见）。
func TestHaremAggregateTruncationFriendlyText(t *testing.T) {
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	if _, err := siteOp.Create(oper.SiteInput{Name: "h", Platform: "nexusphp", BaseURL: "https://x", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	adp := &blockingHaremAdapter{block: true}
	mm := module.New()
	mm.Register(adp)
	siteCh := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	res, err := siteCh.HaremAll(ctx, false)
	if err != nil {
		t.Fatalf("聚合内单站失败不该让整体报错: %v", err)
	}
	if atomic.LoadInt32(&adp.calls) == 0 {
		t.Fatal("适配器未被调用（Harem 未启用或分发失败），用例失效")
	}
	if res == nil || len(res.Prescriptions) != 1 {
		t.Fatalf("want 1 prescription, got %+v", res)
	}
	if it := res.Prescriptions[0]; !strings.Contains(it.Error, "抓取超时") || strings.Contains(it.Error, "context") {
		t.Errorf("截断应给人话文案，got %q", it.Error)
	}
}

// TestCtxErrText 单元级：截断→人话；ctx 存活+真实错误→原样透出（含包裹形态）。
func TestCtxErrText(t *testing.T) {
	c1, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	<-c1.Done()
	if got := ctxErrText(c1, context.DeadlineExceeded); !strings.Contains(got, "抓取超时") {
		t.Errorf("DeadlineExceeded 应给人话, got %q", got)
	}
	if got := ctxErrText(c1, fmt.Errorf("get torrents: %w", context.Canceled)); !strings.Contains(got, "抓取超时") {
		t.Errorf("包裹的 Canceled 应给人话, got %q", got)
	}
	if got := ctxErrText(context.Background(), errors.New("boom")); got != "boom" {
		t.Errorf("ctx 存活时真实错误应原样透出, got %q", got)
	}
}

// TestAssessListFailurePropagates 验证站点清单读失败必须交回错误（JSON 端点回 errcode），
// 不能渲染成「0 站考核」假绿。
func TestAssessListFailurePropagates(t *testing.T) {
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if _, e := siteOp.ListActive(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 ListActive 仍成功")
	}
	ch := NewAssessmentChain(
		NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry()),
		assessment.NewRegistry(), oper.NewAssessmentOper(newAssessDB(t)), nil, nil)

	if _, err := ch.AssessAll(context.Background(), false); err == nil {
		t.Error("AssessAll：ListActive 失败应交回错误，而不是 nil+nil")
	}
}
