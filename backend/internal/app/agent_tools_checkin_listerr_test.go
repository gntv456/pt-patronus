// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

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
	"gorm.io/gorm"
)

// checkinTool 从注册表里取出 MCP 的 site_checkin_all 工具。
func checkinTool(t *testing.T, siteCh *chain.SiteChain, siteOp *oper.SiteOper) func(context.Context, map[string]any) (string, error) {
	t.Helper()
	for _, tool := range registerAgentTools(siteOp, siteCh, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil) {
		if tool.Name == "site_checkin_all" {
			return tool.Handler
		}
	}
	t.Fatal("site_checkin_all 工具未注册")
	return nil
}

// TestAgentToolCheckinAllReportsListReadError AI 助手调「一键签到」时，
// 站清单读不出来必须在这条出口上说出来。回归：链层咽掉错误 ⇒ 工具回 {"results":[]}，
// 模型据此向用户播报「你没有可签到的站点」——一次数据库故障被翻译成了账户状态。
func TestAgentToolCheckinAllReportsListReadError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/agent.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	siteOp := oper.NewSiteOper(db, security.NewCrypto("agent-seed"))
	siteCh := chain.NewSiteChain(chain.NewBase(module.New(), event.New()), siteOp, nil)

	out, err := checkinTool(t, siteCh, siteOp)(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("工具不该以 Go error 结束（会变成协议层失败），got %v", err)
	}
	if !strings.Contains(out, "list_error") {
		t.Fatalf("读失败必须带 list_error 通道，got %s", out)
	}
	if !strings.Contains(out, "站点清单") {
		t.Fatalf("要说明读不出的是站清单，got %s", out)
	}

	// 正控：清单读得出来时不许冒出 list_error（否则助手每次都以为后端坏了）。
	// 注意成功出口回的是裸数组（mcpJSON(results)），失败出口才是 {results,list_error} 对象。
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	if _, err := siteOp.Create(oper.SiteInput{
		Name: "n", Platform: "nexusphp", BaseURL: "https://n.example", IsActive: true,
	}); err != nil {
		t.Fatalf("正控建站失败: %v", err)
	}
	out2, _ := checkinTool(t, siteCh, siteOp)(context.Background(), map[string]any{})
	if strings.Contains(out2, "list_error") {
		t.Fatalf("清单正常时不该有 list_error，got %s", out2)
	}
	if !strings.Contains(out2, `"site_name":"n"`) {
		t.Fatalf("正控：清单可读时应返回逐站结果，got %s", out2)
	}
}
