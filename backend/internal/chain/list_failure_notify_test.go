// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package chain

import (
	"context"
	"strings"
	"testing"

	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notice"
	"github.com/hxpt/ptpatronus/internal/site"
)

// TestSiteStreamsListFailurePropagates：消息/连通性两个流面读站点清单失败时
// 必须返回 listErr（handler 末尾帧带出），且 emit 0 帧——否则前端渲染成「暂无/全部正常」。
func TestSiteStreamsListFailurePropagates(t *testing.T) {
	db := newChainDB(t)
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if _, e := siteOp.ListActive(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 ListActive 仍成功")
	}
	if _, e := siteOp.List(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 List 仍成功")
	}
	ch := NewSiteChain(NewBase(module.New(), event.New()), siteOp, site.NewRegistry())

	emitted := 0
	listErr := ch.MessagesStream(context.Background(), site.MailboxInbox, false, func(MessageResult) { emitted++ })
	if listErr == "" {
		t.Error("MessagesStream：ListActive 失败应返回 listErr 供末尾 done 行带出")
	}
	if emitted != 0 {
		t.Errorf("MessagesStream：失败时不应 emit 任何帧，emitted=%d", emitted)
	}

	emitted = 0
	listErr = ch.TestConnectionsStream(context.Background(), func(SiteConnectionResult) { emitted++ })
	if listErr == "" {
		t.Error("TestConnectionsStream：List 失败应返回 listErr 供 done 帧带出")
	}
	if emitted != 0 {
		t.Errorf("TestConnectionsStream：失败时不应 emit 任何帧，emitted=%d", emitted)
	}
}

// TestRssCheckAllListFailureNotifies：RSS cron 读启用订阅失败必须发站内通知留痕（当天去重）。
func TestRssCheckAllListFailureNotifies(t *testing.T) {
	db := newChainDB(t)
	rssOp := oper.NewRssOper(db)
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if _, e := rssOp.ListEnabled(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 ListEnabled 仍成功")
	}

	noticeDB := newChainDB(t)
	if err := noticeDB.AutoMigrate(&models.Notice{}); err != nil {
		t.Fatal(err)
	}
	ns := notice.NewService(oper.NewNoticeOper(noticeDB), nil, nil)
	ch := &RssChain{op: rssOp, notice: ns}

	ch.CheckAll(context.Background())
	ch.CheckAll(context.Background()) // 第二轮验证当天去重

	got, err := oper.NewNoticeOper(noticeDB).List(false, 10)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, x := range got {
		if x.Source == notice.SourceRssListFail {
			n++
			if x.Level != "warning" {
				t.Errorf("通知级别应为 warning，got %q", x.Level)
			}
		}
	}
	if n == 0 {
		t.Error("ListEnabled 失败应发 rss_list_fail 站内通知，实际一条没有（假绿）")
	}
	if n > 1 {
		t.Errorf("同天多轮失败应去重为 1 条，实际 %d 条", n)
	}
}

// TestCloudStrmListFailureNotifies：STRM 续签与订阅追更的读库失败必须发站内通知留痕。
func TestCloudStrmListFailureNotifies(t *testing.T) {
	noticeDB := newChainDB(t)
	if err := noticeDB.AutoMigrate(&models.Notice{}); err != nil {
		t.Fatal(err)
	}
	ns := notice.NewService(oper.NewNoticeOper(noticeDB), nil, nil)

	// 续签面：盘列表读失败。
	db := newChainDB(t)
	driveOp := oper.NewCloudDriveOper(db, security.NewCrypto("seed"))
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if _, e := driveOp.List(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 CloudDriveOper.List 仍成功")
	}
	ch := &CloudChain{op: driveOp, notice: ns}
	ch.ResignAllStrm(context.Background())

	// 追更面：订阅列表读失败（此前被并进「无订阅」分支静默）。
	db2 := newChainDB(t)
	subOp := oper.NewSubscriptionOper(db2)
	sql2, _ := db2.DB()
	sql2.Close()
	if _, e := subOp.List(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 SubscriptionOper.List 仍成功")
	}
	ch2 := &CloudChain{sub: &SubscribeChain{op: subOp}, notice: ns}
	ch2.SyncSubscriptions(context.Background())

	got, err := oper.NewNoticeOper(noticeDB).List(false, 10)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, x := range got {
		if x.Source == notice.SourceStrmListFail {
			n++
			if x.Level != "warning" {
				t.Errorf("通知级别应为 warning，got %q", x.Level)
			}
		}
	}
	if n != 2 {
		t.Errorf("续签+追更两处读失败应各发 1 条 strm_list_fail，实际 %d 条", n)
	}
}

// TestBrushflowCronListFailureNotifies：刷流选种/扫删 cron 读任务清单失败
// 必须发站内通知留痕（当天去重）——此前 `if err != nil || len==0 { return }`
// 全程无声，界面与「没有启用任务」完全同形。
func TestBrushflowCronListFailureNotifies(t *testing.T) {
	db := newChainDB(t)
	bfOp := oper.NewBrushFlowOper(db)
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if _, e := bfOp.ListEnabledTasks(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 ListEnabledTasks 仍成功")
	}
	if _, e := bfOp.ListTasks(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 ListTasks 仍成功")
	}

	noticeDB := newChainDB(t)
	if err := noticeDB.AutoMigrate(&models.Notice{}); err != nil {
		t.Fatal(err)
	}
	ns := notice.NewService(oper.NewNoticeOper(noticeDB), nil, nil)
	ch := &BrushFlowChain{op: bfOp, notice: ns}

	ch.RunAllSelect(context.Background())
	ch.RunAllSelect(context.Background()) // 第二轮验证当天去重
	ch.RunAllSweep(context.Background())

	got, err := oper.NewNoticeOper(noticeDB).List(false, 10)
	if err != nil {
		t.Fatal(err)
	}
	sel, swe := 0, 0
	for _, x := range got {
		if x.Source != notice.SourceBrushflowListFail {
			continue
		}
		if x.Level != "warning" {
			t.Errorf("通知级别应为 warning，got %q", x.Level)
		}
		switch {
		case strings.Contains(x.Key, "select"):
			sel++
		case strings.Contains(x.Key, "sweep"):
			swe++
		}
	}
	if sel != 1 {
		t.Errorf("选种清单读失败应恰好 1 条（同天去重），实际 %d 条", sel)
	}
	if swe != 1 {
		t.Errorf("扫删清单读失败应恰好 1 条（同天去重），实际 %d 条", swe)
	}
}

// TestContractCronListFailureNotifies：保种契约 cron 读启用规则失败必须发站内
// 通知留痕（当天去重）——此前静默 return，当天一条规则都不检查也没人知道。
func TestContractCronListFailureNotifies(t *testing.T) {
	db := newChainDB(t)
	cOp := oper.NewContractOper(db)
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if _, e := cOp.ListEnabled(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 ContractOper.ListEnabled 仍成功")
	}

	noticeDB := newChainDB(t)
	if err := noticeDB.AutoMigrate(&models.Notice{}); err != nil {
		t.Fatal(err)
	}
	ns := notice.NewService(oper.NewNoticeOper(noticeDB), nil, nil)
	ch := &ContractChain{op: cOp, notice: ns}

	ch.CheckAll(context.Background())
	ch.CheckAll(context.Background()) // 第二轮验证当天去重

	got, err := oper.NewNoticeOper(noticeDB).List(false, 10)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, x := range got {
		if x.Source == notice.SourceContractListFail {
			n++
			if x.Level != "warning" {
				t.Errorf("通知级别应为 warning，got %q", x.Level)
			}
		}
	}
	if n != 1 {
		t.Errorf("规则清单读失败应恰好 1 条（同天去重），实际 %d 条", n)
	}
}
