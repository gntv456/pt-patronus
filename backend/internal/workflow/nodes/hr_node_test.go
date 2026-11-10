// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package nodes

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
	"github.com/hxpt/ptpatronus/internal/workflow"
	"gorm.io/gorm"
)

// newHRSiteChain 造一条带一个站点的 SiteChain；hrEnabled 决定站点定义里是否开 hr.enabled。
func newHRSiteChain(t *testing.T, hrEnabled bool) (*chain.SiteChain, *oper.SiteOper, uint) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/hrnode.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	siteOp := oper.NewSiteOper(db, security.NewCrypto("hr-node-test-key"))
	def := &site.SiteMetadata{ID: "hrnode", Name: "HR 节点站", Schema: "nexusphp"}
	def.HR.Enabled = hrEnabled
	reg := site.NewRegistry()
	reg.AddDefinition(def)
	s, err := siteOp.Create(oper.SiteInput{
		Name: "站点", Platform: "nexusphp", Definition: "hrnode", BaseURL: "https://hr.example", IsActive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return chain.NewSiteChain(chain.NewBase(module.New(), event.New()), siteOp, reg), siteOp, s.ID
}

// TestCheckHRNodeUnwiredIsError 回归：站点定义没开 hr.enabled 时 chain.HR 返回 (空, nil)，
// 节点曾据此写 hr_count=0 并播报「H&R：站点 → 0 条」——下游条件会把「从没查过」当成
// 「查过且没有 H&R」，封号命脉的检查在模板里静默失效。
func TestCheckHRNodeUnwiredIsError(t *testing.T) {
	siteCh, siteOp, id := newHRSiteChain(t, false)
	ecx := &workflow.Context{Variables: map[string]any{"hr_count": 42}}
	err := NewCheckHRNode(siteCh, siteOp).Execute(context.Background(), map[string]any{"site_id": float64(id)}, ecx)
	if err == nil {
		t.Fatalf("未开启 H&R 检测必须报错，got 消息=%v", ecx.Messages)
	}
	if !strings.Contains(err.Error(), "未开启 H&R 检测") {
		t.Errorf("错误要说明是没开启检测，got %v", err)
	}
	if len(ecx.Messages) != 0 {
		t.Errorf("报错时不得再播报假结果，got %v", ecx.Messages)
	}
	if ecx.Variables["hr_count"] != 42 {
		t.Errorf("报错时不得覆写上游变量，got %v", ecx.Variables["hr_count"])
	}
}

// TestCheckHRNodeWiredRunsFetch 接线后仍照常走抓取（无适配器时按抓取失败报错，而不是「0 条」）。
func TestCheckHRNodeWiredRunsFetch(t *testing.T) {
	siteCh, siteOp, id := newHRSiteChain(t, true)
	ecx := &workflow.Context{Variables: map[string]any{}}
	err := NewCheckHRNode(siteCh, siteOp).Execute(context.Background(), map[string]any{"site_id": float64(id)}, ecx)
	if err == nil {
		t.Fatalf("没注册适配器时抓取应当失败，got 消息=%v", ecx.Messages)
	}
	if strings.Contains(err.Error(), "未开启 H&R 检测") {
		t.Errorf("已接线的站不该被判成未开启，got %v", err)
	}
}
