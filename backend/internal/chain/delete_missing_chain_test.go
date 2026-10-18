package chain

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/hxpt/ptpatronus/internal/core/event"
	"github.com/hxpt/ptpatronus/internal/core/module"
	"github.com/hxpt/ptpatronus/internal/db/models"
	"github.com/hxpt/ptpatronus/internal/db/oper"
	"gorm.io/gorm"
)

// 取消订阅 / 删 RSS 源的存在性判定收在链层：HTTP 面、批量取消台账、
// AI 工具 subscribe_delete 与 servarr 移除都走这两个函数，抄在 handler 里
// 只会让工具面继续谎报「已删除」。
func newSubscribeProbeChains(t *testing.T) (*SubscribeChain, *RssChain, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/gone.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&models.Subscription{}, &models.SubscriptionHit{}, &models.RssSubscription{}, &models.RssItem{}); err != nil {
		t.Fatal(err)
	}
	subCh := NewSubscribeChain(nil, nil, nil, oper.NewSubscriptionOper(db), nil, nil, nil, nil)
	rssCh := NewRssChain(NewBase(module.New(), event.New()), oper.NewRssOper(db), nil, nil, nil, nil, nil, nil, nil)
	return subCh, rssCh, db
}

func TestDeleteMissingRecordAtChainLevel(t *testing.T) {
	subCh, rssCh, db := newSubscribeProbeChains(t)

	// 库里没有这条：链层必须报「记录不存在」，而不是 nil（GORM 删未命中不报错）。
	if err := subCh.Delete(999999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("删不存在的订阅应报 ErrRecordNotFound，got %v", err)
	}
	if err := rssCh.Delete(999999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("删不存在的 RSS 源应报 ErrRecordNotFound，got %v", err)
	}

	// 正例：真存在的删掉应成功，且第二次删同一条转「不存在」。
	s := &models.Subscription{Title: "示例订阅", Source: "tmdb", SourceID: "tv:1", Type: "tv"}
	if err := db.Create(s).Error; err != nil {
		t.Fatal(err)
	}
	if err := subCh.Delete(s.ID); err != nil {
		t.Fatalf("删除已存在的订阅不该报错，got %v", err)
	}
	var n int64
	if err := db.Model(&models.Subscription{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("删除后应剩 0 条，got %d", n)
	}
	if err := subCh.Delete(s.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("重复删除应报 ErrRecordNotFound，got %v", err)
	}
}
