// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 The PTPatronus Authors

package nodes

import (
	"context"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/chain"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/core/security"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"github.com/hxpt/ptpatronus/internal/workflow"
	"gorm.io/gorm"
)

func newSiteStatsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/wf.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// 节点走 db.Table("site_stats")，这里按实际用到的列建表。
	ddl := `CREATE TABLE site_stats (id INTEGER PRIMARY KEY, site_id INTEGER, uploaded INTEGER,` +
		`downloaded INTEGER, ratio REAL, bonus REAL, snap_at TEXT)`
	if err := db.Exec(ddl).Error; err != nil {
		t.Fatalf("create site_stats: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// TestFetchSiteStatsNodeNoSnapshotIsError 回归：Scan 在零行时不报错，
// 于是从没采过样的站点会带着全 0 变量「成功」下去——播报写成「上传 0 / 分享率 0.00」，
// 下游条件分支也按这个假 0 决策。
func TestFetchSiteStatsNodeNoSnapshotIsError(t *testing.T) {
	db := newSiteStatsDB(t)
	ecx := &workflow.Context{}
	err := NewFetchSiteStatsNode(db).Execute(context.Background(), map[string]any{"site_id": float64(7)}, ecx)
	if err == nil {
		t.Fatalf("零行必须报错，got 变量=%v 消息=%v", ecx.Variables, ecx.Messages)
	}
	if !strings.Contains(err.Error(), "还没有采集到统计数据") {
		t.Errorf("错误应说明没有数据，got %v", err)
	}
	if len(ecx.Variables) != 0 {
		t.Errorf("报错时不得写入全 0 变量，got %v", ecx.Variables)
	}

	// 别的站有数据也算这个站没数据（WHERE 仍生效）。
	seed := `INSERT INTO site_stats (site_id, uploaded, downloaded, ratio, bonus, snap_at)` +
		` VALUES (8, 999, 99, 9.9, 9, '2026-10-10 08:00:00')`
	if err := db.Exec(seed).Error; err != nil {
		t.Fatal(err)
	}
	ecx2 := &workflow.Context{}
	if err := NewFetchSiteStatsNode(db).Execute(context.Background(), map[string]any{"site_id": float64(7)}, ecx2); err == nil {
		t.Fatalf("站 7 无数据不得拿站 8 的行成功，got %v", ecx2.Variables)
	}
}

// TestFetchSiteStatsNodeReadsLatestSnapshot 正控：有数据时照常取最新一行并写入变量。
func TestFetchSiteStatsNodeReadsLatestSnapshot(t *testing.T) {
	db := newSiteStatsDB(t)
	inserts := []string{
		`INSERT INTO site_stats (site_id, uploaded, downloaded, ratio, bonus, snap_at) VALUES (7, 100, 50, 2.0, 5, '2026-10-09 08:00:00')`,
		`INSERT INTO site_stats (site_id, uploaded, downloaded, ratio, bonus, snap_at) VALUES (7, 123, 45, 2.7, 9, '2026-10-10 08:00:00')`,
	}
	for _, q := range inserts {
		if err := db.Exec(q).Error; err != nil {
			t.Fatal(err)
		}
	}
	ecx := &workflow.Context{}
	if err := NewFetchSiteStatsNode(db).Execute(context.Background(), map[string]any{"site_id": float64(7)}, ecx); err != nil {
		t.Fatalf("有数据不应报错: %v", err)
	}
	if got := ecx.Variables["uploaded"]; got != int64(123) {
		t.Fatalf("应取最新一行的 uploaded，got %#v", got)
	}
	if len(ecx.Messages) == 0 || !strings.Contains(ecx.Messages[len(ecx.Messages)-1], "上传 123") {
		t.Errorf("播报消息应带上真实数值，got %v", ecx.Messages)
	}
}

// newEmptySiteChain 造一个「库里一个站点都没有」的 SiteChain（SearchSites 返回空切片 + nil error）。
func newEmptySiteChain(t *testing.T) *chain.SiteChain {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/sites.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Site{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	return chain.NewSiteChain(chain.NewBase(module.New(), event.New()), oper.NewSiteOper(db, security.NewCrypto("wf-node-test-key")), nil)
}

// TestSearchNodesNoSiteIsError 回归：零站点时 SearchSites 返回 (空, nil)，
// 两个搜索节点都曾把它播报成「搜索到 0 条资源」并记 success——
// 自动追剧模板在新实例上每天全绿，其实一次都没搜过。
func TestSearchNodesNoSiteIsError(t *testing.T) {
	siteCh := newEmptySiteChain(t)
	ecx := &workflow.Context{}
	err := NewSearchTorrentsNode(siteCh).Execute(context.Background(), map[string]any{"keyword": "某种剧"}, ecx)
	if err == nil {
		t.Fatalf("零站点必须报错，got 消息=%v 种子=%d", ecx.Messages, len(ecx.Torrents))
	}
	if !strings.Contains(err.Error(), "没有可搜索的站点") {
		t.Errorf("错误应说明没有站点，got %v", err)
	}
	if len(ecx.Messages) != 0 {
		t.Errorf("报错时不得再播报假结果，got %v", ecx.Messages)
	}

	ecx2 := &workflow.Context{}
	err2 := NewSearchAllSitesNode(siteCh).Execute(context.Background(), map[string]any{"keyword": "某种剧", "category": ""}, ecx2)
	if err2 == nil || !strings.Contains(err2.Error(), "没有可搜索的站点") {
		t.Fatalf("search_all_sites 零站点同样必须报错，got err=%v 消息=%v", err2, ecx2.Messages)
	}
}
