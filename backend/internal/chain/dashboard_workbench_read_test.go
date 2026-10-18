package chain

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// newWorkbenchDB 建工作台所需的全部表，并放 2 个启用站点、无任何统计。
func newWorkbenchDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/wb.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Site{}, &models.SiteStat{}, &models.Subscription{}, &models.SubscriptionHit{},
		&models.TransferHistory{}, &models.CloudTask{}, &models.StrmRecord{}, &models.Notice{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	siteOp := oper.NewSiteOper(db, security.NewCrypto("wb"))
	for _, n := range []string{"alpha", "beta"} {
		if _, err := siteOp.Create(oper.SiteInput{Name: n, Platform: "nexusphp", BaseURL: "https://" + n + ".pt", IsActive: true}); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func workbenchTodosOf(w *Workbench, kind string) []WorkbenchTodo {
	var out []WorkbenchTodo
	for _, td := range w.Todos {
		if td.Kind == kind {
			out = append(out, td)
		}
	}
	return out
}

func workbenchHasErr(w *Workbench, want string) bool {
	for _, e := range w.Errors {
		if strings.Contains(e, want) {
			return true
		}
	}
	return false
}

// TestWorkbenchHealthyNoDataSites 正控：表都在、只是没采到过数据 → 逐站造出「无采集数据」待办，
// Errors 为空。没有这一条，下面的「读失败不得造待办」可以靠把整块逻辑关掉蒙混过关。
func TestWorkbenchHealthyNoDataSites(t *testing.T) {
	db := newWorkbenchDB(t)
	c := &DashboardChain{db: db}
	w, err := c.Workbench(context.Background(), nil)
	if err != nil {
		t.Fatalf("Workbench: %v", err)
	}
	if len(w.Errors) != 0 {
		t.Fatalf("健康库不应有读取错误：%v", w.Errors)
	}
	if w.Summary.SitesNoData != 2 || len(workbenchTodosOf(w, "no_data_site")) != 2 {
		t.Fatalf("两个无统计的启用站应各出一条待办，got summary=%d todos=%+v", w.Summary.SitesNoData, w.Todos)
	}
}

// TestWorkbenchReadFailureNotRenderedAsFindings 回归：站点最新统计那条查询原先丢弃错误——
// 读失败时 statMap 为空，逐站判「无采集数据（Cookie 失效或未采集）」，凭空造出 N 条待办、
// 还把体检分扣下去；用户会去重登所有站点，而真因是这一次查询失败。
func TestWorkbenchReadFailureNotRenderedAsFindings(t *testing.T) {
	db := newWorkbenchDB(t)
	if err := db.Migrator().DropTable(&models.SiteStat{}); err != nil {
		t.Fatal(err)
	}
	c := &DashboardChain{db: db}
	w, err := c.Workbench(context.Background(), nil)
	if err != nil {
		t.Fatalf("部分域读失败不应整体报错（其余区块仍要能看）: %v", err)
	}
	if !workbenchHasErr(w, "站点最新统计") {
		t.Fatalf("应记录统计读取失败原因，got %v", w.Errors)
	}
	if !workbenchHasErr(w, "今日流量增量") {
		t.Fatalf("同一张表崩了，增量聚合也要记录，got %v", w.Errors)
	}
	if n := len(workbenchTodosOf(w, "no_data_site")); n != 0 {
		t.Fatalf("读不到统计不得造无数据待办，got %d 条", n)
	}
	if w.Summary.SitesNoData != 0 {
		t.Fatalf("SitesNoData 应保持 0（未知≠无数据），got %d", w.Summary.SitesNoData)
	}
	if w.Summary.SitesActive != 2 {
		t.Errorf("能读到的域仍应正常汇总，SitesActive=%d", w.Summary.SitesActive)
	}
	// 体检分不该因为「读不到」被扣：无数据项已跳过，此处只应剩正常分值。
	if w.HealthScore < 100 {
		t.Errorf("读失败不应拉低体检分（会被当成站点出问题），got %d", w.HealthScore)
	}
}

// TestWorkbenchCountsReadFailure 整理失败数读不出来时，既不能报 0 当「今日无失败」，
// 也不能悄悄不扣分——必须留痕在 Errors 里。
func TestWorkbenchCountsReadFailure(t *testing.T) {
	db := newWorkbenchDB(t)
	if err := db.Migrator().DropTable(&models.TransferHistory{}); err != nil {
		t.Fatal(err)
	}
	c := &DashboardChain{db: db}
	w, err := c.Workbench(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !workbenchHasErr(w, "今日整理失败") || !workbenchHasErr(w, "今日整理成功") {
		t.Fatalf("整理域的读取失败要逐条留痕，got %v", w.Errors)
	}
	if w.Summary.OrganizeFailed != 0 || len(workbenchTodosOf(w, "transfer_failed")) != 0 {
		t.Fatalf("读不到时不得凭空造「整理失败待办」，got %+v", w.Todos)
	}
}
