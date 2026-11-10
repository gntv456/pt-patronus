// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/site/medalwall"
)

// newPanicMedalChain 构造两站场景：OK 站正常返回商店数据，崩站在 FetchShop 内 panic
// （BaseURL 命中 fakeMedalAdapter.panicOnURL）。返回聚合链与两站 ID。
func newPanicMedalChain(t *testing.T) (*MedalChain, uint, uint) {
	t.Helper()
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	okSite, err := siteOp.Create(oper.SiteInput{Name: "OK站", Platform: "nexusphp", BaseURL: "https://ok.example", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	boomSite, err := siteOp.Create(oper.SiteInput{Name: "崩站", Platform: "nexusphp", BaseURL: "https://boom.example", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	src := &fakeMedalAdapter{
		shop:       []medalwall.Item{{MedalID: "m1", Name: "勋章A", Price: 10}},
		panicOnURL: "https://boom.example",
	}
	reg := medalwall.NewRegistry()
	reg.Register(src)
	medalOp := oper.NewMedalOper(newMedalDB(t))
	siteCh := NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry())
	return newMedalChain(siteCh, reg, medalOp, nil), okSite.ID, boomSite.ID
}

// TestMedalAllPanickedSiteYieldsErrorCard 接线验证：单站 FetchShop panic 必须变成该站的
// 错误卡（JSON 面）。以前 panic 走 safego 兜底只留日志，槽位保持零值 supported=false →
// 该站一卡不出，与「未开启抓取」同形静默缺席。
func TestMedalAllPanickedSiteYieldsErrorCard(t *testing.T) {
	ch, okID, boomID := newPanicMedalChain(t)
	cards, err := ch.MedalAll(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 2 {
		t.Fatalf("期望 2 张卡（OK 站勋章 + 崩站错误卡），got %d: %+v", len(cards), cards)
	}
	var okCard, boomCard *MedalCard
	for i := range cards {
		switch cards[i].SiteID {
		case okID:
			okCard = &cards[i]
		case boomID:
			boomCard = &cards[i]
		}
	}
	if okCard == nil || okCard.MedalID != "m1" || okCard.Error != "" {
		t.Fatalf("正常站勋章卡缺失或异常: %+v", cards)
	}
	if boomCard == nil {
		t.Fatalf("崩站必须出一张错误卡（否则与未开启抓取同形静默缺席）: %+v", cards)
	}
	if !strings.Contains(boomCard.Error, "panic:") {
		t.Fatalf("崩站错误卡必须带 panic 文本, got %q", boomCard.Error)
	}
}

// TestMedalAllStreamPanickedSiteYieldsErrorFrame 接线验证：SSE 面单站 panic 必须推出一帧
// 错误卡。以前 panic → 零帧，前端与「该站没有勋章」同形。
func TestMedalAllStreamPanickedSiteYieldsErrorFrame(t *testing.T) {
	ch, okID, boomID := newPanicMedalChain(t)
	var mu sync.Mutex
	var frames [][]MedalCard
	listErr := ch.MedalAllStream(context.Background(), true, func(cards []MedalCard) {
		mu.Lock()
		frames = append(frames, cards)
		mu.Unlock()
	})
	if listErr != "" {
		t.Fatal("listErr:", listErr)
	}
	var okSeen, boomSeen bool
	for _, f := range frames {
		for _, c := range f {
			if c.SiteID == okID && c.MedalID == "m1" && c.Error == "" {
				okSeen = true
			}
			if c.SiteID == boomID && strings.Contains(c.Error, "panic:") {
				boomSeen = true
			}
		}
	}
	if !okSeen {
		t.Fatalf("正常站勋章帧缺失: %+v", frames)
	}
	if !boomSeen {
		t.Fatalf("崩站必须出错误帧（零帧=静默缺席）: %+v", frames)
	}
}
