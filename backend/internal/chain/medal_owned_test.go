// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/site/medalwall"
)

// ownedGuardSetup 构造「商店返回 m1/m2 均未标 OWNED、旧快照 m1=OWNED 佩戴中」的场景。
// ownedErr/owned 由调用方指定（回源失败 vs 成功空列表）。
func ownedGuardSetup(t *testing.T, ownedErr error) (*MedalChain, *fakeMedalAdapter, *oper.MedalOper, *models.Site) {
	t.Helper()
	db := newMedalDB(t)
	siteOp := oper.NewSiteOper(newChainDB(t), security.NewCrypto("seed"))
	s, _ := siteOp.Create(oper.SiteInput{Name: "t", Platform: "nexusphp", BaseURL: "https://owned-guard", IsActive: true})
	medalOp := oper.NewMedalOper(db)

	_ = medalOp.BatchUpsert(s.ID, []models.Medal{
		{MedalID: "m1", Name: "勋章A", Price: 10, Currency: "魔力", PurchaseStatus: string(medalwall.StatusOwned), WearStatus: true, Stock: -1},
		{MedalID: "m2", Name: "勋章B", Price: 20, Currency: "魔力", PurchaseStatus: string(medalwall.StatusAvailable), Stock: -1},
	}, time.Now().Add(-49*time.Hour))

	src := &fakeMedalAdapter{
		shop: []medalwall.Item{
			{MedalID: "m1", Name: "勋章A", Price: 10, Currency: "魔力", Stock: -1}, // 商店页未标注已拥有
			{MedalID: "m2", Name: "勋章B", Price: 20, Currency: "魔力", Stock: -1},
		},
		ownedErr: ownedErr,
		proceed:  make(chan struct{}),
	}
	close(src.proceed)
	reg := medalwall.NewRegistry()
	reg.Register(src)
	siteCh := NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry())
	ch := newMedalChain(siteCh, reg, medalOp, nil)
	return ch, src, medalOp, s
}

// TestFetchMedalSiteKeepsOwnedOnOwnedFetchFailure 商店成功 + 已拥有列表回源失败时，
// 旧 OWNED/佩戴状态必须保留（BatchUpsert 删后插，否则一次瞬时故障把已拥有勋章洗成可购买）。
func TestFetchMedalSiteKeepsOwnedOnOwnedFetchFailure(t *testing.T) {
	ch, src, medalOp, s := ownedGuardSetup(t, errors.New("已拥有列表请求失败: HTTP 503"))
	items, errMsg := ch.fetchMedalSite(context.Background(), nil, s, ch.siteCh.defFor(s), src)
	if errMsg != "" {
		t.Fatalf("商店成功不应带错误消息, got %q", errMsg)
	}
	byID := map[string]medalwall.Item{}
	for _, it := range items {
		byID[it.MedalID] = it
	}
	if got := byID["m1"]; got.PurchaseStatus != medalwall.StatusOwned || !got.WearStatus {
		t.Errorf("回源失败时 m1 应保留 OWNED+佩戴中, got %s wear=%v", got.PurchaseStatus, got.WearStatus)
	}
	if got := byID["m2"]; got.PurchaseStatus == medalwall.StatusOwned {
		t.Errorf("m2 从未拥有，不应被回填为 OWNED, got %s", got.PurchaseStatus)
	}
	stored := map[string]models.Medal{}
	ms, _ := medalOp.FindBySite(s.ID)
	for _, m := range ms {
		stored[m.MedalID] = m
	}
	if stored["m1"].PurchaseStatus != string(medalwall.StatusOwned) || !stored["m1"].WearStatus {
		t.Errorf("DB 中 m1 = %s wear=%v，应保留 OWNED+佩戴", stored["m1"].PurchaseStatus, stored["m1"].WearStatus)
	}
}

// 正控：已拥有列表「回源成功但为空」时必须洗掉旧 OWNED（真实结果优先）——
// 证明上一条测的保留行为只在失败时生效，且其断言可判别。
func TestFetchMedalSiteTrustsEmptyOwnedWhenFetchSucceeds(t *testing.T) {
	ch, src, medalOp, s := ownedGuardSetup(t, nil) // owned=nil 且无错误 = 成功的空列表
	items, errMsg := ch.fetchMedalSite(context.Background(), nil, s, ch.siteCh.defFor(s), src)
	if errMsg != "" {
		t.Fatalf("商店成功不应带错误消息, got %q", errMsg)
	}
	byID := map[string]medalwall.Item{}
	for _, it := range items {
		byID[it.MedalID] = it
	}
	if got := byID["m1"]; got.PurchaseStatus == medalwall.StatusOwned {
		t.Errorf("已拥有列表成功为空时 m1 不应仍是 OWNED, got %s", got.PurchaseStatus)
	}
	stored := map[string]models.Medal{}
	ms, _ := medalOp.FindBySite(s.ID)
	for _, m := range ms {
		stored[m.MedalID] = m
	}
	if stored["m1"].PurchaseStatus == string(medalwall.StatusOwned) {
		t.Errorf("DB 中 m1 应被真实结果覆盖, got OWNED")
	}
}
