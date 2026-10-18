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

// TestDashboardOverviewStatReadFailureNotRenderedAsAllSitesOffline 回归：
// 「每站最新统计」查询出错时（库锁/表异常），此前错误被整条吞掉——statMap 空 →
// 每个站都走「无统计」分支 → 首屏红横幅喊「疑似 cookie 失效/未采集」，把读取故障
// 渲染成全站集体掉线，用户会白跑一趟去重登所有站。
// 现在必须：StatReadError 带出原因，且不再据此判定「无数据」。
func TestDashboardOverviewStatReadFailureNotRenderedAsAllSitesOffline(t *testing.T) {
	db := newOverviewDB(t)
	siteOp := oper.NewSiteOper(db, nil)
	for _, n := range []string{"alpha", "beta"} {
		if _, err := siteOp.Create(oper.SiteInput{
			Name: n, Platform: "nexusphp", BaseURL: "https://" + n + ".pt", IsActive: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	ch := &DashboardChain{db: db, siteOp: siteOp, dlOp: oper.NewDownloaderOper(db, nil)}

	// 正控（表健康、真的没采过）：两个站都进 no_data_sites，且没有读取错误。
	// 没有这一半，「读取失败时不报无数据」可以靠把功能整体关掉蒙混过关。
	first, err := ch.Overview(context.Background())
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	if first.StatReadError != "" {
		t.Fatalf("表健康时不该有读取错误，got %q", first.StatReadError)
	}
	if len(first.NoDataSites) != 2 {
		t.Fatalf("健康表下应判 2 个站无数据，got %+v", first.NoDataSites)
	}

	// 制造读取故障：表没了，MAX(id) 那条 SQL 必然报错。
	if err := db.Migrator().DropTable(&models.SiteStat{}); err != nil {
		t.Fatal(err)
	}
	second, err := ch.Overview(context.Background())
	if err != nil {
		t.Fatalf("Overview 不应整体失败（站点列表仍可用）: %v", err)
	}
	if second.StatReadError == "" {
		t.Fatal("统计查询失败必须带出原因（此前被吞成空 statMap）")
	}
	if len(second.NoDataSites) != 0 {
		t.Fatalf("读不到统计不等于站点没数据，不应报无数据，got %+v", second.NoDataSites)
	}
	if len(second.Sites) != 2 {
		t.Fatalf("站点列表仍应返回 2 站，got %d", len(second.Sites))
	}
	// 契约：字段名与前端读取一致。
	raw, _ := json.Marshal(second)
	if !strings.Contains(string(raw), `"stat_read_error":`) {
		t.Fatalf("overview JSON 应含 stat_read_error:\n%s", raw)
	}
}
