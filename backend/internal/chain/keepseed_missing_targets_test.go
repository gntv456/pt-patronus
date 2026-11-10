// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/db/oper"
)

// missingDLStub 假装下载器连不上：正控用它确认「空目标站」这条守卫不是唯一的错误来源。
type missingDLStub struct{}

func (missingDLStub) Get(context.Context, uint) (reseedClient, error) {
	return nil, errors.New("下载器不存在")
}

// TestFindMissingRejectsEmptyTargets 回归：target_site_ids 为空时旧实现返回一个全零的
// MissingSummary（Total=0/Targets=0/Missing=[]），界面与工具把它渲染成「目标站都不缺辅种」——
// 「一个站都没选」和「查过了、没缺」共用同一个绿色出口。
func TestFindMissingRejectsEmptyTargets(t *testing.T) {
	db := newReseedDB(t)
	cfgOp := oper.NewSystemConfigOper(db)
	ch := &KeepSeedChain{cfgOp: cfgOp, dl: missingDLStub{}}
	if err := ch.SetToken("test-iyuu-token"); err != nil {
		t.Fatal(err)
	}
	// 前提断言：token 必须真的可读，否则下面拿到的错误就不是本条守卫给的
	if tok, err := ch.GetToken(); err != nil || tok != "test-iyuu-token" {
		t.Fatalf("前提不成立：IYUU token 应可读，got %q err %v", tok, err)
	}

	for _, ids := range [][]uint{nil, {}} {
		sum, err := ch.FindMissing(context.Background(), 1, ids)
		if err == nil {
			t.Fatalf("空目标站必须报错，got summary %+v", sum)
		}
		if !strings.Contains(err.Error(), "未指定") {
			t.Errorf("错误要说清是没选目标站，got %v", err)
		}
	}

	// 正控：选了站就不该撞这条守卫（这里让它去连不存在的下载器，错误口径应不同）。
	if _, err := ch.FindMissing(context.Background(), 999, []uint{7}); err != nil &&
		strings.Contains(err.Error(), "未指定") {
		t.Errorf("已指定目标站却被守卫拦下：%v", err)
	}
}
