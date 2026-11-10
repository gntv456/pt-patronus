// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package automation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/notification"
	"github.com/hxpt/ptpatronus/internal/site"
	"gorm.io/gorm"
)

// 签到汇总的失败必须带级别、也必须出声：
//   - 汇总恒报 info ⇒ 通知列表里「失败 3」和「全部成功」同色；
//   - 站点清单读不出时只打日志后 return ⇒ 与「今天没安排签到」长得一模一样。

type checkinSpec struct {
	name, platform string
	result         *site.CheckinResult
	err            error
}

// newCheckinService 建一条接内存 DB 的签到 Service，并返回捕获通知的通道。
func newCheckinService(t *testing.T, specs []checkinSpec) (*Service, *captureChannel) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/checkin_sev.db"), &gorm.Config{})
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
	siteOp := oper.NewSiteOper(db, security.NewCrypto("checkin-sev-seed"))
	mm := module.New()
	for _, sp := range specs {
		if _, err := siteOp.Create(oper.SiteInput{
			Name: sp.name, Platform: sp.platform, BaseURL: "https://" + sp.platform + ".example", IsActive: true,
		}); err != nil {
			t.Fatal(err)
		}
		mm.Register(&fakeCheckinAdapter{schema: sp.platform, result: sp.result, err: sp.err})
	}
	capCh := &captureChannel{}
	nc := chain.NewNotificationChain(chain.NewBase(module.New(), event.New()), notification.NewManager(capCh))
	return &Service{
		siteCh:  chain.NewSiteChain(chain.NewBase(mm, event.New()), siteOp, site.NewRegistry()),
		siteOp:  siteOp,
		notif:   nc,
		rootCtx: context.Background(),
	}, capCh
}

func TestRunCheckinLevelFollowsOutcome(t *testing.T) {
	ok := &site.CheckinResult{Success: true, Message: "签到成功"}

	s, capCh := newCheckinService(t, []checkinSpec{
		{"A", "nexusphp", ok, nil},
		{"B", "hdchina", ok, nil},
	})
	s.runCheckin()
	if !strings.Contains(capCh.last.Body, "成功 2") {
		t.Fatalf("前提不成立：两站都应签到成功，body=%q", capCh.last.Body)
	}
	if capCh.last.Level != "info" {
		t.Errorf("全部成功应为 info，got %q", capCh.last.Level)
	}

	s2, cap2 := newCheckinService(t, []checkinSpec{
		{"A", "nexusphp", ok, nil},
		{"B", "hdchina", nil, errors.New("boom")},
	})
	s2.runCheckin()
	if !strings.Contains(cap2.last.Body, "失败 1") {
		t.Fatalf("汇总要计入失败：%q", cap2.last.Body)
	}
	if cap2.last.Level != "warning" {
		t.Errorf("有失败时通知必须是 warning 级，got %q（body=%q）", cap2.last.Level, cap2.last.Body)
	}
}

// TestRunCheckinAnnouncesUnreadableSiteList 站点清单读不出：旧实现只打日志后 return，
// 通知中心当天什么都收不到，与「没配签到」同一副面孔。
func TestRunCheckinAnnouncesUnreadableSiteList(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/checkin_dead.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	siteOp := oper.NewSiteOper(db, security.NewCrypto("checkin-dead-seed"))
	capCh := &captureChannel{}
	nc := chain.NewNotificationChain(chain.NewBase(module.New(), event.New()), notification.NewManager(capCh))
	s := &Service{siteOp: siteOp, notif: nc, rootCtx: context.Background()}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	// 前提断言：清单必须真的读不出来，否则下面断言的是别的路径
	if _, e := siteOp.ListActive(); e == nil {
		t.Fatal("前提不成立：关闭 DB 后 ListActive 仍成功")
	}

	s.runCheckin()
	if capCh.last.Body == "" {
		t.Fatal("读不出站点清单时必须发通知，实际一条都没有")
	}
	if !strings.Contains(capCh.last.Body, "未能开始") || capCh.last.Level != "warning" {
		t.Errorf("通知要说清没跑成且为告警级，got level=%q body=%q", capCh.last.Level, capCh.last.Body)
	}
}
