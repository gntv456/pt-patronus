package chain

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

func newMediaLibTestChain(t *testing.T) (*MediaServerChain, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/ml.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.MediaServer{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	ch, err := NewMediaServerChain(NewBase(module.New(), event.New()), oper.NewMediaServerOper(db, security.NewCrypto("ml")), nil)
	if err != nil {
		t.Fatal(err)
	}
	return ch, db
}

// TestMediaLibraryOverviewDistinguishesReadFailure 「读不到媒体服务器列表」与
// 「一台都没配置」必须给出不同结论。
// 回归：c.List() 出错时直接返回 Configured=false，工作台首屏于是对着一台配了
// Emby/Plex 的库显示「未配置媒体库 · 去添加」，用户顺着新建一遍就成了重复配置。
func TestMediaLibraryOverviewDistinguishesReadFailure(t *testing.T) {
	ch, db := newMediaLibTestChain(t)

	// 正控：表在、确实没配 → 未配置且无错误（这时才该引导去添加）。
	empty := ch.MediaLibraryOverview(context.Background())
	if empty.Configured || empty.Error != "" || len(empty.Servers) != 0 {
		t.Fatalf("空配置应报「未配置」且无错误，got %+v", empty)
	}

	// 制造读取故障。
	if err := db.Migrator().DropTable(&models.MediaServer{}); err != nil {
		t.Fatal(err)
	}
	bad := ch.MediaLibraryOverview(context.Background())
	if bad.Configured {
		t.Error("读失败时不得声称已配置")
	}
	if !strings.Contains(bad.Error, "媒体服务器列表读取失败") {
		t.Fatalf("读失败必须与未配置区分开，got %q", bad.Error)
	}
}

// TestWorkbenchCarriesMediaLibraryError 回调给出的留痕要能原样传到响应里（否则前端看不见）。
func TestWorkbenchCarriesMediaLibraryError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/wb_ml.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Site{}, &models.SiteStat{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	c := &DashboardChain{db: db}
	w, err := c.Workbench(context.Background(), func(context.Context) WorkbenchMediaLibrary {
		return WorkbenchMediaLibrary{Configured: false, Servers: []MediaLibraryStat{}, Error: "媒体服务器列表读取失败: boom"}
	})
	if err != nil {
		t.Fatal(err)
	}
	if w.MediaLibrary.Error == "" || w.MediaLibrary.Configured {
		t.Fatalf("媒体库读取失败要原样带出，got %+v", w.MediaLibrary)
	}
}
