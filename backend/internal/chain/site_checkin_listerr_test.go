package chain

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// noSitesTableChain 造一个「站点清单读不出来」的链：库里压根没有 sites 表，
// ListActive 的 Find 必然报错（SQLite: no such table: sites）。
// 这就是现网里 sqlite 文件损坏 / 迁移没跑完 / 磁盘只读时的真实形状。
func noSitesTableChain(t *testing.T) *SiteChain {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/empty.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	siteOp := oper.NewSiteOper(db, security.NewCrypto("seed"))
	return NewSiteChain(NewBase(module.New(), event.New()), siteOp, nil)
}

// TestCheckinAllSurfacesSiteListReadFailure 一键签到的站清单读不出来时必须回错误。
// 回归：链层把 err 咽成 (nil, nil)，四个消费者各自把「读不到」讲成另一种谎——
// Web/移动端弹「没有启用的站点」，工作流节点播报「全站签到：0/0 成功」，
// MCP 工具回空数组。用户看到的是「一切正常，只是没站可签」，实际是一次查询失败。
func TestCheckinAllSurfacesSiteListReadFailure(t *testing.T) {
	ch := noSitesTableChain(t)

	results, err := ch.CheckinAll(context.Background())
	if err == nil {
		t.Fatalf("站清单查询失败必须透传，got err=nil results=%+v", results)
	}
	if !strings.Contains(err.Error(), "读取启用站点清单失败") {
		t.Errorf("错误要带「哪一步读不出来」的中文上下文，got %q", err.Error())
	}
	if results != nil {
		t.Errorf("读失败时不该返回半截结果，got %+v", results)
	}

	// 正控：同一个链换成有表的库就能正常跑出 0 站（空清单不是错误，别把两者混成一个出口）。
	okCh, siteOp := newSearchChain(t, &fakeAdapter{schema: "nexusphp"})
	createActive(t, siteOp, "nexusphp", "nexusphp")
	got, err := okCh.CheckinAll(context.Background())
	if err != nil {
		t.Fatalf("正控：清单可读时不该报错: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("正控：1 个启用站应有 1 行结果, got %d", len(got))
	}
}
