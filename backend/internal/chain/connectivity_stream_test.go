// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// TestConnectionsStream 验证流式逐站检测：每站完成即回调 emit，停用站直接 disabled（不发请求），
// 启用站 connected/disconnected，结果 upsert 进 site_connectivity 缓存（含 CheckedAt）。
func TestConnectionsStream(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/sc.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Site{}, &models.SiteConnectivity{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	sOK, _ := siteOp.Create(oper.SiteInput{Name: "ok", Platform: "nexusphp", BaseURL: "https://ok", IsActive: true})
	sFail, _ := siteOp.Create(oper.SiteInput{Name: "fail", Platform: "dead", BaseURL: "https://fail", IsActive: true}) // 无 adapter → ErrNoAdapter → disconnected
	sOff, _ := siteOp.Create(oper.SiteInput{Name: "off", Platform: "nexusphp", BaseURL: "https://off", IsActive: false})

	mm := module.New()
	mm.Register(&fakeAdapter{schema: "nexusphp", ui: &site.UserInfo{Level: "VIP"}})
	ch := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())

	// emit 由多站 goroutine 并发回调（链的契约：调用方自行串行化）——无锁 append 会丢帧。
	var mu sync.Mutex
	var got []SiteConnectionResult
	ch.TestConnectionsStream(context.Background(), func(r SiteConnectionResult) {
		mu.Lock()
		got = append(got, r)
		mu.Unlock()
	})

	if len(got) != 3 {
		t.Fatalf("emit count=%d, want 3", len(got))
	}
	byID := make(map[uint]SiteConnectionResult, len(got))
	for _, r := range got {
		byID[r.SiteID] = r
	}
	if r := byID[sOK.ID]; r.Status != "connected" || r.Level != "VIP" || r.CheckedAt.IsZero() {
		t.Errorf("ok site = %+v, want connected/VIP/nonzero CheckedAt", r)
	}
	if r := byID[sFail.ID]; r.Status != "disconnected" || r.Error == "" {
		t.Errorf("fail site = %+v, want disconnected with error", r)
	}
	if r := byID[sOff.ID]; r.Status != "disabled" {
		t.Errorf("off site = %+v, want disabled", r)
	}

	// 逐站结果已 upsert 进 site_connectivity 缓存（供进页面瞬时回显）
	var cnt int64
	if err := db.Model(&models.SiteConnectivity{}).Count(&cnt).Error; err != nil {
		t.Fatal(err)
	}
	if cnt != 3 {
		t.Errorf("site_connectivity rows=%d, want 3", cnt)
	}

	// GET 缓存（ListConnectivity）回读应与检测结果一致
	list, err := siteOp.ListConnectivity()
	if err != nil {
		t.Fatal(err)
	}
	got2 := make(map[uint]models.SiteConnectivity, len(list))
	for _, c := range list {
		got2[c.SiteID] = c
	}
	if got2[sOK.ID].Status != "connected" || got2[sFail.ID].Status != "disconnected" || got2[sOff.ID].Status != "disabled" {
		t.Errorf("ListConnectivity mismatch: %+v", got2)
	}
	// 统计累加：connected → Success++（且 secs>0 更新 AvgSeconds）；disconnected → Fail++；disabled 不计
	if got2[sOK.ID].Success < 1 {
		t.Errorf("ok site Success=%d, want >=1", got2[sOK.ID].Success)
	}
	if got2[sFail.ID].Fail < 1 {
		t.Errorf("fail site Fail=%d, want >=1", got2[sFail.ID].Fail)
	}
	if got2[sOff.ID].Success != 0 || got2[sOff.ID].Fail != 0 {
		t.Errorf("disabled site should not count: Success=%d Fail=%d", got2[sOff.ID].Success, got2[sOff.ID].Fail)
	}

	// AvgSeconds 滑动平均：直测 upsert（绕过 TestConnection 在瞬时 fakeAdapter + Windows 低精度时钟下 secs 可能为 0 的不确定）。
	// 两次成功(1.0s, 3.0s) → avg=(1.0×1+3.0)/2=2.0；Success=2。生产环境 UserInfo 是网络请求，secs 不会为 0。
	sStat, _ := siteOp.Create(oper.SiteInput{Name: "stat", Platform: "nexusphp", BaseURL: "https://stat", IsActive: true})
	_ = siteOp.UpsertConnectivity(sStat.ID, "connected", "L", "", 1.0)
	_ = siteOp.UpsertConnectivity(sStat.ID, "connected", "L", "", 3.0)
	var sc models.SiteConnectivity
	if err := db.First(&sc, "site_id = ?", sStat.ID).Error; err != nil {
		t.Fatal(err)
	}
	if sc.Success != 2 || sc.AvgSeconds < 1.9 || sc.AvgSeconds > 2.1 {
		t.Errorf("avg stats: Success=%d AvgSeconds=%v, want 2 / ~2.0", sc.Success, sc.AvgSeconds)
	}
}
