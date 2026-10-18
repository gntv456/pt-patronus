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

func newSubSyncChain(t *testing.T) (*CloudChain, *oper.StrmRuleOper, *oper.SubscriptionOper) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/subsync.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.CloudDrive{}, &models.StrmRule{}, &models.StrmRecord{}, &models.Subscription{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	base := NewBase(module.New(), event.New())
	rOp := oper.NewStrmRuleOper(db)
	sOp := oper.NewSubscriptionOper(db)
	subCh := NewSubscribeChain(nil, nil, nil, sOp, nil, nil, nil, nil)
	ch, err := NewCloudChain(base, oper.NewCloudDriveOper(db, security.NewCrypto("subsync")), rOp, subCh, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ch, rOp, sOp
}

func seedSub(t *testing.T, op *oper.SubscriptionOper, title, status string) {
	t.Helper()
	if err := op.Create(&models.Subscription{Title: title, Source: "tmdb", Type: "tv", Status: status}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
}

// TestSyncSubscriptionsSummary 手动点「订阅追更」必须知道这一轮到底做了什么。
// 修复前本函数返回 void，HTTP 面恒回 {ok:true}，界面一律 toast「匹配完成」：
// 读失败、一条订阅都没有、扫完零命中，全都长得一样。
func TestSyncSubscriptionsSummary(t *testing.T) {
	ch, rOp, sOp := newSubSyncChain(t)
	seedSub(t, sOp, "Show", "subscribed")
	seedSub(t, sOp, "Ended", "ended") // 非追更中的订阅不该计入
	seedSub(t, sOp, "Ghost", "paused")
	for _, p := range []string{"/tv/Show.S01E01.mkv", "/tv/Other.S01E01.mkv", "/tv/already.mkv"} {
		if _, err := rOp.UpsertRecord(1, p, "/strm/"+p, 10, 0); err != nil {
			t.Fatalf("seed record: %v", err)
		}
	}
	// 已链接的记录不参与本轮匹配（复现「重复点击不会重复计数」）
	if _, err := rOp.UpsertRecord(2, "/tv/linked.mkv", "/strm/linked.mkv", 1, 7); err != nil {
		t.Fatalf("seed linked record: %v", err)
	}

	s := ch.SyncSubscriptions(context.Background())
	if s.Subs != 1 {
		t.Errorf("只有 subscribed 的订阅该计入，got subs=%d", s.Subs)
	}
	if s.Records != 3 { // 4 条记录里 1 条已链接，剩 already/Show/Other 三条待匹配
		t.Errorf("待匹配记录应为 3（已链接的那条不算），got records=%d", s.Records)
	}
	if s.Linked != 1 {
		t.Errorf("命中一条就该 linked=1，got linked=%d", s.Linked)
	}
	if len(s.Failed) != 0 {
		t.Errorf("本轮没有失败，got %v", s.Failed)
	}

	// 再点一次：命中的已链接，本轮零动作，但这是「无事可做」而不是失败
	again := ch.SyncSubscriptions(context.Background())
	if again.Linked != 0 || again.Records != 2 || len(again.Failed) != 0 {
		t.Errorf("第二轮应只剩两条未命中记录且零链接，got %+v", again)
	}
	if !strings.Contains(again.String(), "linked=0") {
		t.Errorf("String() 要能进日志看清零动作，got %q", again.String())
	}
}

// TestSyncSubscriptionsReportsMissingWiring 订阅模块没装配时不能说「完成」。
func TestSyncSubscriptionsReportsMissingWiring(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/nosub.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.CloudDrive{}, &models.StrmRule{}, &models.StrmRecord{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close() // Windows 上不关文件会让 TempDir 清理失败
		}
	})
	base := NewBase(module.New(), event.New())
	ch, err := NewCloudChain(base, oper.NewCloudDriveOper(db, security.NewCrypto("nosub")), oper.NewStrmRuleOper(db), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := ch.SyncSubscriptions(context.Background())
	if len(s.Failed) == 0 || !strings.Contains(s.Failed[0], "未装配") {
		t.Fatalf("sub 为 nil 必须自曝，got %+v", s)
	}
}
