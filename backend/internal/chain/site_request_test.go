// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/site"
)

// TestRequestsListFailurePropagates 验证站点清单读失败必须交回错误（JSON errcode），
// 不能渲染成「全部站无求种」假绿。
func TestRequestsListFailurePropagates(t *testing.T) {
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if _, e := siteOp.ListActive(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 ListActive 仍成功")
	}
	ch := NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry())

	if _, err := ch.RequestsAll(context.Background(), false); err == nil {
		t.Error("RequestsAll：ListActive 失败应交回错误，而不是 nil+nil")
	}
}
