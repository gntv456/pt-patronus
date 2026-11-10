// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/mcp"
	"gorm.io/gorm"
)

func openBareDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/sysinfo.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// sysDeps 造 system.info 用到的三个清单依赖（清单本身走真 SQL，不用替身）。
func sysDeps(t *testing.T, db *gorm.DB) (*oper.SiteOper, *chain.SiteChain, *chain.SubscribeChain, *oper.DownloaderOper) {
	t.Helper()
	crypto := security.NewCrypto("sysinfo-seed")
	siteOp := oper.NewSiteOper(db, crypto)
	dlOp := oper.NewDownloaderOper(db, crypto)
	siteCh := chain.NewSiteChain(chain.NewBase(module.New(), event.New()), siteOp, nil)
	subCh := chain.NewSubscribeChain(siteCh, nil, nil, oper.NewSubscriptionOper(db), nil, nil, nil, nil)
	return siteOp, siteCh, subCh, dlOp
}

// assertUnreadableCounts 三个数量必须是 null + 原因，不许是 0。
func assertUnreadableCounts(t *testing.T, out map[string]any) {
	t.Helper()
	for _, key := range []string{"sites", "subscriptions", "downloaders"} {
		v, ok := out[key]
		if !ok {
			t.Fatalf("%s 键必须存在（缺键同样会被助手读成 0）：%v", key, out)
		}
		if v != nil {
			t.Errorf("%s 读不出来时必须是 null，got %#v", key, v)
		}
		if reason, _ := out[key+"_error"].(string); reason == "" {
			t.Errorf("%s 要留下读不出来的原因，got %v", key, out[key+"_error"])
		}
	}
}

// TestSystemInfoPayloadMarksUnreadableCounts `system.info` 是助手问的第一句话，
// 它报的数字会被当成事实转述。回归：三处 `x, _ := List()` 把查询失败压成 0，
// 数据库读不动时助手对用户说「你还没配置站点/订阅/下载器」（#259）。
func TestSystemInfoPayloadMarksUnreadableCounts(t *testing.T) {
	siteOp, _, subCh, dlOp := sysDeps(t, openBareDB(t))
	assertUnreadableCounts(t, systemInfoPayload(siteOp, subCh, dlOp))

	// 正控：表都在 ⇒ 数字照旧给、不冒 *_error（否则助手每次都以为后端坏了）。
	db := openBareDB(t)
	if err := db.AutoMigrate(&models.Site{}, &models.Subscription{}, &models.Downloader{}); err != nil {
		t.Fatal(err)
	}
	siteOp2, _, subCh2, dlOp2 := sysDeps(t, db)
	if _, err := siteOp2.Create(oper.SiteInput{
		Name: "n", Platform: "nexusphp", BaseURL: "https://n.example", IsActive: true,
	}); err != nil {
		t.Fatalf("正控建站失败: %v", err)
	}
	out := systemInfoPayload(siteOp2, subCh2, dlOp2)
	if got, _ := out["sites"].(int); got != 1 {
		t.Errorf("sites 应为 1，got %#v", out["sites"])
	}
	if got, _ := out["downloaders"].(int); got != 0 {
		t.Errorf("空下载器表应给 0，got %#v", out["downloaders"])
	}
	for key := range out {
		if len(key) > 6 && key[len(key)-6:] == "_error" {
			t.Errorf("清单可读时不该有 %s，账面=%v", key, out)
		}
	}
}

// TestAgentToolSystemInfoOutlet 进程内 AI 助手那条 `system.info` 出口必须给同一份账面。
func TestAgentToolSystemInfoOutlet(t *testing.T) {
	siteOp, siteCh, subCh, dlOp := sysDeps(t, openBareDB(t))
	var handler func(context.Context, map[string]any) (string, error)
	for _, tool := range registerAgentTools(siteOp, siteCh, subCh, dlOp,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil) {
		if tool.Name == "system.info" {
			handler = tool.Handler
		}
	}
	if handler == nil {
		t.Fatal("system.info 未注册")
	}
	raw, err := handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("工具不该以 Go error 结束: %v", err)
	}
	var out map[string]any
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		t.Fatalf("载荷应可解析: %v (%s)", e, raw)
	}
	assertUnreadableCounts(t, out)
}

// TestHostMCPSystemInfoOutlet 对外 MCP server 那条同名工具也要给同一份账面
// （两处各抄一份时，改了一处另一处仍在报 0）。
func TestHostMCPSystemInfoOutlet(t *testing.T) {
	siteOp, siteCh, subCh, dlOp := sysDeps(t, openBareDB(t))
	srv := mcp.NewServer()
	registerHostMCPTools(srv, siteOp, siteCh, subCh, dlOp)
	var handler func(context.Context, map[string]any) (string, error)
	for _, tool := range srv.ToolList() {
		if tool.Name == "system.info" {
			handler = tool.Handler
		}
	}
	if handler == nil {
		t.Fatal("MCP server 未注册 system.info")
	}
	raw, err := handler(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("工具不该以 Go error 结束: %v", err)
	}
	var out map[string]any
	if e := json.Unmarshal([]byte(raw), &out); e != nil {
		t.Fatalf("载荷应可解析: %v (%s)", e, raw)
	}
	assertUnreadableCounts(t, out)
}
