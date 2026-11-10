// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/httpclient"
	"github.com/hxpt/ptpatronus/internal/site"
)

// newMeasuredFixture 起一条真实 sqlite + 假适配器，规则内容由调用方给。
// 适配器总能返回 Ratio=9/Seeding=10，用于区分「真读到了」和「这次没跑」。
func newMeasuredFixture(t *testing.T, rule *models.ContractRule) (*ContractChain, uint) {
	t.Helper()
	db := newChainDB(t)
	if err := db.AutoMigrate(&models.ContractRule{}); err != nil {
		t.Fatal(err)
	}
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	st, err := siteOp.Create(oper.SiteInput{Name: "mz-site", Platform: "nexusphp", BaseURL: "https://mz.pt", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	ad := &fakeAdapter{schema: "nexusphp", ui: &site.UserInfo{
		Name: "u", Ratio: 9, Seeding: 10, SeedingSize: 200 << 30, Uploaded: 500 << 30,
	}}
	mm := module.New()
	mm.Register(ad)
	siteCh := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())
	cOp := oper.NewContractOper(db)
	rule.SiteID = st.ID
	if err := cOp.Upsert(rule); err != nil {
		t.Fatal(err)
	}
	var got models.ContractRule
	if err := db.Where("site_id = ?", st.ID).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	return NewContractChain(NewBase(mm, event.New()), siteCh, siteOp, cOp, nil), got.ID
}

// TestContractCheckDisabledRule 未启用的规则被手动点「检查」：从前返回 Status:"ok"+Detail:"规则未启用"，
// 而界面状态标签读库里的 last_status（仍是空）、detail 又不渲染，卡片只剩一行
// 「实时：分享率 0.00 · 做种 0 · 上传 0.0GiB」——一个站点都没访问，却画出四个可信读数。
func TestContractCheckDisabledRule(t *testing.T) {
	c, ruleID := newMeasuredFixture(t, &models.ContractRule{Enabled: false, MinRatio: 1})
	res, err := c.Check(context.Background(), ruleID)
	if err == nil || !errors.Is(err, ErrRuleDisabled) {
		t.Fatalf("未启用必须报错，got res=%+v err=%v", res, err)
	}
	if res != nil {
		t.Errorf("未启用不得返回检查结果（否则界面照画读数），got %+v", res)
	}
	if !strings.Contains(err.Error(), "未启用") {
		t.Errorf("err = %q, want 说清「未启用」这个根因", err)
	}
}

// TestContractCheckNotYetEffective 生效时间没到：判定确实没跑，measured 必须为 false，
// 否则零值字段会以「实时读数」的身份出现在界面上。
func TestContractCheckNotYetEffective(t *testing.T) {
	start := time.Now().Add(2 * time.Hour)
	c, ruleID := newMeasuredFixture(t, &models.ContractRule{Enabled: true, MinRatio: 1, StartTime: &start})
	res, err := c.Check(context.Background(), ruleID)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Measured {
		t.Errorf("没到生效时间不该算取到数，got %+v", res)
	}
	if res.Detail == "" {
		t.Errorf("measured=false 时必须给出原因（界面靠它替代读数）")
	}
}

// TestContractCheckMarksMeasuredOnlyAfterReading 正控：真读到 UserInfo 才 measured=true，
// 别把闸门做成永远关（那样界面就再也没法显示实时值）。
func TestContractCheckMarksMeasuredOnlyAfterReading(t *testing.T) {
	c, ruleID := newMeasuredFixture(t, &models.ContractRule{Enabled: true, MinRatio: 1})
	res, err := c.Check(context.Background(), ruleID)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !res.Measured {
		t.Fatalf("已读到站点数据应 measured=true，got %+v", res)
	}
	if res.Ratio == 0 || res.Seeding == 0 {
		t.Errorf("读数应来自适配器，got ratio=%v seeding=%v", res.Ratio, res.Seeding)
	}
}

// TestContractOfficialDetailFailureUsesChineseText 官种明细拉取失败的那句「无法核验」
// 会直接落进 last_detail 显示在契约卡片上，站点侧错误要过 errtext（SSRF/超时/未登录翻中文）。
func TestContractOfficialDetailFailureUsesChineseText(t *testing.T) {
	db := newChainDB(t)
	if err := db.AutoMigrate(&models.ContractRule{}); err != nil {
		t.Fatal(err)
	}
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	st, err := siteOp.Create(oper.SiteInput{Name: "gz-cn", Platform: "nexusphp", BaseURL: "https://gz-cn.pt", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	ad := &fakeSeedingAdapter{
		fakeAdapter: &fakeAdapter{schema: "nexusphp", ui: &site.UserInfo{Name: "u", Ratio: 9, Seeding: 10}},
		serr:        fmt.Errorf("fetch seeding list: %w", httpclient.ErrBlocked),
	}
	mm := module.New()
	mm.Register(ad)
	siteCh := NewSiteChain(NewBase(mm, event.New()), siteOp, site.NewRegistry())
	cOp := oper.NewContractOper(db)
	if err := cOp.Upsert(&models.ContractRule{SiteID: st.ID, Enabled: true, OfficialOnly: true, MinSeeding: 5}); err != nil {
		t.Fatal(err)
	}
	var rule models.ContractRule
	if err := db.Where("site_id = ?", st.ID).First(&rule).Error; err != nil {
		t.Fatal(err)
	}
	res, err := NewContractChain(NewBase(mm, event.New()), siteCh, siteOp, cOp, nil).Check(context.Background(), rule.ID)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("官种维度读不到应报 error，got %q", res.Status)
	}
	if strings.Contains(res.Detail, "blocked private/internal") {
		t.Errorf("last_detail 不得带英文原文给用户，got %q", res.Detail)
	}
	if !strings.Contains(res.Detail, "SSRF") && !strings.Contains(res.Detail, "内网") {
		t.Errorf("Detail = %q, want 中文的 SSRF 提示", res.Detail)
	}
}
