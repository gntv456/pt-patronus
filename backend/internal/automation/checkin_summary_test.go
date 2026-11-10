// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package automation

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

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

// fakeCheckinAdapter 支持签到的假适配器；fakePlainAdapter 不支持（模拟 mteam/unit3d 等）。
type fakeCheckinAdapter struct {
	schema string
	result *site.CheckinResult
	err    error
}

func (f *fakeCheckinAdapter) Name() string   { return "fake-checkin" }
func (f *fakeCheckinAdapter) Priority() int  { return 100 }
func (f *fakeCheckinAdapter) Schema() string { return f.schema }
func (f *fakeCheckinAdapter) Search(context.Context, *site.SiteMetadata, string, string, string, string, int) ([]site.Torrent, error) {
	return nil, nil
}
func (f *fakeCheckinAdapter) FetchUserInfo(context.Context, *site.SiteMetadata, string, string) (*site.UserInfo, error) {
	return &site.UserInfo{}, nil
}
func (f *fakeCheckinAdapter) Checkin(context.Context, *site.SiteMetadata, string, string) (*site.CheckinResult, error) {
	return f.result, f.err
}

type fakePlainAdapter struct{ schema string }

func (f *fakePlainAdapter) Name() string   { return "fake-plain" }
func (f *fakePlainAdapter) Priority() int  { return 100 }
func (f *fakePlainAdapter) Schema() string { return f.schema }
func (f *fakePlainAdapter) Search(context.Context, *site.SiteMetadata, string, string, string, string, int) ([]site.Torrent, error) {
	return nil, nil
}
func (f *fakePlainAdapter) FetchUserInfo(context.Context, *site.SiteMetadata, string, string) (*site.UserInfo, error) {
	return &site.UserInfo{}, nil
}

// fakeKeepAliveAdapter 支持 cookie 保活的假适配器（返回轮换后的 cookie）。
type fakeKeepAliveAdapter struct{ schema string }

func (f *fakeKeepAliveAdapter) Name() string   { return "fake-keepalive" }
func (f *fakeKeepAliveAdapter) Priority() int  { return 100 }
func (f *fakeKeepAliveAdapter) Schema() string { return f.schema }
func (f *fakeKeepAliveAdapter) Search(context.Context, *site.SiteMetadata, string, string, string, string, int) ([]site.Torrent, error) {
	return nil, nil
}
func (f *fakeKeepAliveAdapter) FetchUserInfo(context.Context, *site.SiteMetadata, string, string) (*site.UserInfo, error) {
	return &site.UserInfo{}, nil
}
func (f *fakeKeepAliveAdapter) KeepAlive(context.Context, *site.SiteMetadata, string, string) (*site.KeepAliveResult, error) {
	return &site.KeepAliveResult{LoggedIn: true, RefreshedCookie: "new=2"}, nil
}

// captureChannel 捕获最后一条通知（断言汇总文案用）。
type captureChannel struct{ last notification.Message }

func (c *captureChannel) Name() string { return "capture" }
func (c *captureChannel) Send(_ context.Context, msg notification.Message) error {
	c.last = msg
	return nil
}

// TestRunCheckinSummaryCountsUnsupportedAsSkipped 每日签到汇总：不支持签到的站（未实现
// CheckinCapable）与无适配器站计「跳过」而非「失败」——否则每日通知恒报「失败 N」，
// N≈非 NexusPHP 站数，把真实失败淹没在噪音里。
func TestRunCheckinSummaryCountsUnsupportedAsSkipped(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/checkin.db"), &gorm.Config{})
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
	siteOp := oper.NewSiteOper(db, security.NewCrypto("checkin-test-seed"))
	mk := func(name, platform string) {
		t.Helper()
		if _, err := siteOp.Create(oper.SiteInput{Name: name, Platform: platform, BaseURL: "https://" + platform + ".example", IsActive: true}); err != nil {
			t.Fatal(err)
		}
	}
	mk("签到成功站", "nexusphp")
	mk("签到失败站", "hdchina") // 适配器在但 Checkin 报错 → 计失败
	mk("不支持签到站", "mteam")  // 无 CheckinCapable → 计跳过
	mk("无适配器站", "dead")    // ErrNoAdapter → 计跳过

	mm := module.New()
	mm.Register(&fakeCheckinAdapter{schema: "nexusphp", result: &site.CheckinResult{Success: true, Message: "签到成功"}})
	mm.Register(&fakeCheckinAdapter{schema: "hdchina", err: errors.New("boom")})
	mm.Register(&fakePlainAdapter{schema: "mteam"})
	siteCh := chain.NewSiteChain(chain.NewBase(mm, event.New()), siteOp, site.NewRegistry())

	cap := &captureChannel{}
	nc := chain.NewNotificationChain(chain.NewBase(module.New(), event.New()), notification.NewManager(cap))
	s := &Service{siteCh: siteCh, siteOp: siteOp, notif: nc, rootCtx: context.Background()}

	s.runCheckin()

	want := "签到完成：成功 1，失败 1，跳过 2（不支持签到）"
	if cap.last.Body != want {
		t.Fatalf("汇总文案 = %q, want %q", cap.last.Body, want)
	}
}

// ctxBlockingCheckinAdapter 慢站适配器：阻塞至 ctx 取消（模拟站点挂起/无响应）。
type ctxBlockingCheckinAdapter struct{ schema string }

func (f *ctxBlockingCheckinAdapter) Name() string   { return "fake-block" }
func (f *ctxBlockingCheckinAdapter) Priority() int  { return 200 }
func (f *ctxBlockingCheckinAdapter) Schema() string { return f.schema }
func (f *ctxBlockingCheckinAdapter) Search(context.Context, *site.SiteMetadata, string, string, string, string, int) ([]site.Torrent, error) {
	return nil, nil
}
func (f *ctxBlockingCheckinAdapter) FetchUserInfo(context.Context, *site.SiteMetadata, string, string) (*site.UserInfo, error) {
	return &site.UserInfo{}, nil
}
func (f *ctxBlockingCheckinAdapter) Checkin(ctx context.Context, _ *site.SiteMetadata, _, _ string) (*site.CheckinResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestRunCheckinSlowSiteDoesNotStarveOthers 慢站（挂起）被单站超时隔离，后续站仍签到成功：
// 此前全站共享总预算，慢站耗尽后其余站全部 ctx 取消 → 当日漏签（cron 当日不再重试）。
func TestRunCheckinSlowSiteDoesNotStarveOthers(t *testing.T) {
	oldPerSite, oldTotal := checkinPerSiteTimeout, checkinTotalTimeout
	checkinPerSiteTimeout, checkinTotalTimeout = 100*time.Millisecond, 2*time.Second
	t.Cleanup(func() { checkinPerSiteTimeout, checkinTotalTimeout = oldPerSite, oldTotal })

	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/checkin.db"), &gorm.Config{})
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
	siteOp := oper.NewSiteOper(db, security.NewCrypto("checkin-test-seed"))
	for _, x := range []struct{ name, platform string }{
		{"慢站", "slow"}, {"快站", "fast"},
	} {
		if _, err := siteOp.Create(oper.SiteInput{Name: x.name, Platform: x.platform, BaseURL: "https://" + x.platform + ".example", IsActive: true}); err != nil {
			t.Fatal(err)
		}
	}

	mm := module.New()
	mm.Register(&ctxBlockingCheckinAdapter{schema: "slow"})
	mm.Register(&fakeCheckinAdapter{schema: "fast", result: &site.CheckinResult{Success: true, Message: "签到成功"}})
	siteCh := chain.NewSiteChain(chain.NewBase(mm, event.New()), siteOp, site.NewRegistry())

	cap := &captureChannel{}
	nc := chain.NewNotificationChain(chain.NewBase(module.New(), event.New()), notification.NewManager(cap))
	s := &Service{siteCh: siteCh, siteOp: siteOp, notif: nc, rootCtx: context.Background()}

	start := time.Now()
	s.runCheckin()

	want := "签到完成：成功 1，失败 1"
	if cap.last.Body != want {
		t.Fatalf("慢站应被单站超时隔离、快站照常成功：汇总 = %q, want %q", cap.last.Body, want)
	}
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("慢站不应拖满总预算：耗时 %v", elapsed)
	}
}

// TestRunCookieKeepAliveClassifiesUnsupportedAndPersistsCookie 小时级 cookie 保活：
//   - 不支持保活的站（mteam 等未实现 KeepAliveCapable）不算失败（防 Grafana 误报）；
//   - 支持的站轮换后的 cookie 合并持久化（保活→落库链路）。
func TestRunCookieKeepAliveClassifiesUnsupportedAndPersistsCookie(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/keepalive.db"), &gorm.Config{})
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
	siteOp := oper.NewSiteOper(db, security.NewCrypto("keepalive-test-seed"))
	sKeep, err := siteOp.Create(oper.SiteInput{Name: "可保活站", Platform: "nexusphp", BaseURL: "https://keep.example", Cookie: "old=1", IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := siteOp.Create(oper.SiteInput{Name: "不支持保活站", Platform: "mteam", BaseURL: "https://mteam.example", IsActive: true}); err != nil {
		t.Fatal(err)
	}

	mm := module.New()
	mm.Register(&fakeKeepAliveAdapter{schema: "nexusphp"})
	mm.Register(&fakePlainAdapter{schema: "mteam"})
	siteCh := chain.NewSiteChain(chain.NewBase(mm, event.New()), siteOp, site.NewRegistry())

	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })

	s := &Service{siteCh: siteCh, siteOp: siteOp, rootCtx: context.Background()}
	s.runCookieKeepAlive()

	logs := buf.String()
	if strings.Contains(logs, "cookie-keepalive failed") {
		t.Errorf("不支持保活的站不应计入失败: %s", logs)
	}
	if !strings.Contains(logs, "failed=0") {
		t.Errorf("汇总 failed 应为 0: %s", logs)
	}
	fresh, err := siteOp.Get(sKeep.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := siteOp.Cookie(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "new=2") {
		t.Errorf("轮换后 cookie 未持久化: %q", got)
	}
}
