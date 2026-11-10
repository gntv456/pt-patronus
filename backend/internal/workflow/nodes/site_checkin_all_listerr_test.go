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
	"github.com/hxpt/ptpatronus/internal/workflow"
	"gorm.io/gorm"
)

// brokenChain 造一个「站点清单读不出来」的 SiteChain：库里没有 sites 表。
func brokenChain(t *testing.T, migrate bool) (*chain.SiteChain, *oper.SiteOper) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/wf.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if migrate {
		if err := db.AutoMigrate(&models.Site{}); err != nil {
			t.Fatal(err)
		}
	}
	siteOp := oper.NewSiteOper(db, security.NewCrypto("wf-seed"))
	return chain.NewSiteChain(chain.NewBase(module.New(), event.New()), siteOp, nil), siteOp
}

// TestCheckinAllNodeFailsOnListReadError 工作流的「全站签到」步骤读不出站清单时必须报错。
// 回归：链层咽掉错误 → 节点照旧播报「全站签到：0/0 成功」，自动化把这次失败当成正常完成，
// 运行记录里全绿，用户永远不会发现那天的签到一站都没签。
func TestCheckinAllNodeFailsOnListReadError(t *testing.T) {
	ch, siteOp := brokenChain(t, false)
	ecx := &workflow.Context{Variables: map[string]any{}}

	err := NewCheckinAllNode(ch, siteOp).Execute(context.Background(), map[string]any{}, ecx)
	if err == nil {
		t.Fatalf("清单读失败必须让节点失败，got err=nil messages=%v", ecx.Messages)
	}
	if !strings.Contains(err.Error(), "全站签到失败") {
		t.Errorf("错误要说明是哪一步失败，got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "站点清单") {
		t.Errorf("错误要带上「清单读不出」的原因，got %q", err.Error())
	}
	for _, m := range ecx.Messages {
		if strings.Contains(m, "成功") {
			t.Errorf("读失败时不许留任何「成功」运行日志，got %q", m)
		}
	}

	// 正控：清单读得出来（只是没有启用站）时仍按原样播报 0/0——两种情况不能混。
	okCh, okOp := brokenChain(t, true)
	ecx2 := &workflow.Context{Variables: map[string]any{}}
	if err := NewCheckinAllNode(okCh, okOp).Execute(context.Background(), map[string]any{}, ecx2); err != nil {
		t.Fatalf("正控：空清单不是失败, got %v", err)
	}
	if len(ecx2.Messages) != 1 || !strings.Contains(ecx2.Messages[0], "0/0 成功") {
		t.Fatalf("正控：0 个启用站应播报 0/0，got %v", ecx2.Messages)
	}
}
