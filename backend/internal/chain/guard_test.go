// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// TestRunGuardedPanicBecomesError：单项 panic 必须就地转成错误交回调用方
// （cron 循环里一个坏条目不得沿栈上抛、让同轮其余条目全漏跑），
// 非 panic 错误必须原样透传（errors.Is 语义不丢）。
func TestRunGuardedPanicBecomesError(t *testing.T) {
	sentinel := errors.New("boom")
	if err := runGuarded(func() error { return sentinel }); !errors.Is(err, sentinel) {
		t.Errorf("非 panic 错误应原样透传，got %v", err)
	}
	if err := runGuarded(func() error { return nil }); err != nil {
		t.Errorf("正常返回应为 nil，got %v", err)
	}
	err := runGuarded(func() error { panic("bad item") })
	if err == nil {
		t.Fatal("panic 应被转成错误（nil 会让调用方把崩掉的一格当成功）")
	}
	if !strings.Contains(err.Error(), "panic: bad item") {
		t.Errorf("错误应携带 panic 内容，got %q", err.Error())
	}
}

// TestContractCheckAllIsolatesPerItemPanic：循环接线验证——单项 panic 必须被就地兜住
// （scheduler 级 recover 只保进程不保同轮其余条目），且坏条目之后的规则必须继续检查。
func TestContractCheckAllIsolatesPerItemPanic(t *testing.T) {
	db := newChainDB(t)
	if err := db.AutoMigrate(&models.ContractRule{}); err != nil {
		t.Fatal(err)
	}
	cOp := oper.NewContractOper(db)

	// bad：链未组装站侧（siteOp=nil），Check 走到站点查询即 panic。
	bad := &models.ContractRule{SiteID: 1, Enabled: true}
	if err := cOp.Upsert(bad); err != nil {
		t.Fatal(err)
	}
	// good：未到生效时间 → 短路分支先落 MarkChecked ok，作为「循环继续」的正向断言。
	future := time.Now().Add(time.Hour)
	good := &models.ContractRule{SiteID: 2, Enabled: true, StartTime: &future}
	if err := cOp.Upsert(good); err != nil {
		t.Fatal(err)
	}

	ch := &ContractChain{op: cOp}
	ch.CheckAll(context.Background())

	got, err := cOp.Get(good.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastStatus != "ok" {
		t.Errorf("坏条目 panic 后同轮其余规则必须继续检查：LastStatus 应 ok，got %q", got.LastStatus)
	}
}
