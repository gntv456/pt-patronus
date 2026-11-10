// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// fakeSeedingAdapter 在 fakeAdapter 表面上补一个可注入错误/结果做种明细的 FetchSeedingList。
type fakeSeedingAdapter struct {
	*fakeAdapter
	items []site.SeedingItem
	serr  error
}

func (f *fakeSeedingAdapter) FetchSeedingList(context.Context, *site.SiteMetadata, string, string) ([]site.SeedingItem, error) {
	if f.serr != nil {
		return nil, f.serr
	}
	return f.items, nil
}

// newContractFixture 起一条真实 sqlite + 假适配器：全站做种 10 个（体积充足），
// 官种数量由 items 决定，明细接口可用 serr 模拟瞬时失败。
func newContractFixture(t *testing.T, items []site.SeedingItem, serr error) (*ContractChain, uint, *gorm.DB) {
	t.Helper()
	db := newChainDB(t)
	if err := db.AutoMigrate(&models.ContractRule{}); err != nil {
		t.Fatal(err)
	}
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	st, err := siteOp.Create(oper.SiteInput{Name: "gz-site", Platform: "nexusphp", BaseURL: "https://gz.pt", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	ad := &fakeSeedingAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp", ui: &site.UserInfo{
			Name: "u", Ratio: 9, Seeding: 10, SeedingSize: 200 << 30, Uploaded: 500 << 30,
		}},
		items: items, serr: serr,
	}
	mm := module.New()
	mm.Register(ad)
	siteCh := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())
	cOp := oper.NewContractOper(db)
	c := NewContractChain(NewBase(mm, event.New()), siteCh, siteOp, cOp, nil)

	// 官种契约：要求至少保住 5 个做种（官种维度）。
	if err := cOp.Upsert(&models.ContractRule{
		SiteID: st.ID, Enabled: true, OfficialOnly: true, MinSeeding: 5,
	}); err != nil {
		t.Fatal(err)
	}
	var rule models.ContractRule
	if err := db.Where("site_id = ?", st.ID).First(&rule).Error; err != nil {
		t.Fatal(err)
	}
	return c, rule.ID, db
}

func officialItems(n int) []site.SeedingItem {
	out := make([]site.SeedingItem, 0, n+3)
	for i := 0; i < n; i++ {
		out = append(out, site.SeedingItem{Title: "官方.电影.2024", Size: 10 << 30})
	}
	for i := 0; i < 3; i++ {
		out = append(out, site.SeedingItem{Title: "普通做种", Size: 1 << 30})
	}
	return out
}

// TestContractOfficialRuleFailsWhenListUnavailable 回归：官种契约在做种明细「瞬时拉取失败」时
// 沿用全量做种数判定——全站 10 个做种 ≥ 5 于是记成达标，而真正要保的官种数量根本没读到。
// 判不出来就必须说「无法核验」，不能说达标（违约被吞 = 站点端警告/降级时才暴露）。
func TestContractOfficialRuleFailsWhenListUnavailable(t *testing.T) {
	c, ruleID, db := newContractFixture(t, nil, errors.New("read tcp: connection reset by peer"))
	res, err := c.Check(context.Background(), ruleID)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("明细拉不到时不得判成 %s（官种维度压根没读到），got detail=%q", res.Status, res.Detail)
	}
	if res.Detail == "" || !strings.Contains(res.Detail, "无法核验") {
		t.Errorf("Detail 应说明无法核验，got %q", res.Detail)
	}

	// 界面上的状态标签/详情读的是规则行里落库的 last_status/last_detail，
	// 只改内存结果不改库等于用户永远看不到这次「核验不了」。
	var rule models.ContractRule
	if err := db.First(&rule, ruleID).Error; err != nil {
		t.Fatal(err)
	}
	if rule.LastStatus != "error" || !strings.Contains(rule.LastDetail, "无法核验") {
		t.Errorf("无法核验必须落库（UI 读的是 last_status/last_detail），got %q / %q", rule.LastStatus, rule.LastDetail)
	}
}

// TestContractOfficialRuleBreachesOnFewOfficials 正控 A：明细能读到但官种只有 2 个 → 违约。
func TestContractOfficialRuleBreachesOnFewOfficials(t *testing.T) {
	c, ruleID, _ := newContractFixture(t, officialItems(2), nil)
	res, err := c.Check(context.Background(), ruleID)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Status != "breach" {
		t.Fatalf("官种 2 < 下限 5 应违约，got status=%s detail=%q", res.Status, res.Detail)
	}
	if res.SeedingOfficial != 2 {
		t.Errorf("应回报官种数 2，got %d", res.SeedingOfficial)
	}
}

// TestContractOfficialRulePassesOnEnoughOfficials 正控 B：官种 5 个 → 达标（闸门不误伤）。
func TestContractOfficialRulePassesOnEnoughOfficials(t *testing.T) {
	c, ruleID, _ := newContractFixture(t, officialItems(5), nil)
	res, err := c.Check(context.Background(), ruleID)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("官种够数应达标，got status=%s detail=%q", res.Status, res.Detail)
	}
}

// TestContractSizeOnlyRuleKeepsUiFallback 划界：非官种契约（只设体积下限）在明细失败时
// 仍按 ui 汇总基数判定并放行——体积是同一口径的近似，不能一并改成 error（会把可用结果也掐掉）。
func TestContractSizeOnlyRuleKeepsUiFallback(t *testing.T) {
	db := newChainDB(t)
	if err := db.AutoMigrate(&models.ContractRule{}); err != nil {
		t.Fatal(err)
	}
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	st, err := siteOp.Create(oper.SiteInput{Name: "size-site", Platform: "nexusphp", BaseURL: "https://size.pt", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	ad := &fakeSeedingAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp", ui: &site.UserInfo{
			Name: "u", Ratio: 9, Seeding: 10, SeedingSize: 200 << 30,
		}},
		serr: errors.New("read tcp: connection reset by peer"),
	}
	mm := module.New()
	mm.Register(ad)
	siteCh := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())
	cOp := oper.NewContractOper(db)
	c := NewContractChain(NewBase(mm, event.New()), siteCh, siteOp, cOp, nil)
	if err := cOp.Upsert(&models.ContractRule{SiteID: st.ID, Enabled: true, MinSeedingSizeGB: 100}); err != nil {
		t.Fatal(err)
	}
	var rule models.ContractRule
	if err := db.Where("site_id = ?", st.ID).First(&rule).Error; err != nil {
		t.Fatal(err)
	}
	res, err := c.Check(context.Background(), rule.ID)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("仅体积下限的规则应沿用全量基数并达标，got status=%s detail=%q", res.Status, res.Detail)
	}
}
