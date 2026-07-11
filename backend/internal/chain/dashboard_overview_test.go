package chain

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// newOverviewDB 建一个迁移了 sites/downloaders/site_stats 的内存文件库
// （Overview 三处查询：siteOp.List / dlOp.List / site_stats MAX(id)）。
func newOverviewDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/overview.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Site{}, &models.Downloader{}, &models.SiteStat{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// 回归：无站点时 Overview 返回的 Sites 必须是空数组而非 nil。
// Go 的 nil slice 经 json.Marshal 会输出 "sites":null，前端裸访问
// overview.sites.length 会抛 "Cannot read properties of null (reading 'length')"，
// 表现为仪表盘一打开就白屏报错（新部署 / 全删站点场景）。
func TestDashboardOverviewEmptySitesNotSerializedAsNull(t *testing.T) {
	db := newOverviewDB(t)
	chain := &DashboardChain{
		db:     db,
		siteOp: oper.NewSiteOper(db, nil), // List 路径不解密，Crypto 可空
		dlOp:   oper.NewDownloaderOper(db, nil),
	}

	out, err := chain.Overview(context.Background())
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if out.Sites == nil {
		t.Fatalf("out.Sites is nil; want non-nil empty slice so JSON serializes to []")
	}

	// 契约级断言：JSON 必须含 "sites":[]，不得含 "sites":null。
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(raw), `"sites":null`) {
		t.Fatalf("overview JSON contains \"sites\":null (nil slice leaked); want []:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"sites":[]`) {
		t.Fatalf("overview JSON missing \"sites\":[]:\n%s", raw)
	}
}
